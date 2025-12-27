package kickinfra

import (
	"context"

	"zhatBot/internal/domain"
)

type KickStatusAdapter struct {
	svc               domain.KickStreamService
	broadcasterUserID int
}

func NewKickStatusAdapter(
	svc domain.KickStreamService,
	broadcasterUserID int,
) domain.StreamStatusService {
	return &KickStatusAdapter{
		svc:               svc,
		broadcasterUserID: broadcasterUserID,
	}
}

func (a *KickStatusAdapter) Status(ctx context.Context) (domain.StreamStatus, error) {
	return a.svc.GetStreamStatus(ctx, a.broadcasterUserID)
}
