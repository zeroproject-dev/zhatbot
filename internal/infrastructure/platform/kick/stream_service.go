package kickinfra

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	kicksdk "github.com/glichtv/kick-sdk"
	optional "github.com/glichtv/kick-sdk/optional"

	"zhatBot/internal/domain"
)

type KickStreamServiceConfig struct {
	AccessToken string
}

type KickStreamService struct {
	client *kicksdk.Client
}

func NewStreamService(cfg KickStreamServiceConfig) (domain.KickStreamService, error) {
	if cfg.AccessToken == "" {
		return nil, fmt.Errorf("kick access token vacío")
	}

	client := kicksdk.NewClient(
		kicksdk.WithAccessTokens(kicksdk.AccessTokens{
			UserAccessToken: cfg.AccessToken,
		}),
	)

	return &KickStreamService{client: client}, nil
}

// Cambiar título del directo en Kick
func (s *KickStreamService) SetTitle(ctx context.Context, newTitle string) error {
	if strings.TrimSpace(newTitle) == "" {
		return fmt.Errorf("título vacío")
	}

	input := kicksdk.UpdateStreamInput{
		StreamTitle: optional.From(newTitle),
		// si tu SDK requiere BroadcasterUserID, lo añades aquí
	}

	_, err := s.client.Channels().UpdateStream(ctx, input)
	if err != nil {
		return fmt.Errorf("kick: error al actualizar título: %w", err)
	}

	return nil
}

func (s *KickStreamService) SetCategory(ctx context.Context, categoryName string) error {
	categoryName = strings.TrimSpace(categoryName)
	if categoryName == "" {
		return fmt.Errorf("categoría vacía")
	}

	// 1) armar el input correcto para Search
	searchInput := kicksdk.SearchCategoriesInput{
		Query: categoryName, // <- ESTE es el campo correcto
	}

	// 2) llamar a la API
	resp, err := s.client.Categories().Search(ctx, searchInput)
	if err != nil {
		return fmt.Errorf("kick: error buscando categorías: %w", err)
	}

	// 3) sacar el slice real del Response
	categories := resp.Payload // []kicksdk.Category

	if len(categories) == 0 {
		return fmt.Errorf("kick: no se encontró categoría para %q", categoryName)
	}

	categoryID := categories[0].ID

	// 4) actualizar el stream con esa categoría
	input := kicksdk.UpdateStreamInput{
		CategoryID: optional.From(categoryID),
		// si en algún momento necesitas ChannelID, se añade aquí con otro optional.From(...)
	}

	if _, err := s.client.Channels().UpdateStream(ctx, input); err != nil {
		return fmt.Errorf("kick: error actualizando categoría: %w", err)
	}

	return nil
}

func (s *KickStreamService) SearchCategories(ctx context.Context, query string) ([]domain.CategoryOption, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("categoría vacía")
	}

	searchInput := kicksdk.SearchCategoriesInput{
		Query: query,
	}

	resp, err := s.client.Categories().Search(ctx, searchInput)
	if err != nil {
		return nil, fmt.Errorf("kick: error buscando categorías: %w", err)
	}

	categories := resp.Payload

	options := make([]domain.CategoryOption, 0, len(categories))
	for _, cat := range categories {
		options = append(options, domain.CategoryOption{
			ID:   strconv.Itoa(cat.ID),
			Name: cat.Name,
		})
	}

	return options, nil
}
