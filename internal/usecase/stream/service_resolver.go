package stream

import "zhatBot/internal/domain"

type Resolver struct {
	twitch domain.StreamTitleService
	kick   domain.StreamTitleService
}

func NewResolver(
	twitch domain.StreamTitleService,
	kick domain.StreamTitleService,
) *Resolver {
	return &Resolver{
		twitch: twitch,
		kick:   kick,
	}
}

func (r *Resolver) ForPlatform(p domain.Platform) domain.StreamTitleService {
	switch p {
	case domain.PlatformTwitch:
		return r.twitch
	case domain.PlatformKick:
		return r.kick
	default:
		return nil
	}
}
