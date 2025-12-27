package domain

import (
	"context"
	"time"
)

type StreamStatus struct {
	Platform    Platform
	IsLive      bool
	Title       string
	GameTitle   string
	ViewerCount int
	StartedAt   time.Time
	URL         string
}

type StreamStatusService interface {
	Status(ctx context.Context) (StreamStatus, error)
}
