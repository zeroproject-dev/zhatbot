package providers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"zhatBot/internal/app/emotes"
)

const (
	sevenTVGlobalURL = "https://7tv.io/v3/emote-sets/global"
	sevenTVUserURL   = "https://7tv.io/v3/users/twitch/%s"
)

type SevenTVProvider struct {
	client httpClient
}

func NewSevenTVProvider(client *http.Client) *SevenTVProvider {
	return &SevenTVProvider{client: ensureClient(client)}
}

func (p *SevenTVProvider) Name() string {
	return "7tv"
}

func (p *SevenTVProvider) FetchGlobal(ctx context.Context) ([]emotes.Emote, error) {
	var payload sevenTVSet
	if err := doJSON(ctx, p.client, http.MethodGet, sevenTVGlobalURL, &payload); err != nil {
		return nil, fmt.Errorf("7tv global: %w", err)
	}
	return p.normalize(payload.Emotes), nil
}

func (p *SevenTVProvider) FetchChannel(ctx context.Context, twitchID, _ string) ([]emotes.Emote, error) {
	if strings.TrimSpace(twitchID) == "" {
		return nil, nil
	}
	var payload sevenTVUserResponse
	url := fmt.Sprintf(sevenTVUserURL, twitchID)
	if err := doJSON(ctx, p.client, http.MethodGet, url, &payload); err != nil {
		return nil, fmt.Errorf("7tv channel: %w", err)
	}
	if payload.EmoteSet == nil {
		return nil, nil
	}
	return p.normalize(payload.EmoteSet.Emotes), nil
}

func (p *SevenTVProvider) normalize(list []sevenTVSetEmote) []emotes.Emote {
	if len(list) == 0 {
		return nil
	}
	out := make([]emotes.Emote, 0, len(list))
	for _, item := range list {
		if item.ID == "" || item.Name == "" {
			continue
		}
		urls := buildSevenTVURLs(item.Data.Host)
		out = append(out, emotes.Emote{
			Provider: p.Name(),
			ID:       item.ID,
			Code:     item.Name,
			Animated: item.Data.Animated,
			URLs:     urls,
		})
	}
	return out
}

func buildSevenTVURLs(host sevenTVHost) emotes.EmoteURLs {
	base := strings.TrimSuffix(absoluteURL(host.URL), "/")
	if base == "" {
		return emotes.EmoteURLs{}
	}
	var small, medium, large string
	for _, file := range host.Files {
		if !strings.EqualFold(file.Format, "WEBP") {
			continue
		}
		scale := detectScale(file.Name)
		if scale == 0 {
			continue
		}
		url := fmt.Sprintf("%s/%s", base, file.Name)
		switch scale {
		case 1:
			if small == "" {
				small = url
			}
		case 2:
			if medium == "" {
				medium = url
			}
		case 3:
			if large == "" {
				large = url
			}
		}
	}
	urls := emotes.EmoteURLs{
		Small:  small,
		Medium: medium,
		Large:  large,
	}
	return normalizeURLs(urls)
}

func detectScale(name string) int {
	if name == "" {
		return 0
	}
	var digits strings.Builder
	for _, r := range name {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
			continue
		}
		if r == 'x' || r == 'X' {
			break
		}
		break
	}
	if digits.Len() == 0 {
		return 0
	}
	scale, err := strconv.Atoi(digits.String())
	if err != nil {
		return 0
	}
	return scale
}

type sevenTVUserResponse struct {
	EmoteSet *sevenTVSet `json:"emote_set"`
}

type sevenTVSet struct {
	Emotes []sevenTVSetEmote `json:"emotes"`
}

type sevenTVSetEmote struct {
	ID   string           `json:"id"`
	Name string           `json:"name"`
	Data sevenTVEmoteData `json:"data"`
}

type sevenTVEmoteData struct {
	Animated bool        `json:"animated"`
	Host     sevenTVHost `json:"host"`
}

type sevenTVHost struct {
	URL   string            `json:"url"`
	Files []sevenTVHostFile `json:"files"`
}

type sevenTVHostFile struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
