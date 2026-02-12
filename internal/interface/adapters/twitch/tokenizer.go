package twitchadapter

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"zhatBot/internal/domain"
)

const (
	twitchEmoteURLSmall  = "https://static-cdn.jtvnw.net/emoticons/v2/%s/default/dark/1.0"
	twitchEmoteURLMedium = "https://static-cdn.jtvnw.net/emoticons/v2/%s/default/dark/2.0"
	twitchEmoteURLLarge  = "https://static-cdn.jtvnw.net/emoticons/v2/%s/default/dark/3.0"
)

func tokenizeNativeEmotes(text, tag string) []domain.MessageToken {
	runes := []rune(text)
	if tag == "" {
		return []domain.MessageToken{newTextToken(text)}
	}

	spans := parseEmoteTag(tag, len(runes))
	if len(spans) == 0 {
		return []domain.MessageToken{newTextToken(text)}
	}

	sort.Slice(spans, func(i, j int) bool {
		return spans[i].start < spans[j].start
	})

	var tokens []domain.MessageToken
	cursor := 0
	for _, span := range spans {
		if span.start > len(runes) || span.end >= len(runes) || span.start > span.end {
			continue
		}
		if cursor < span.start {
			tokens = append(tokens, newTextToken(string(runes[cursor:span.start])))
		}
		code := string(runes[span.start : span.end+1])
		tokens = append(tokens, domain.MessageToken{
			Type: domain.MessageTokenEmoteType,
			Emote: &domain.MessageTokenEmote{
				Provider: "twitch",
				ID:       span.id,
				Code:     code,
				URL:      fmt.Sprintf(twitchEmoteURLSmall, span.id),
				URL2x:    fmt.Sprintf(twitchEmoteURLMedium, span.id),
				URL3x:    fmt.Sprintf(twitchEmoteURLLarge, span.id),
				Animated: false,
			},
		})
		cursor = span.end + 1
	}
	if cursor < len(runes) {
		tokens = append(tokens, newTextToken(string(runes[cursor:])))
	}
	if len(tokens) == 0 {
		return []domain.MessageToken{newTextToken(text)}
	}
	return tokens
}

func newTextToken(value string) domain.MessageToken {
	return domain.MessageToken{
		Type: domain.MessageTokenText,
		Text: value,
	}
}

type emoteSpan struct {
	id    string
	start int
	end   int
}

func parseEmoteTag(tag string, length int) []emoteSpan {
	var spans []emoteSpan
	chunks := strings.Split(tag, "/")
	for _, chunk := range chunks {
		if chunk == "" {
			continue
		}
		parts := strings.SplitN(chunk, ":", 2)
		if len(parts) != 2 {
			continue
		}
		emoteID := strings.TrimSpace(parts[0])
		if emoteID == "" {
			continue
		}
		for _, pos := range strings.Split(parts[1], ",") {
			if pos == "" {
				continue
			}
			index := strings.SplitN(pos, "-", 2)
			if len(index) != 2 {
				continue
			}
			start, err := strconv.Atoi(index[0])
			if err != nil || start < 0 {
				continue
			}
			end, err := strconv.Atoi(index[1])
			if err != nil || end < start {
				continue
			}
			if start >= length {
				continue
			}
			if end >= length {
				end = length - 1
			}
			spans = append(spans, emoteSpan{
				id:    emoteID,
				start: start,
				end:   end,
			})
		}
	}
	return spans
}
