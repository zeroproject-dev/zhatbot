package category

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"zhatBot/internal/domain"
)

// Service centraliza la lógica para buscar/actualizar categorías por plataforma.
type Service struct {
	mu                  sync.RWMutex
	twitch              domain.TwitchChannelService
	twitchBroadcasterID string
	kick                domain.KickStreamService
}

type Config struct {
	Twitch              domain.TwitchChannelService
	TwitchBroadcasterID string
	Kick                domain.KickStreamService
}

func NewService(cfg Config) *Service {
	return &Service{
		twitch:              cfg.Twitch,
		twitchBroadcasterID: strings.TrimSpace(cfg.TwitchBroadcasterID),
		kick:                cfg.Kick,
	}
}

func (s *Service) SetKickService(svc domain.KickStreamService) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kick = svc
}

func (s *Service) SetTwitchService(svc domain.TwitchChannelService, broadcasterID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.twitch = svc
	s.twitchBroadcasterID = strings.TrimSpace(broadcasterID)
}

func (s *Service) Search(ctx context.Context, platform domain.Platform, query string) ([]domain.CategoryOption, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query vacío")
	}

	switch platform {
	case domain.PlatformTwitch:
		s.mu.RLock()
		twitchSvc := s.twitch
		s.mu.RUnlock()
		if s.twitch == nil {
			return nil, fmt.Errorf("servicio de Twitch no disponible")
		}
		return twitchSvc.SearchCategories(ctx, query)
	case domain.PlatformKick:
		s.mu.RLock()
		kickSvc := s.kick
		s.mu.RUnlock()
		if kickSvc == nil {
			return nil, fmt.Errorf("servicio de Kick no disponible")
		}
		return kickSvc.SearchCategories(ctx, query)
	default:
		return nil, fmt.Errorf("plataforma no soportada")
	}
}

func (s *Service) Update(ctx context.Context, platform domain.Platform, categoryName string) error {
	categoryName = strings.TrimSpace(categoryName)
	if categoryName == "" {
		return fmt.Errorf("nombre de categoría vacío")
	}

	switch platform {
	case domain.PlatformTwitch:
		s.mu.RLock()
		twitchSvc := s.twitch
		broadcasterID := s.twitchBroadcasterID
		s.mu.RUnlock()
		if twitchSvc == nil {
			return fmt.Errorf("servicio de Twitch no disponible")
		}
		if broadcasterID == "" {
			return fmt.Errorf("broadcasterID de Twitch vacío")
		}
		return twitchSvc.UpdateCategory(ctx, broadcasterID, categoryName)
	case domain.PlatformKick:
		s.mu.RLock()
		kickSvc := s.kick
		s.mu.RUnlock()
		if kickSvc == nil {
			return fmt.Errorf("servicio de Kick no disponible")
		}
		return kickSvc.SetCategory(ctx, categoryName)
	default:
		return fmt.Errorf("plataforma no soportada")
	}
}
