package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	kicksdk "github.com/glichtv/kick-sdk"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"zhatBot/internal/app/events"
	appruntime "zhatBot/internal/app/runtime"
	ttsruntime "zhatBot/internal/app/tts/runner"
	"zhatBot/internal/domain"
	"zhatBot/internal/infrastructure/config"
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
	oauthMu         sync.Mutex
	oauthFlows      map[string]*oauthLoopback
}

const (
	twitchAuthorizeURL = "https://id.twitch.tv/oauth2/authorize"
	twitchTokenURL     = "https://id.twitch.tv/oauth2/token"
)

func NewApp() *App {
	return &App{
		oauthFlows: make(map[string]*oauthLoopback),
	}
}

type oauthLoopback struct {
	provider     domain.Platform
	role         string
	state        string
	codeVerifier string
	redirectURI  string
	listener     net.Listener
	server       *http.Server
	result       chan oauthResult
	cancel       context.CancelFunc
}

type oauthResult struct {
	Provider string
	Status   string
	Error    string
}

func (f *oauthLoopback) sendResult(status string, err error) {
	if f == nil {
		return
	}
	var errMsg string
	if err != nil {
		errMsg = err.Error()
	}
	select {
	case f.result <- oauthResult{
		Provider: string(f.provider),
		Status:   status,
		Error:    errMsg,
	}:
	default:
	}
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
	a.subscribeToTopic(events.TopicTwitchBotConnected)
	a.subscribeToTopic(events.TopicTwitchBotError)
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

type NotificationCreateDTO struct {
	Type     string            `json:"type"`
	Platform string            `json:"platform"`
	Username string            `json:"username"`
	Amount   float64           `json:"amount"`
	Message  string            `json:"message"`
	Metadata map[string]string `json:"metadata"`
}

type CategoryOptionDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type OAuthCredentialStatusDTO struct {
	HasAccessToken  bool   `json:"has_access_token"`
	HasRefreshToken bool   `json:"has_refresh_token"`
	UpdatedAt       string `json:"updated_at,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
}

type OAuthStatusDTO struct {
	Credentials map[string]map[string]OAuthCredentialStatusDTO `json:"credentials"`
}

type ChatCommandDTO struct {
	Text      string `json:"text"`
	Platform  string `json:"platform"`
	ChannelID string `json:"channel_id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
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

func (a *App) Notifications_Create(payload NotificationCreateDTO) (NotificationDTO, error) {
	repo := a.notificationRepo()
	if repo == nil {
		return NotificationDTO{}, fmt.Errorf("notification repository unavailable")
	}

	notificationType := domain.NotificationType(strings.TrimSpace(payload.Type))
	if notificationType == "" {
		return NotificationDTO{}, fmt.Errorf("type is required")
	}

	record := &domain.Notification{
		Type:      notificationType,
		Platform:  parsePlatform(payload.Platform),
		Username:  strings.TrimSpace(payload.Username),
		Amount:    payload.Amount,
		Message:   strings.TrimSpace(payload.Message),
		Metadata:  payload.Metadata,
		CreatedAt: time.Now(),
	}

	if record.Metadata == nil {
		record.Metadata = make(map[string]string)
	}

	saved, err := repo.SaveNotification(a.ctx, record)
	if err != nil {
		return NotificationDTO{}, err
	}
	if saved == nil {
		return NotificationDTO{}, fmt.Errorf("notification not saved")
	}

	created := ""
	if saved != nil && !saved.CreatedAt.IsZero() {
		created = saved.CreatedAt.UTC().Format(time.RFC3339)
	}

	return NotificationDTO{
		ID:        saved.ID,
		Type:      string(saved.Type),
		Platform:  string(saved.Platform),
		Username:  saved.Username,
		Amount:    saved.Amount,
		Message:   saved.Message,
		Metadata:  saved.Metadata,
		CreatedAt: created,
	}, nil
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

func (a *App) Category_Search(platform, query string) ([]CategoryOptionDTO, error) {
	if a.runtime == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	service := a.runtime.CategoryService()
	if service == nil {
		return nil, fmt.Errorf("category service unavailable")
	}
	plat := parsePlatform(platform)
	if plat == "" {
		return nil, fmt.Errorf("invalid platform")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	results, err := service.Search(a.ctx, plat, query)
	if err != nil {
		return nil, err
	}
	out := make([]CategoryOptionDTO, 0, len(results))
	for _, option := range results {
		out = append(out, CategoryOptionDTO{
			ID:   option.ID,
			Name: option.Name,
		})
	}
	return out, nil
}

func (a *App) Category_Update(platform, name string) error {
	if a.runtime == nil {
		return fmt.Errorf("runtime unavailable")
	}
	service := a.runtime.CategoryService()
	if service == nil {
		return fmt.Errorf("category service unavailable")
	}
	plat := parsePlatform(platform)
	if plat == "" {
		return fmt.Errorf("invalid platform")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	return service.Update(a.ctx, plat, name)
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

func (a *App) OAuth_Start(platform, role string) error {
	if a.runtime == nil {
		return fmt.Errorf("runtime unavailable")
	}
	plat := parsePlatform(platform)
	if plat == "" {
		return fmt.Errorf("invalid platform")
	}
	role = normalizeRole(role)
	return a.startOAuthLoopback(plat, role)
}

func (a *App) OAuth_Status() (OAuthStatusDTO, error) {
	if a.runtime == nil {
		return OAuthStatusDTO{}, fmt.Errorf("runtime unavailable")
	}
	status, err := a.runtime.OAuthStatus(a.ctx)
	if err != nil {
		return OAuthStatusDTO{}, err
	}
	dto := OAuthStatusDTO{
		Credentials: make(map[string]map[string]OAuthCredentialStatusDTO),
	}
	for platform, roles := range status.Credentials {
		if dto.Credentials[platform] == nil {
			dto.Credentials[platform] = make(map[string]OAuthCredentialStatusDTO)
		}
		for role, entry := range roles {
			updated := ""
			if !entry.UpdatedAt.IsZero() {
				updated = entry.UpdatedAt.UTC().Format(time.RFC3339)
			}
			expires := ""
			if !entry.ExpiresAt.IsZero() {
				expires = entry.ExpiresAt.UTC().Format(time.RFC3339)
			}
			dto.Credentials[platform][role] = OAuthCredentialStatusDTO{
				HasAccessToken:  entry.HasAccessToken,
				HasRefreshToken: entry.HasRefreshToken,
				UpdatedAt:       updated,
				ExpiresAt:       expires,
			}
		}
	}
	return dto, nil
}

func (a *App) OAuth_Logout(platform, role string) error {
	if a.runtime == nil {
		return fmt.Errorf("runtime unavailable")
	}
	plat := parsePlatform(platform)
	if plat == "" {
		return fmt.Errorf("invalid platform")
	}
	return a.runtime.OAuthLogout(a.ctx, plat, role)
}

func parsePlatform(value string) domain.Platform {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(domain.PlatformTwitch):
		return domain.PlatformTwitch
	case string(domain.PlatformKick):
		return domain.PlatformKick
	default:
		return ""
	}
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "streamer":
		return "streamer"
	default:
		return "bot"
	}
}

func (a *App) startOAuthLoopback(platform domain.Platform, role string) error {
	if platform == domain.PlatformKick {
		role = "streamer"
	}
	cfg := a.runtime.Config()
	if cfg == nil {
		return fmt.Errorf("config unavailable")
	}
	if platform == domain.PlatformTwitch {
		if strings.TrimSpace(cfg.TwitchClientSecret) == "" {
			return a.requireTwitchSecret()
		}
	}

	basePort := preferredLoopbackPort(platform, cfg)
	listener, port, err := listenLoopbackWithFallback(basePort)
	if err != nil {
		return err
	}
	redirectURI := fmt.Sprintf("http://localhost:%d/oauth/callback/%s", port, platform)
	state, err := generateRandomString(32)
	if err != nil {
		listener.Close()
		return fmt.Errorf("state: %w", err)
	}

	flowCtx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	flow := &oauthLoopback{
		provider:    platform,
		role:        role,
		state:       state,
		redirectURI: redirectURI,
		listener:    listener,
		result:      make(chan oauthResult, 1),
		cancel:      cancel,
	}

	authURL, err := a.buildOAuthURL(flow, cfg)
	if err != nil {
		cancel()
		listener.Close()
		return err
	}

	a.oauthMu.Lock()
	if existing := a.oauthFlows[string(platform)]; existing != nil {
		existing.cancel()
	}
	a.oauthFlows[string(platform)] = flow
	a.oauthMu.Unlock()

	if err := a.runOAuthLoopbackServer(flowCtx, flow); err != nil {
		a.removeOAuthFlow(platform)
		cancel()
		return err
	}

	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "oauth:status", map[string]any{
			"provider": string(platform),
			"status":   "started",
			"redirect": redirectURI,
		})
		wailsruntime.BrowserOpenURL(a.ctx, authURL)
	}

	go a.waitOAuthResult(flowCtx, flow)
	return nil
}

func (a *App) buildOAuthURL(flow *oauthLoopback, cfg *config.Config) (string, error) {
	switch flow.provider {
	case domain.PlatformTwitch:
		clientID := strings.TrimSpace(cfg.TwitchClientId)
		if clientID == "" {
			return "", missingConfigError(a.ctx, "TWITCH_CLIENT_ID", "twitch_client_id")
		}
		verifier, err := generateRandomString(64)
		if err != nil {
			return "", err
		}
		flow.codeVerifier = verifier
		challenge := codeChallenge(verifier)

		q := url.Values{}
		q.Set("client_id", clientID)
		q.Set("redirect_uri", flow.redirectURI)
		q.Set("response_type", "code")
		q.Set("scope", strings.Join(twitchScopesForRole(flow.role), " "))
		q.Set("state", flow.state)
		q.Set("code_challenge", challenge)
		q.Set("code_challenge_method", "S256")
		return twitchAuthorizeURL + "?" + q.Encode(), nil
	case domain.PlatformKick:
		clientID := strings.TrimSpace(cfg.KickClientID)
		if clientID == "" {
			return "", missingConfigError(a.ctx, "KICK_CLIENT_ID", "kick_client_id")
		}
		verifier, err := generateRandomString(64)
		if err != nil {
			return "", err
		}
		flow.codeVerifier = verifier
		challenge := codeChallenge(verifier)

		client := kicksdk.NewClient(
			kicksdk.WithCredentials(kicksdk.Credentials{
				ClientID:    clientID,
				RedirectURI: flow.redirectURI,
			}),
		)

		return client.OAuth().AuthorizationURL(kicksdk.AuthorizationURLInput{
			ResponseType:  "code",
			State:         flow.state,
			Scopes:        kickScopesForRole(flow.role),
			CodeChallenge: challenge,
		}), nil
	default:
		return "", fmt.Errorf("unsupported platform %s", flow.provider)
	}
}

func (a *App) runOAuthLoopbackServer(ctx context.Context, flow *oauthLoopback) error {
	mux := http.NewServeMux()
	callbackPath := fmt.Sprintf("/oauth/callback/%s", flow.provider)
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		a.handleOAuthCallback(ctx, flow, w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	server := &http.Server{
		Handler: mux,
	}
	flow.server = server

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	go func() {
		err := server.Serve(flow.listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			flow.sendResult("error", fmt.Errorf("loopback server: %w", err))
		}
	}()

	return nil
}

func (a *App) waitOAuthResult(ctx context.Context, flow *oauthLoopback) {
	defer a.removeOAuthFlow(flow.provider)
	select {
	case result := <-flow.result:
		if a.ctx != nil {
			payload := map[string]any{
				"provider": result.Provider,
				"status":   result.Status,
			}
			if result.Error != "" {
				payload["error"] = result.Error
			}
			wailsruntime.EventsEmit(a.ctx, "oauth:complete", payload)
		}
	case <-ctx.Done():
		flow.sendResult("timeout", errors.New("timeout"))
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "oauth:complete", map[string]any{
				"provider": string(flow.provider),
				"status":   "timeout",
				"error":    "OAuth flow timed out",
			})
		}
	}
}

func (a *App) handleOAuthCallback(ctx context.Context, flow *oauthLoopback, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	queryState := strings.TrimSpace(r.URL.Query().Get("state"))
	if queryState == "" || queryState != flow.state {
		flow.sendResult("error", fmt.Errorf("invalid state"))
		writeOAuthHTML(w, false, "Estado inválido. Intenta de nuevo.")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		flow.sendResult("error", fmt.Errorf("missing code"))
		writeOAuthHTML(w, false, "No se recibió el código de autorización.")
		return
	}

	var err error
	switch flow.provider {
	case domain.PlatformTwitch:
		err = a.completeTwitchOAuth(ctx, flow, code)
	case domain.PlatformKick:
		err = a.completeKickOAuth(ctx, flow, code)
	default:
		err = fmt.Errorf("unsupported platform %s", flow.provider)
	}

	if err != nil {
		wailsruntime.LogErrorf(a.ctx, "oauth %s error: %v", flow.provider, err)
		writeOAuthHTML(w, false, "No se pudo completar el inicio de sesión.")
		flow.sendResult("error", err)
		flow.cancel()
		return
	}

	writeOAuthHTML(w, true, "✅ Listo. Puedes volver a la aplicación.")
	flow.sendResult("success", nil)
	flow.cancel()
}

func (a *App) completeTwitchOAuth(ctx context.Context, flow *oauthLoopback, code string) error {
	cfg := a.runtime.Config()
	if cfg == nil {
		return fmt.Errorf("config unavailable")
	}
	clientID := strings.TrimSpace(cfg.TwitchClientId)
	if clientID == "" {
		return missingConfigError(a.ctx, "TWITCH_CLIENT_ID", "twitch_client_id")
	}
	clientSecret := strings.TrimSpace(cfg.TwitchClientSecret)
	if clientSecret == "" {
		return a.requireTwitchSecret()
	}

	tokenResp, err := exchangeTwitchToken(ctx, clientID, clientSecret, flow.redirectURI, code, flow.codeVerifier)
	if err != nil {
		return err
	}

	metadata := make(map[string]string)
	if profile, err := fetchTwitchProfile(ctx, clientID, tokenResp.AccessToken); err == nil {
		if profile.ID != "" {
			metadata["user_id"] = profile.ID
		}
		if profile.Login != "" {
			metadata["login"] = profile.Login
		}
	}

	cred := &domain.Credential{
		Platform:     domain.PlatformTwitch,
		Role:         flow.role,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Metadata:     metadata,
	}

	repo := a.runtime.CredentialRepo()
	if repo == nil {
		return fmt.Errorf("credential repo unavailable")
	}
	if err := repo.Save(ctx, cred); err != nil {
		return err
	}
	log.Printf("twitch oauth (%s): credential stored; attempting connect", flow.role)
	a.runtime.NotifyCredentialUpdate(ctx, cred)
	return nil
}

func (a *App) completeKickOAuth(ctx context.Context, flow *oauthLoopback, code string) error {
	cfg := a.runtime.Config()
	if cfg == nil {
		return fmt.Errorf("config unavailable")
	}
	clientID := strings.TrimSpace(cfg.KickClientID)
	if clientID == "" {
		return missingConfigError(a.ctx, "KICK_CLIENT_ID", "kick_client_id")
	}

	payload, err := exchangeKickTokenPKCE(ctx, clientID, flow.redirectURI, code, flow.codeVerifier)
	if err != nil {
		return err
	}

	cred := &domain.Credential{
		Platform:     domain.PlatformKick,
		Role:         flow.role,
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second),
	}

	repo := a.runtime.CredentialRepo()
	if repo == nil {
		return fmt.Errorf("credential repo unavailable")
	}
	if err := repo.Save(ctx, cred); err != nil {
		return err
	}
	a.runtime.NotifyCredentialUpdate(ctx, cred)
	return nil
}

type twitchTokenResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int64    `json:"expires_in"`
	TokenType    string   `json:"token_type"`
	Scope        []string `json:"scope"`
}

type twitchProfile struct {
	ID    string `json:"id"`
	Login string `json:"login"`
}

func exchangeTwitchToken(ctx context.Context, clientID, clientSecret, redirectURI, code, verifier string) (*twitchTokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURI)
	data.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, twitchTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch token endpoint error: %s", string(body))
	}

	var payload twitchTokenResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func fetchTwitchProfile(ctx context.Context, clientID, accessToken string) (*twitchProfile, error) {
	token := strings.TrimSpace(accessToken)
	if token == "" || clientID == "" {
		return nil, fmt.Errorf("missing credentials")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitch.tv/helix/users", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-ID", clientID)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("twitch profile request failed (%d): %s", resp.StatusCode, string(body))
	}

	var payload struct {
		Data []twitchProfile `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("twitch profile empty")
	}
	return &payload.Data[0], nil
}

type kickTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

const kickTokenEndpoint = "https://api.kick.com/oauth/token"

func exchangeKickTokenPKCE(ctx context.Context, clientID, redirectURI, code, verifier string) (*kickTokenResponse, error) {
	data := url.Values{}
	data.Set("code", code)
	data.Set("client_id", clientID)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURI)
	data.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kickTokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kick token endpoint error: %s", string(body))
	}

	var payload kickTokenResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func (a *App) removeOAuthFlow(provider domain.Platform) {
	a.oauthMu.Lock()
	defer a.oauthMu.Unlock()
	delete(a.oauthFlows, string(provider))
}

func writeOAuthHTML(w http.ResponseWriter, success bool, message string) {
	status := "Error"
	if success {
		status = "Listo"
	}
	body := fmt.Sprintf(`
<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="utf-8">
<title>%s</title>
<style>
body { font-family: system-ui, sans-serif; background: #0f172a; color: #f8fafc; display:flex; align-items:center; justify-content:center; height:100vh; margin:0; }
.card { background: rgba(15,23,42,0.85); padding:2rem; border-radius:20px; text-align:center; max-width:360px; box-shadow:0 20px 60px rgba(0,0,0,0.4); }
button { margin-top:1.5rem; padding:0.6rem 1.5rem; border:none; border-radius:999px; background:#38bdf8; color:#0f172a; font-weight:600; cursor:pointer; }
</style>
</head>
<body>
	<div class="card">
		<h1>%s</h1>
		<p>%s</p>
		<button onclick="window.close()">Cerrar</button>
	</div>
</body>
</html>`, status, status, message)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, body)
}

func generateRandomString(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func codeChallenge(verifier string) string {
	sum := sha256Sum([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum)
}

func sha256Sum(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}

func twitchScopesForRole(role string) []string {
	if role == "streamer" {
		return []string{"channel:manage:broadcast"}
	}
	return []string{"chat:read", "chat:edit"}
}

func kickScopesForRole(role string) []kicksdk.OAuthScope {
	return []kicksdk.OAuthScope{
		kicksdk.ScopeUserRead,
		kicksdk.ScopeChannelRead,
		kicksdk.ScopeChannelWrite,
		kicksdk.ScopeChatWrite,
	}
}

func missingConfigError(ctx context.Context, envVar, jsonKey string) error {
	path := config.ConfigFilePath()
	if ctx != nil {
		payload := map[string]any{
			"path":        path,
			"missingKeys": []string{jsonKey},
		}
		wailsruntime.EventsEmit(ctx, "config:missing", payload)
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s missing. Set env var %s", envVar, envVar)
	}
	return fmt.Errorf("%s missing. Set env var %s or edit %s (%s)", envVar, envVar, path, jsonKey)
}

func (a *App) requireTwitchSecret() error {
	path := config.ConfigFilePath()
	if a.ctx != nil {
		payload := map[string]any{
			"provider":   "twitch",
			"configPath": path,
		}
		wailsruntime.EventsEmit(a.ctx, "oauth:missing-secret", payload)
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("Twitch Client Secret required. Please enter it in Settings.")
	}
	return fmt.Errorf("Twitch Client Secret required. Please edit %s.", path)
}

const defaultLoopbackPort = 17833

func preferredLoopbackPort(platform domain.Platform, cfg *config.Config) int {
	var raw string
	provider := string(platform)
	switch platform {
	case domain.PlatformTwitch:
		raw = cfg.TwitchRedirectURI
	case domain.PlatformKick:
		raw = cfg.KickRedirectURI
	}
	if port, ok := parseLocalhostPort(raw, provider); ok {
		return port
	}
	return defaultLoopbackPort
}

func parseLocalhostPort(raw, provider string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		log.Printf("desktop oauth: ignoring invalid redirect_uri for %s (%s)", provider, raw)
		return 0, false
	}
	if !strings.EqualFold(u.Scheme, "http") {
		log.Printf("desktop oauth: ignoring non-http redirect_uri for %s (%s)", provider, raw)
		return 0, false
	}
	host := strings.ToLower(u.Hostname())
	if host != "localhost" && host != "127.0.0.1" {
		log.Printf("desktop oauth: ignoring legacy redirect_uri for %s (%s); using localhost loopback", provider, raw)
		return 0, false
	}
	portStr := u.Port()
	if portStr == "" {
		return 0, false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		return 0, false
	}
	return port, true
}

func listenLoopbackWithFallback(basePort int) (net.Listener, int, error) {
	if basePort <= 0 {
		basePort = defaultLoopbackPort
	}
	var lastErr error
	for i := 0; i < 20; i++ {
		port := basePort + i
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, port, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("could not bind loopback port range starting at %d", basePort)
	}
	return nil, 0, fmt.Errorf("loopback listener: %w", lastErr)
}

func (a *App) Chat_SendCommand(payload ChatCommandDTO) error {
	if a.runtime == nil {
		return fmt.Errorf("runtime unavailable")
	}
	text := strings.TrimSpace(payload.Text)
	if text == "" {
		return fmt.Errorf("text is required")
	}
	msg := domain.Message{
		Platform:  parsePlatform(payload.Platform),
		ChannelID: strings.TrimSpace(payload.ChannelID),
		UserID:    strings.TrimSpace(payload.UserID),
		Username:  strings.TrimSpace(payload.Username),
		Text:      text,
	}
	if msg.Platform == "" {
		msg.Platform = domain.PlatformTwitch
	}
	if msg.UserID == "" {
		msg.UserID = "desktop"
	}
	if msg.Username == "" {
		msg.Username = "desktop"
	}
	return a.runtime.DispatchMessage(a.ctx, msg)
}

func (a *App) Config_SetTwitchSecret(secret string) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return fmt.Errorf("secret cannot be empty")
	}
	if err := config.SaveTwitchSecret(secret); err != nil {
		return err
	}
	if a.runtime != nil {
		if cfg := a.runtime.Config(); cfg != nil {
			cfg.TwitchClientSecret = secret
		}
	}
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "config:updated", map[string]any{
			"keys": []string{"twitch_client_secret"},
		})
	}
	return nil
}
