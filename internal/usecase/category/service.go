package category

import (
	"context"
	"fmt"
	"strings"

	"zhatBot/internal/domain"
)

// Service centraliza la lógica para buscar/actualizar categorías por plataforma.
type Service struct {
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

func (s *Service) Search(ctx context.Context, platform domain.Platform, query string) ([]domain.CategoryOption, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query vacío")
	}

	switch platform {
	case domain.PlatformTwitch:
		if s.twitch == nil {
			return nil, fmt.Errorf("servicio de Twitch no disponible")
		}
		return s.twitch.SearchCategories(ctx, query)
	case domain.PlatformKick:
		if s.kick == nil {
			return nil, fmt.Errorf("servicio de Kick no disponible")
		}
		return s.kick.SearchCategories(ctx, query)
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
		if s.twitch == nil {
			return fmt.Errorf("servicio de Twitch no disponible")
		}
		if s.twitchBroadcasterID == "" {
			return fmt.Errorf("broadcasterID de Twitch vacío")
		}
		return s.twitch.UpdateCategory(ctx, s.twitchBroadcasterID, categoryName)
	case domain.PlatformKick:
		if s.kick == nil {
			return fmt.Errorf("servicio de Kick no disponible")
		}
		return s.kick.SetCategory(ctx, categoryName)
	default:
		return fmt.Errorf("plataforma no soportada")
	}
}
