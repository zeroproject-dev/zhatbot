package emotes

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"zhatBot/internal/domain"
)

// NewManager crea un administrador de emotes con las fuentes y TTL provistos.
func NewManager(opts ManagerOptions) *Manager {
	globalTTL := opts.GlobalTTL
	if globalTTL <= 0 {
		globalTTL = defaultGlobalTTL
	}
	channelTTL := opts.ChannelTTL
	if channelTTL <= 0 {
		channelTTL = defaultChannelTTL
	}

	var states []*providerState
	for _, provider := range opts.Providers {
		if provider == nil {
			continue
		}
		states = append(states, &providerState{
			provider: provider,
			name:     provider.Name(),
			channels: make(map[string]*cacheEntry),
		})
	}

	return &Manager{
		providers:  states,
		globalTTL:  globalTTL,
		channelTTL: channelTTL,
	}
}

type cacheEntry struct {
	data      map[string]domain.MessageTokenEmote
	expiresAt time.Time
}

func (c *cacheEntry) valid(now time.Time) bool {
	return c != nil && now.Before(c.expiresAt)
}

type providerState struct {
	provider Provider
	name     string

	mu       sync.RWMutex
	global   *cacheEntry
	channels map[string]*cacheEntry
}

func (p *providerState) getGlobal(ctx context.Context, ttl time.Duration) map[string]domain.MessageTokenEmote {
	now := time.Now()
	p.mu.RLock()
	entry := p.global
	if entry != nil && entry.valid(now) {
		defer p.mu.RUnlock()
		return entry.data
	}
	p.mu.RUnlock()

	emotes, err := p.provider.FetchGlobal(ctx)
	if err != nil {
		if entry != nil {
			log.Printf("emotes: provider=%s global fetch failed (using stale cache): %v", p.name, err)
			return entry.data
		}
		log.Printf("emotes: provider=%s global fetch failed: %v", p.name, err)
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.global = &cacheEntry{
		data:      indexEmotes(emotes),
		expiresAt: now.Add(ttl),
	}
	return p.global.data
}

func (p *providerState) getChannel(ctx context.Context, key string, info ChannelInfo, ttl time.Duration) map[string]domain.MessageTokenEmote {
	if key == "" {
		return nil
	}
	now := time.Now()
	p.mu.RLock()
	entry := p.channels[key]
	if entry != nil && entry.valid(now) {
		defer p.mu.RUnlock()
		return entry.data
	}
	p.mu.RUnlock()

	emotes, err := p.provider.FetchChannel(ctx, info.TwitchID, info.TwitchLogin)
	if err != nil {
		if entry != nil {
			log.Printf("emotes: provider=%s channel=%s fetch failed (using stale cache): %v", p.name, key, err)
			return entry.data
		}
		log.Printf("emotes: provider=%s channel=%s fetch failed: %v", p.name, key, err)
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	entry = &cacheEntry{
		data:      indexEmotes(emotes),
		expiresAt: now.Add(ttl),
	}
	p.channels[key] = entry
	return entry.data
}

func (m *Manager) catalogsForChannel(ctx context.Context, info ChannelInfo) []map[string]domain.MessageTokenEmote {
	if len(m.providers) == 0 {
		return nil
	}
	key := strings.TrimSpace(info.TwitchID)
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(info.TwitchLogin))
	}

	var catalogs []map[string]domain.MessageTokenEmote
	for _, provider := range m.providers {
		global := provider.getGlobal(ctx, m.globalTTL)
		channel := provider.getChannel(ctx, key, info, m.channelTTL)
		if len(global) == 0 && len(channel) == 0 {
			catalogs = append(catalogs, nil)
			continue
		}
		combined := make(map[string]domain.MessageTokenEmote, len(global)+len(channel))
		for code, emote := range global {
			combined[code] = emote
		}
		for code, emote := range channel {
			combined[code] = emote
		}
		catalogs = append(catalogs, combined)
	}
	return catalogs
}

func indexEmotes(list []Emote) map[string]domain.MessageTokenEmote {
	if len(list) == 0 {
		return nil
	}
	index := make(map[string]domain.MessageTokenEmote, len(list))
	for _, emote := range list {
		if emote.Code == "" {
			continue
		}
		index[emote.Code] = domain.MessageTokenEmote{
			Provider: emote.Provider,
			ID:       emote.ID,
			Code:     emote.Code,
			URL:      emote.URLs.Small,
			URL2x:    emote.URLs.Medium,
			URL3x:    emote.URLs.Large,
			Animated: emote.Animated,
		}
	}
	return index
}
