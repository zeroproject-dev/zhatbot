package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hegedustibor/htgo-tts/voices"

	"zhatBot/internal/domain"
)

type VoiceOption struct {
	Code  string
	Label string
}

type Service struct {
	repo      domain.TTSSettingsRepository
	publisher domain.TTSEventPublisher
	voices    []VoiceOption
	httpCli   *http.Client
}

func NewService(repo domain.TTSSettingsRepository, publisher domain.TTSEventPublisher, _ string) *Service {
	return &Service{
		repo:      repo,
		publisher: publisher,
		voices: []VoiceOption{
			{Code: voices.Spanish, Label: "Español"},
			{Code: "es-es", Label: "Español España"},
			{Code: voices.English, Label: "Inglés US"},
			{Code: voices.EnglishUK, Label: "Inglés UK"},
			{Code: voices.Portuguese, Label: "Portugués"},
			{Code: voices.French, Label: "Francés"},
			{Code: voices.German, Label: "Alemán"},
		},
		httpCli: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *Service) ListVoices() []VoiceOption {
	return append([]VoiceOption(nil), s.voices...)
}

func (s *Service) SetVoice(ctx context.Context, code string) (VoiceOption, error) {
	option, ok := s.findVoice(code)
	if !ok {
		return VoiceOption{}, fmt.Errorf("voz no soportada")
	}
	if s.repo != nil {
		if err := s.repo.SetTTSVoice(ctx, option.Code); err != nil {
			return VoiceOption{}, fmt.Errorf("no pude guardar la voz: %w", err)
		}
	}
	return option, nil
}

func (s *Service) CurrentVoice(ctx context.Context) VoiceOption {
	if s.repo != nil {
		if stored, err := s.repo.GetTTSVoice(ctx); err == nil {
			if option, ok := s.findVoice(stored); ok {
				return option
			}
		}
	}
	option, _ := s.findVoice("")
	return option
}

func (s *Service) RequestSpeech(ctx context.Context, text, requestedBy string, platform domain.Platform, channelID string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("texto vacío")
	}
	if !s.isEnabled(ctx) {
		return fmt.Errorf("el TTS está desactivado")
	}
	if s.publisher == nil {
		return fmt.Errorf("tts publisher no disponible")
	}
	voice := s.CurrentVoice(ctx)
	audio, err := s.generateAudio(text, voice.Code)
	if err != nil {
		return fmt.Errorf("tts: %w", err)
	}

	event := domain.TTSEvent{
		Voice:       voice.Code,
		VoiceLabel:  voice.Label,
		Text:        text,
		RequestedBy: requestedBy,
		Platform:    platform,
		ChannelID:   channelID,
		Timestamp:   time.Now(),
		AudioBase64: base64.StdEncoding.EncodeToString(audio),
	}

	return s.publisher.PublishTTSEvent(ctx, event)
}

func (s *Service) findVoice(code string) (VoiceOption, bool) {
	code = normalizeVoice(code)
	if code == "" {
		return s.voices[0], true
	}
	for _, option := range s.voices {
		if normalizeVoice(option.Code) == code {
			return option, true
		}
	}
	// allow prefix fallback (es-es -> es)
	if idx := strings.Index(code, "-"); idx > 0 {
		return s.findVoice(code[:idx])
	}
	return VoiceOption{}, false
}

func (s *Service) generateAudio(text, voice string) ([]byte, error) {
	voice = strings.TrimSpace(voice)
	if voice == "" {
		voice = voices.Spanish
	}

	chunkSize := 200
	runes := []rune(text)
	buf := bytes.NewBuffer(nil)

	for start := 0; start < len(runes); start += chunkSize {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunk := string(runes[start:end])
		audio, err := s.fetchChunk(chunk, voice)
		if err != nil {
			return nil, err
		}
		buf.Write(audio)
	}

	return buf.Bytes(), nil
}

func (s *Service) fetchChunk(text, voice string) ([]byte, error) {
	params := url.Values{}
	params.Set("ie", "UTF-8")
	params.Set("client", "tw-ob")
	params.Set("q", text)
	params.Set("tl", voice)
	params.Set("total", "1")
	params.Set("idx", "0")
	params.Set("textlen", fmt.Sprintf("%d", len([]rune(text))))

	req, err := http.NewRequest(http.MethodGet, "https://translate.google.com/translate_tts?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.httpCli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("tts: google tts status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

func normalizeVoice(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

func (s *Service) isEnabled(ctx context.Context) bool {
	if s.repo == nil {
		return true
	}
	enabled, err := s.repo.GetTTSEnabled(ctx)
	if err != nil {
		return true
	}
	return enabled
}

func (s *Service) SetEnabled(ctx context.Context, enabled bool) error {
	if s.repo == nil {
		return nil
	}
	return s.repo.SetTTSEnabled(ctx, enabled)
}

func (s *Service) Enabled(ctx context.Context) bool {
	return s.isEnabled(ctx)
}
