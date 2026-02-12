package providers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"zhatBot/internal/app/emotes"
)

const (
	bttvGlobalURL  = "https://api.betterttv.net/3/cached/emotes/global"
	bttvChannelURL = "https://api.betterttv.net/3/cached/users/twitch/%s"
	bttvCDN        = "https://cdn.betterttv.net/emote/%s"
)

type BTTVProvider struct {
	client httpClient
}

func NewBTTVProvider(client *http.Client) *BTTVProvider {
	return &BTTVProvider{client: ensureClient(client)}
}

func (p *BTTVProvider) Name() string {
	return "bttv"
}

func (p *BTTVProvider) FetchGlobal(ctx context.Context) ([]emotes.Emote, error) {
	var payload []bttvEmote
	if err := doJSON(ctx, p.client, http.MethodGet, bttvGlobalURL, &payload); err != nil {
		return nil, fmt.Errorf("bttv global: %w", err)
	}
	return p.normalize(payload), nil
}

func (p *BTTVProvider) FetchChannel(ctx context.Context, twitchID, _ string) ([]emotes.Emote, error) {
	if strings.TrimSpace(twitchID) == "" {
		return nil, nil
	}
	var payload bttvChannelResponse
	url := fmt.Sprintf(bttvChannelURL, twitchID)
	if err := doJSON(ctx, p.client, http.MethodGet, url, &payload); err != nil {
		return nil, fmt.Errorf("bttv channel: %w", err)
	}
	emotes := append([]bttvEmote{}, payload.ChannelEmotes...)
	emotes = append(emotes, payload.SharedEmotes...)
	return p.normalize(emotes), nil
}

func (p *BTTVProvider) normalize(list []bttvEmote) []emotes.Emote {
	out := make([]emotes.Emote, 0, len(list))
	for _, item := range list {
		if item.ID == "" || item.Code == "" {
			continue
		}
		emote := emotes.Emote{
			Provider: p.Name(),
			ID:       item.ID,
			Code:     item.Code,
			Animated: strings.EqualFold(item.ImageType, "gif"),
			URLs: emotes.EmoteURLs{
				Small:  fmt.Sprintf(bttvCDN, item.ID) + "/1x",
				Medium: fmt.Sprintf(bttvCDN, item.ID) + "/2x",
				Large:  fmt.Sprintf(bttvCDN, item.ID) + "/3x",
			},
		}
		out = append(out, emote)
	}
	return out
}

type bttvEmote struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	ImageType string `json:"imageType"`
}

type bttvChannelResponse struct {
	ChannelEmotes []bttvEmote `json:"channelEmotes"`
	SharedEmotes  []bttvEmote `json:"sharedEmotes"`
}
