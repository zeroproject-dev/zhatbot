package twitchinfra

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/nicklaw5/helix/v2"

	"zhatBot/internal/domain"
)

type TwitchStreamService struct {
	client *helix.Client
	mu     sync.RWMutex
}

func NewStreamService(clientID, userAccessToken string) (domain.TwitchChannelService, error) {
	client, err := helix.NewClient(&helix.Options{
		ClientID:        clientID,
		UserAccessToken: userAccessToken,
	})
	if err != nil {
		return nil, fmt.Errorf("helix: NewClient: %w", err)
	}

	return &TwitchStreamService{
		client: client,
	}, nil
}

func (s *TwitchStreamService) SetTitle(ctx context.Context, broadcasterID, newTitle string) error {
	client := s.getClient()
	resp, err := client.EditChannelInformation(&helix.EditChannelInformationParams{
		BroadcasterID: broadcasterID,
		Title:         newTitle,
	})
	if err != nil {
		return fmt.Errorf("helix: EditChannelInformation: %w", err)
	}

	// El endpoint de "Modify Channel Information" devuelve 204 No Content en éxito.
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("helix: EditChannelInformation failed (%d: %s) %s",
			resp.StatusCode, resp.Error, resp.ErrorMessage)
	}

	return nil
}

func (s *TwitchStreamService) UpdateCategory(ctx context.Context, broadcasterID, gameName string) error {
	gameName = strings.TrimSpace(gameName)
	if gameName == "" {
		return fmt.Errorf("empty game name")
	}

	client := s.getClient()
	gamesResp, err := client.GetGames(&helix.GamesParams{
		Names: []string{gameName},
		// TODO: add favourite categories for fast changes instead of put the name IDs: []string,
	})
	if err != nil {
		return fmt.Errorf("helix: GetGames: %w", err)
	}

	if gamesResp.StatusCode != http.StatusOK {
		return fmt.Errorf("helix: GetGames failed (%d: %s) %s",
			gamesResp.StatusCode, gamesResp.Error, gamesResp.ErrorMessage)
	}

	if len(gamesResp.Data.Games) == 0 {
		return fmt.Errorf("game not found: %s", gameName)
	}

	game := gamesResp.Data.Games[0]

	// 2) Editar la info del canal con la nueva categoría
	editResp, err := client.EditChannelInformation(&helix.EditChannelInformationParams{
		BroadcasterID: broadcasterID,
		GameID:        game.ID,
	})
	if err != nil {
		return fmt.Errorf("helix: EditChannelInformation (category): %w", err)
	}

	if editResp.StatusCode != http.StatusNoContent && editResp.StatusCode != http.StatusOK {
		return fmt.Errorf("helix: EditChannelInformation (category) failed (%d: %s) %s",
			editResp.StatusCode, editResp.Error, editResp.ErrorMessage)
	}

	return nil
}

func (s *TwitchStreamService) SearchCategories(ctx context.Context, query string) ([]domain.CategoryOption, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}

	client := s.getClient()
	resp, err := client.SearchCategories(&helix.SearchCategoriesParams{
		Query: query,
		First: 25,
	})
	if err != nil {
		return nil, fmt.Errorf("helix: SearchCategories: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("helix: SearchCategories failed (%d: %s) %s",
			resp.StatusCode, resp.Error, resp.ErrorMessage)
	}

	options := make([]domain.CategoryOption, 0, len(resp.Data.Categories))
	for _, cat := range resp.Data.Categories {
		options = append(options, domain.CategoryOption{
			ID:   cat.ID,
			Name: cat.Name,
		})
	}

	return options, nil
}

func (s *TwitchStreamService) UpdateAccessToken(token string) {
	if s == nil || s.client == nil {
		return
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client.SetUserAccessToken(token)
}

func (s *TwitchStreamService) getClient() *helix.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}
