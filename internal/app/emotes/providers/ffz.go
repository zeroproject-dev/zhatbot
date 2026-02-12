package providers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"zhatBot/internal/app/emotes"
)

const (
	ffzGlobalURL = "https://api.frankerfacez.com/v1/set/global"
	ffzRoomURL   = "https://api.frankerfacez.com/v1/room/%s"
)

type FFZProvider struct {
	client httpClient
}

func NewFFZProvider(client *http.Client) *FFZProvider {
	return &FFZProvider{client: ensureClient(client)}
}

func (p *FFZProvider) Name() string {
	return "ffz"
}

func (p *FFZProvider) FetchGlobal(ctx context.Context) ([]emotes.Emote, error) {
	var payload ffzSetResponse
	if err := doJSON(ctx, p.client, http.MethodGet, ffzGlobalURL, &payload); err != nil {
		return nil, fmt.Errorf("ffz global: %w", err)
	}
	return p.extract(payload.Sets), nil
}

func (p *FFZProvider) FetchChannel(ctx context.Context, _ string, login string) ([]emotes.Emote, error) {
	login = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(login)), "#")
	if login == "" {
		return nil, nil
	}
	var payload ffzRoomResponse
	url := fmt.Sprintf(ffzRoomURL, login)
	if err := doJSON(ctx, p.client, http.MethodGet, url, &payload); err != nil {
		return nil, fmt.Errorf("ffz room: %w", err)
	}
	return p.extract(payload.Sets), nil
}

func (p *FFZProvider) extract(sets map[string]ffzSet) []emotes.Emote {
	if len(sets) == 0 {
		return nil
	}
	var out []emotes.Emote
	for _, set := range sets {
		for _, emote := range set.Emoticons {
			if emote.Name == "" {
				continue
			}
			urls := normalizeFFZURLs(emote.URLs)
			out = append(out, emotes.Emote{
				Provider: p.Name(),
				ID:       fmt.Sprintf("%d", emote.ID),
				Code:     emote.Name,
				Animated: emote.Animated,
				URLs:     urls,
			})
		}
	}
	return out
}

func normalizeFFZURLs(input map[string]string) emotes.EmoteURLs {
	resolve := func(key string) string {
		if value, ok := input[key]; ok {
			return absoluteURL(value)
		}
		return ""
	}
	// keys are typically "1", "2", "4" (x sizes) or "1x".
	urls := emotes.EmoteURLs{
		Small:  firstNonEmpty(resolve("1"), resolve("1x"), resolve("2")),
		Medium: firstNonEmpty(resolve("2"), resolve("2x"), resolve("3")),
		Large:  firstNonEmpty(resolve("4"), resolve("3x"), resolve("4x")),
	}
	return normalizeURLs(urls)
}

type ffzRoomResponse struct {
	Sets map[string]ffzSet `json:"sets"`
}

type ffzSetResponse struct {
	Sets map[string]ffzSet `json:"sets"`
}

type ffzSet struct {
	Emoticons []ffzEmote `json:"emoticons"`
}

type ffzEmote struct {
	ID       int               `json:"id"`
	Name     string            `json:"name"`
	Animated bool              `json:"animated"`
	URLs     map[string]string `json:"urls"`
}
