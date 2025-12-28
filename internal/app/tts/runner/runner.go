package runner

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/go-mp3"
	"github.com/hajimehoshi/oto/v2"

	"zhatBot/internal/app/events"
	"zhatBot/internal/domain"
	ttsusecase "zhatBot/internal/usecase/tts"
)

type Config struct {
	Service   *ttsusecase.Service
	Publisher domain.TTSEventPublisher
	Bus       *events.Bus
	QueueSize int
}

type Runner struct {
	cfg    Config
	queue  []*ttsusecase.Request
	mu     sync.Mutex
	cond   *sync.Cond
	wg     sync.WaitGroup
	closed bool

	current       *ttsusecase.Request
	cancelCurrent context.CancelFunc

	status events.TTSStatusDTO

	audioMu sync.Mutex
}

func New(cfg Config) *Runner {
	r := &Runner{
		cfg: cfg,
	}
	r.cond = sync.NewCond(&r.mu)
	r.status = events.NewTTSStatusDTO("idle", 0, "", "")
	return r
}

func (r *Runner) Start(ctx context.Context) {
	r.wg.Add(1)
	go func() {
		<-ctx.Done()
		r.mu.Lock()
		r.closed = true
		if r.cancelCurrent != nil {
			r.cancelCurrent()
		}
		r.mu.Unlock()
		r.cond.Broadcast()
	}()
	go func() {
		defer r.wg.Done()
		r.run(ctx)
	}()
	r.publishStatus(r.status)
}

func (r *Runner) run(ctx context.Context) {
	for {
		req, ok := r.next(ctx)
		if !ok {
			return
		}
		r.handleRequest(ctx, req)
	}
}

func (r *Runner) next(ctx context.Context) (*ttsusecase.Request, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for {
		if r.closed {
			return nil, false
		}
		if len(r.queue) > 0 {
			req := r.queue[0]
			r.queue = r.queue[1:]
			r.updateStatusLocked("speaking", len(r.queue), req.ID, "")
			return req, true
		}

		r.updateStatusLocked("idle", 0, "", "")
		r.cond.Wait()
		if ctx.Err() != nil {
			return nil, false
		}
	}
}

func (r *Runner) handleRequest(ctx context.Context, req *ttsusecase.Request) {
	if req == nil || r.cfg.Service == nil {
		return
	}

	childCtx, cancel := context.WithCancel(ctx)
	r.setCurrent(req, cancel)
	defer r.clearCurrent()

	audio, voice, err := r.cfg.Service.GenerateAudio(childCtx, req.Text, req.VoiceCode)
	if err != nil {
		r.handleFailure(req, fmt.Errorf("tts synth: %w", err))
		return
	}

	if err := r.publishTTSEvent(ctx, req, audio, voice); err != nil {
		log.Printf("tts runner: publish event failed: %v", err)
	}

	if err := r.playAudio(childCtx, audio); err != nil {
		if ctx.Err() != nil {
			r.handleFailure(req, context.Canceled)
			return
		}
		r.handleFailure(req, err)
		return
	}

	r.emitSpoken(req, true, nil, audio)
	r.updateStatus("idle", r.queueLength(), "", "")
}

func (r *Runner) publishTTSEvent(ctx context.Context, req *ttsusecase.Request, audio []byte, voice ttsusecase.VoiceOption) error {
	if r.cfg.Publisher == nil || req == nil {
		return nil
	}
	event := domain.TTSEvent{
		Voice:       voice.Code,
		VoiceLabel:  voice.Label,
		Text:        req.Text,
		RequestedBy: req.RequestedBy,
		Platform:    req.Platform,
		ChannelID:   req.ChannelID,
		Timestamp:   time.Now(),
		AudioBase64: base64.StdEncoding.EncodeToString(audio),
	}
	c := ctx
	if c == nil {
		c = context.Background()
	}
	return r.cfg.Publisher.PublishTTSEvent(c, event)
}

