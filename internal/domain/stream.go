package domain

import "context"

type StreamTitleService interface {
	SetTitle(ctx context.Context, title string) error
}
