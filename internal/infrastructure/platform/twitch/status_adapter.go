package twitchinfra

import (
	"context"

	"zhatBot/internal/domain"
)

type TwitchStatusAdapter struct {
	svc           domain.TwitchChannelService
	broadcasterID string
}

func NewTwitchStatusAdapter(
	svc domain.TwitchChannelService,
	broadcasterID string,
) domain.StreamStatusService {
	return &TwitchStatusAdapter{
		svc:           svc,
		broadcasterID: broadcasterID,
	}
}

func (a *TwitchStatusAdapter) Status(ctx context.Context) (domain.StreamStatus, error) {
	return a.svc.GetStreamStatus(ctx, a.broadcasterID)
}
