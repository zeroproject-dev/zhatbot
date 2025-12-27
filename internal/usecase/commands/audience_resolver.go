package commands

import (
	"context"
	"strings"

	"zhatBot/internal/domain"
)

type TwitchAudienceResolver struct {
	svc           domain.TwitchChannelService
	broadcasterID string
}

func NewTwitchAudienceResolver(svc domain.TwitchChannelService, broadcasterID string) CommandAudienceResolver {
	if svc == nil || strings.TrimSpace(broadcasterID) == "" {
		return nil
	}
	return &TwitchAudienceResolver{
		svc:           svc,
		broadcasterID: strings.TrimSpace(broadcasterID),
	}
}

func (r *TwitchAudienceResolver) IsFollower(ctx context.Context, msg domain.Message) (bool, error) {
	if r == nil || msg.Platform != domain.PlatformTwitch {
		return false, nil
	}
	return r.svc.IsFollower(ctx, r.broadcasterID, msg.UserID)
}
