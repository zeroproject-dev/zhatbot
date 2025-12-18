package domain

import (
	"context"
	"time"
)

type TTSEvent struct {
	Voice       string    `json:"voice"`
	VoiceLabel  string    `json:"voice_label,omitempty"`
	Text        string    `json:"text"`
	RequestedBy string    `json:"requested_by"`
	Platform    Platform  `json:"platform"`
	ChannelID   string    `json:"channel_id"`
	Timestamp   time.Time `json:"timestamp"`
	AudioBase64 string    `json:"audio_base64"`
}

type TTSEventPublisher interface {
	PublishTTSEvent(ctx context.Context, event TTSEvent) error
}

type TTSSettingsRepository interface {
	SetTTSVoice(ctx context.Context, voice string) error
	GetTTSVoice(ctx context.Context) (string, error)
}
