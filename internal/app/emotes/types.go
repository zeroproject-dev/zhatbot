package emotes

import (
	"context"
	"time"

	"zhatBot/internal/domain"
)

const (
	defaultGlobalTTL  = 6 * time.Hour
	defaultChannelTTL = 10 * time.Minute
)

// Provider representa una fuente de emotes externos (BTTV, FFZ, 7TV, etc).
type Provider interface {
	Name() string
	FetchGlobal(ctx context.Context) ([]Emote, error)
	FetchChannel(ctx context.Context, twitchID, login string) ([]Emote, error)
}

// Emote representa un emote normalizado proveniente de un proveedor.
type Emote struct {
	Provider string
	ID       string
	Code     string
	Animated bool
	URLs     EmoteURLs
}

type EmoteURLs struct {
	Small  string
	Medium string
	Large  string
}

// ChannelInfo contiene la información necesaria para resolver emotes de un canal.
type ChannelInfo struct {
	TwitchID    string
	TwitchLogin string
}

// ManagerOptions configura el comportamiento del administrador de emotes.
type ManagerOptions struct {
	Providers  []Provider
	GlobalTTL  time.Duration
	ChannelTTL time.Duration
}

// Manager mantiene caches de catálogos y resuelve tokens de terceros.
type Manager struct {
	providers  []*providerState
	globalTTL  time.Duration
	channelTTL time.Duration
}

// ResolveThirdPartyTokens convierte tokens de texto en emotes externos cuando corresponde.
func (m *Manager) ResolveThirdPartyTokens(ctx context.Context, tokens []domain.MessageToken, info ChannelInfo) []domain.MessageToken {
	if m == nil || len(tokens) == 0 {
		return tokens
	}
	catalogs := m.catalogsForChannel(ctx, info)
	if len(catalogs) == 0 {
		return tokens
	}
	return injectThirdPartyTokens(tokens, catalogs)
}
