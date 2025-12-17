package domain

import "context"

// Puerto para hacer acciones sobre el canal de Twitch vía Helix.
type TwitchChannelService interface {
	// broadcasterID: ID numérico del canal (tu cuenta de streamer)
	// newTitle: nuevo título a poner
	SetTitle(ctx context.Context, broadcasterID, newTitle string) error

	// broadcasterID: ID numérico del canal (tu cuenta de streamer)
	// gameName: Nombre de la categoria
	UpdateCategory(ctx context.Context, broadcasterID, gameName string) error

	SearchCategories(ctx context.Context, query string) ([]CategoryOption, error)
}
