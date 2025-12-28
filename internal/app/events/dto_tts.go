package events

import "time"

type TTSRequestDTO struct {
	ID          string  `json:"id"`
	Text        string  `json:"text"`
	Voice       string  `json:"voice"`
	VoiceLabel  string  `json:"voice_label,omitempty"`
	Lang        string  `json:"lang,omitempty"`
	Rate        float64 `json:"rate,omitempty"`
	Volume      float64 `json:"volume,omitempty"`
	RequestedBy string  `json:"requested_by,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

type TTSStatusDTO struct {
	State       string `json:"state"`
	QueueLength int    `json:"queue_length"`
	CurrentID   string `json:"current_id,omitempty"`
	LastError   string `json:"last_error,omitempty"`
	UpdatedAt   string `json:"updated_at"`
}

type TTSSpokenDTO struct {
	ID          string `json:"id"`
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	Text        string `json:"text,omitempty"`
	Voice       string `json:"voice,omitempty"`
	VoiceLabel  string `json:"voice_label,omitempty"`
	RequestedBy string `json:"requested_by,omitempty"`
	FinishedAt  string `json:"finished_at"`
	AudioBase64 string `json:"audio_base64,omitempty"`
}

func NewTTSStatusDTO(state string, queueLength int, currentID, lastError string) TTSStatusDTO {
	return TTSStatusDTO{
		State:       state,
		QueueLength: queueLength,
		CurrentID:   currentID,
		LastError:   lastError,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func NewTTSSpokenDTO(id string, ok bool, err error) TTSSpokenDTO {
	payload := TTSSpokenDTO{
		ID:         id,
		OK:         ok,
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err != nil {
		payload.Error = err.Error()
	}
	return payload
}
