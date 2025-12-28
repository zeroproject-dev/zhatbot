package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"zhatBot/internal/app/events"
	appruntime "zhatBot/internal/app/runtime"
	ttsruntime "zhatBot/internal/app/tts/runner"
	"zhatBot/internal/domain"
	commandsusecase "zhatBot/internal/usecase/commands"
	statususecase "zhatBot/internal/usecase/status"
	ttsusecase "zhatBot/internal/usecase/tts"
)

type App struct {
	ctx             context.Context
	heartbeatCancel context.CancelFunc
	runtimeCancel   context.CancelFunc
	runtime         *appruntime.Runtime
	busSubs         []func()
	busWG           sync.WaitGroup
}

func NewApp() *App {
	return &App{}
}

func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx
	hbCtx, cancel := context.WithCancel(ctx)
	a.heartbeatCancel = cancel
	go a.emitHeartbeat(hbCtx)

	rtCtx, rtCancel := context.WithCancel(ctx)
	run, err := appruntime.Start(rtCtx, appruntime.Options{})
	if err != nil {
		rtCancel()
		wailsruntime.LogErrorf(ctx, "runtime start failed: %v", err)
		return
	}

	a.runtime = run
	a.runtimeCancel = rtCancel

	a.subscribeToTopic(events.TopicChatMessage)
	a.subscribeToTopic(events.TopicTTSStatus)
	a.subscribeToTopic(events.TopicTTSSpoken)
}

func (a *App) OnShutdown(ctx context.Context) {
	if a.heartbeatCancel != nil {
		a.heartbeatCancel()
	}

	for _, unsub := range a.busSubs {
		if unsub != nil {
			unsub()
		}
	}
	a.busSubs = nil
	a.busWG.Wait()

	if a.runtimeCancel != nil {
		a.runtimeCancel()
	}

	if a.runtime != nil {
		if err := a.runtime.Stop(); err != nil {
			wailsruntime.LogErrorf(ctx, "runtime stop error: %v", err)
		}
		a.runtime = nil
	}
}

func (a *App) subscribeToTopic(topic string) {
	if a.runtime == nil {
		return
	}
	bus := a.runtime.Bus()
	if bus == nil {
		return
	}

	ch, unsubscribe := bus.Subscribe(topic)
	a.busSubs = append(a.busSubs, unsubscribe)

	a.busWG.Add(1)
	go func() {
		defer a.busWG.Done()
		for {
			select {
			case <-a.ctx.Done():
				return
			case payload, ok := <-ch:
				if !ok {
					return
				}
				if a.ctx != nil {
					wailsruntime.EventsEmit(a.ctx, topic, payload)
				}
			}
		}
	}()
}

func (a *App) emitHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			if a.ctx != nil {
				payload := map[string]any{
					"ts":  t.UnixMilli(),
					"msg": "wails-heartbeat",
				}
				wailsruntime.EventsEmit(a.ctx, "app:heartbeat", payload)
			}
		}
	}
}

// Ping is a sample binding used to validate the bridge.
func (a *App) Ping() string {
	return "pong"
}

func (a *App) ListCommands() ([]commandsusecase.CommandDTO, error) {
	svc := a.commandService()
	if svc == nil {
		return nil, fmt.Errorf("commands service unavailable")
	}
	return svc.List(a.ctx)
}

func (a *App) UpsertCommand(payload commandsusecase.CommandMutationDTO) (commandsusecase.CommandDTO, error) {
	svc := a.commandService()
	if svc == nil {
		return commandsusecase.CommandDTO{}, fmt.Errorf("commands service unavailable")
	}
	result, err := svc.Upsert(a.ctx, payload)
	if err != nil {
		return commandsusecase.CommandDTO{}, err
	}
	a.emitCommandsChanged()
	return result, nil
}

func (a *App) DeleteCommand(name string) error {
	svc := a.commandService()
	if svc == nil {
		return fmt.Errorf("commands service unavailable")
	}
	deleted, err := svc.Delete(a.ctx, name)
	if err != nil {
		return err
	}
	if !deleted {
		return fmt.Errorf("command not found")
	}
	a.emitCommandsChanged()
	return nil
}

func (a *App) commandService() *commandsusecase.Service {
	if a.runtime == nil {
		return nil
	}
	return a.runtime.CommandService()
}

func (a *App) notificationRepo() domain.NotificationRepository {
	if a.runtime == nil {
		return nil
	}
	return a.runtime.NotificationRepo()
}

func (a *App) streamStatusResolver() *statususecase.Resolver {
	if a.runtime == nil {
		return nil
	}
	return a.runtime.StreamStatusResolver()
}

func (a *App) emitCommandsChanged() {
	if a.ctx == nil {
		return
	}
	payload := map[string]any{
		"ts": time.Now().UnixMilli(),
	}
	wailsruntime.EventsEmit(a.ctx, "commands:changed", payload)
}

