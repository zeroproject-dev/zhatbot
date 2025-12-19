package stream

import (
	"sync"

	"zhatBot/internal/domain"
)

type Resolver struct {
	mu       sync.RWMutex
	services map[domain.Platform]domain.StreamTitleService
}

func NewResolver(
	twitch domain.StreamTitleService,
	kick domain.StreamTitleService,
) *Resolver {
	services := make(map[domain.Platform]domain.StreamTitleService)
	if twitch != nil {
		services[domain.PlatformTwitch] = twitch
	}
	if kick != nil {
		services[domain.PlatformKick] = kick
	}
	return &Resolver{
		services: services,
	}
}

func (r *Resolver) Set(platform domain.Platform, svc domain.StreamTitleService) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if svc == nil {
		delete(r.services, platform)
		return
	}
	r.services[platform] = svc
}

func (r *Resolver) ForPlatform(p domain.Platform) domain.StreamTitleService {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.services[p]
}

func (r *Resolver) All() []domain.StreamTitleService {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]domain.StreamTitleService, 0, len(r.services))
	for _, svc := range r.services {
		if svc != nil {
			list = append(list, svc)
		}
	}

	return list
}
