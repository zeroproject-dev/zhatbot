package domain

import "context"

// Puerto para hacer acciones sobre el canal de Twitch vía Helix.
type TwitchChannelService interface {
	// broadcasterID: ID numérico del canal (tu cuenta de streamer)
	// newTitle: nuevo título a poner
	UpdateTitle(ctx context.Context, broadcasterID, newTitle string) error
}