func (r *Runner) playAudio(ctx context.Context, audio []byte) error {
	if len(audio) == 0 {
		return fmt.Errorf("audio vacío")
	}
	r.audioMu.Lock()
	defer r.audioMu.Unlock()

	decoder, err := mp3.NewDecoder(bytes.NewReader(audio))
	if err != nil {
		return fmt.Errorf("mp3 decoder: %w", err)
	}

	otoCtx, readyChan, err := oto.NewContext(decoder.SampleRate(), 2, 2)
	if err != nil {
		return fmt.Errorf("oto context: %w", err)
	}
	<-readyChan

	player := otoCtx.NewPlayer(decoder)
	player.Play()
	defer player.Close()

	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()

	for {
		if !player.IsPlaying() {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}

	return nil
}

func (r *Runner) handleFailure(req *ttsusecase.Request, err error) {
	if err != nil {
		log.Printf("tts runner: %v", err)
		r.publish(events.TopicAppError, map[string]any{
			"source": "tts",
			"error":  err.Error(),
		})
	}
	r.updateStatus("error", r.queueLength(), idOrEmpty(req), safeError(err))
	r.emitSpoken(req, false, err, nil)
}

func (r *Runner) setCurrent(req *ttsusecase.Request, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current = req
	r.cancelCurrent = cancel
}

func (r *Runner) clearCurrent() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current = nil
	r.cancelCurrent = nil
	r.updateStatusLocked("idle", len(r.queue), "", "")
}

func (r *Runner) queueLength() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.queue)
}

func (r *Runner) StopAll(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancelCurrent != nil {
		r.cancelCurrent()
	}
	r.queue = nil
	r.updateStatusLocked("stopped", 0, "", "")
	r.cond.Broadcast()
	return nil
}

func (r *Runner) Enqueue(ctx context.Context, req ttsusecase.Request) (string, error) {
	if r.cfg.Service == nil {
		return "", fmt.Errorf("tts service no disponible")
	}

	req.ID = ensureID(req.ID)

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return "", fmt.Errorf("tts runner detenido")
	}

	r.queue = append(r.queue, &req)
	r.updateStatusLocked(r.status.State, len(r.queue), r.status.CurrentID, r.status.LastError)
	r.cond.Signal()
	return req.ID, nil
}

func (r *Runner) Status() events.TTSStatusDTO {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *Runner) Close() error {
	r.mu.Lock()
	r.closed = true
	if r.cancelCurrent != nil {
		r.cancelCurrent()
	}
	r.queue = nil
	r.cond.Broadcast()
	r.mu.Unlock()

	r.wg.Wait()
	return nil
}

func (r *Runner) emitSpoken(req *ttsusecase.Request, ok bool, err error, audio []byte) {
	if req == nil {
		return
	}
	payload := events.TTSSpokenDTO{
		ID:          req.ID,
		OK:          ok,
		Text:        req.Text,
		Voice:       req.VoiceCode,
		VoiceLabel:  req.VoiceLabel,
		RequestedBy: req.RequestedBy,
		FinishedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err != nil {
		payload.Error = err.Error()
	}
	if len(audio) > 0 {
		payload.AudioBase64 = base64.StdEncoding.EncodeToString(audio)
	}
	r.publish(events.TopicTTSSpoken, payload)
}

func (r *Runner) updateStatus(state string, queueLength int, currentID, lastError string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setStatus(state, queueLength, currentID, lastError)
}

func (r *Runner) updateStatusLocked(state string, queueLength int, currentID, lastError string) {
	r.setStatus(state, queueLength, currentID, lastError)
}

func (r *Runner) setStatus(state string, queueLength int, currentID, lastError string) {
	if strings.TrimSpace(state) == "" {
		state = "idle"
	}
	r.status = events.NewTTSStatusDTO(state, queueLength, currentID, lastError)
	r.publish(events.TopicTTSStatus, r.status)
}

func (r *Runner) publishStatus(status events.TTSStatusDTO) {
	r.publish(events.TopicTTSStatus, status)
}

func (r *Runner) publish(topic string, payload any) {
	if r.cfg.Bus != nil {
		r.cfg.Bus.Publish(topic, payload)
	}
}

func ensureID(id string) string {
	if strings.TrimSpace(id) != "" {
		return id
	}
	return fmt.Sprintf("tts-%d", time.Now().UnixNano())
}

func idOrEmpty(req *ttsusecase.Request) string {
	if req == nil {
		return ""
	}
	return req.ID
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
