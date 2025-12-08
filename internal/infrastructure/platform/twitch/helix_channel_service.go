package twitchinfra

import (
	"context"
	"fmt"
	"net/http"

	"github.com/nicklaw5/helix/v2"

	"zhatBot/internal/domain"
)

type HelixChannelService struct {
	client *helix.Client
}

// clientID: el de tu app de Twitch
// userAccessToken: token de TU cuenta de streamer con scope channel:manage:broadcast
func NewHelixChannelService(clientID, userAccessToken string) (domain.TwitchChannelService, error) {
	client, err := helix.NewClient(&helix.Options{
		ClientID:        clientID,
		UserAccessToken: userAccessToken,
	})
	if err != nil {
		return nil, fmt.Errorf("helix: NewClient: %w", err)
	}

	return &HelixChannelService{
		client: client,
	}, nil
}

// Implementación del puerto domain.TwitchChannelService
func (s *HelixChannelService) UpdateTitle(ctx context.Context, broadcasterID, newTitle string) error {
	resp, err := s.client.EditChannelInformation(&helix.EditChannelInformationParams{
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