type TTSSettingsUpdate struct {
	Voice   string `json:"voice"`
	Enabled *bool  `json:"enabled"`
}

type NotificationDTO struct {
	ID        int64             `json:"id"`
	Type      string            `json:"type"`
	Platform  string            `json:"platform,omitempty"`
	Username  string            `json:"username,omitempty"`
	Amount    float64           `json:"amount,omitempty"`
	Message   string            `json:"message,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt string            `json:"created_at"`
}

type StreamStatusDTO struct {
	Platform    string `json:"platform"`
	IsLive      bool   `json:"is_live"`
	Title       string `json:"title,omitempty"`
	GameTitle   string `json:"game_title,omitempty"`
	ViewerCount int    `json:"viewer_count,omitempty"`
	URL         string `json:"url,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
}

func (a *App) TTS_GetStatus() (events.TTSStatusDTO, error) {
	runner := a.ttsRunner()
	if runner == nil {
		return events.TTSStatusDTO{}, fmt.Errorf("tts runner unavailable")
	}
	return runner.Status(), nil
}

func (a *App) TTS_Enqueue(text, voice, lang string, rate, volume float64) (string, error) {
	service := a.ttsService()
	if service == nil {
		return "", fmt.Errorf("tts service unavailable")
	}
	req := ttsusecase.Request{
		Text:        text,
		VoiceCode:   voice,
		RequestedBy: "desktop",
		Platform:    domain.Platform("desktop"),
		ChannelID:   "desktop",
		Metadata: map[string]string{
			"lang":   lang,
			"rate":   fmt.Sprintf("%.2f", rate),
			"volume": fmt.Sprintf("%.2f", volume),
		},
		CreatedAt: time.Now(),
	}
	return service.Enqueue(a.ctx, req)
}

func (a *App) TTS_StopAll() error {
	runner := a.ttsRunner()
	if runner == nil {
		return fmt.Errorf("tts runner unavailable")
	}
	return runner.StopAll(a.ctx)
}

func (a *App) TTS_GetSettings() (ttsusecase.StatusSnapshot, error) {
	service := a.ttsService()
	if service == nil {
		return ttsusecase.StatusSnapshot{}, fmt.Errorf("tts service unavailable")
	}
	return service.Snapshot(a.ctx), nil
}

func (a *App) TTS_UpdateSettings(update TTSSettingsUpdate) (ttsusecase.StatusSnapshot, error) {
	service := a.ttsService()
	if service == nil {
		return ttsusecase.StatusSnapshot{}, fmt.Errorf("tts service unavailable")
	}
	if strings.TrimSpace(update.Voice) != "" {
		if _, err := service.SetVoice(a.ctx, update.Voice); err != nil {
			return ttsusecase.StatusSnapshot{}, err
		}
	}
	if update.Enabled != nil {
		if err := service.SetEnabled(a.ctx, *update.Enabled); err != nil {
			return ttsusecase.StatusSnapshot{}, err
		}
	}
	return service.Snapshot(a.ctx), nil
}

func (a *App) Notifications_List(limit int) ([]NotificationDTO, error) {
	repo := a.notificationRepo()
	if repo == nil {
		return nil, fmt.Errorf("notification repository unavailable")
	}
	if limit <= 0 {
		limit = 50
	}
	items, err := repo.ListNotifications(a.ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]NotificationDTO, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		created := ""
		if !item.CreatedAt.IsZero() {
			created = item.CreatedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, NotificationDTO{
			ID:        item.ID,
			Type:      string(item.Type),
			Platform:  string(item.Platform),
			Username:  item.Username,
			Amount:    item.Amount,
			Message:   item.Message,
			Metadata:  item.Metadata,
			CreatedAt: created,
		})
	}
	return out, nil
}

func (a *App) StreamStatus_List() ([]StreamStatusDTO, error) {
	resolver := a.streamStatusResolver()
	if resolver == nil {
		return nil, fmt.Errorf("stream status resolver unavailable")
	}
	snapshot := resolver.Snapshot(a.ctx)
	out := make([]StreamStatusDTO, 0, len(snapshot))
	for _, entry := range snapshot {
		started := ""
		if !entry.StartedAt.IsZero() {
			started = entry.StartedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, StreamStatusDTO{
			Platform:    string(entry.Platform),
			IsLive:      entry.IsLive,
			Title:       entry.Title,
			GameTitle:   entry.GameTitle,
			ViewerCount: entry.ViewerCount,
			URL:         entry.URL,
			StartedAt:   started,
		})
	}
	return out, nil
}

func (a *App) ttsService() *ttsusecase.Service {
	if a.runtime == nil {
		return nil
	}
	return a.runtime.TTSService()
}

func (a *App) ttsRunner() *ttsruntime.Runner {
	if a.runtime == nil {
		return nil
	}
	return a.runtime.TTSRunner()
}
