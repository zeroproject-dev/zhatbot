package twitchinfra

import (
	"context"

	"zhatBot/internal/domain"
)

type TwitchTitleAdapter struct {
	svc           domain.TwitchChannelService
	broadcasterID string
}

func NewTwitchTitleAdapter(
	svc domain.TwitchChannelService,
	broadcasterID string,
) domain.StreamTitleService {
	return &TwitchTitleAdapter{
		svc:           svc,
		broadcasterID: broadcasterID,
	}
}

func (a *TwitchTitleAdapter) SetTitle(
	ctx context.Context,
	title string,
) error {
	return a.svc.SetTitle(ctx, a.broadcasterID, title)
}
