package status

import (
	"context"
	"log"
	"sync"

	"zhatBot/internal/domain"
)

type Resolver struct {
	mu       sync.RWMutex
	services map[domain.Platform]domain.StreamStatusService
}

func NewResolver() *Resolver {
	return &Resolver{
		services: make(map[domain.Platform]domain.StreamStatusService),
	}
}

func (r *Resolver) Set(platform domain.Platform, svc domain.StreamStatusService) {
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

func (r *Resolver) Snapshot(ctx context.Context) []domain.StreamStatus {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	services := make(map[domain.Platform]domain.StreamStatusService, len(r.services))
	for platform, svc := range r.services {
		if svc != nil {
			services[platform] = svc
		}
	}
	r.mu.RUnlock()

	out := make([]domain.StreamStatus, 0, len(services))
	for platform, svc := range services {
		status, err := svc.Status(ctx)
		if err != nil {
			log.Printf("stream-status: %s status failed: %v", platform, err)
			continue
		}
		status.Platform = platform
		out = append(out, status)
	}

	return out
}
