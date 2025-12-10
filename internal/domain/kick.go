package domain

import "context"

type KickStreamService interface {
	SetTitle(ctx context.Context, newTitle string) error
	SetCategory(ctx context.Context, categoryName string) error
}
