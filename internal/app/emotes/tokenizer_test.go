package emotes

import (
	"testing"

	"zhatBot/internal/domain"
)

func TestTokenizeTextWithPunctuation(t *testing.T) {
	input := []domain.MessageToken{
		{Type: domain.MessageTokenText, Text: "hola PepeHands!"},
	}
	catalogs := []map[string]domain.MessageTokenEmote{
		{
			"PepeHands": {
				Provider: "7tv",
				ID:       "seven-1",
				Code:     "PepeHands",
				URL:      "https://cdn.example/1",
			},
		},
	}

	result := injectThirdPartyTokens(input, catalogs)
	if len(result) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %+v", len(result), result)
	}
	if result[1].Type != domain.MessageTokenEmoteType || result[1].Emote == nil {
		t.Fatalf("expected second token to be emote: %+v", result[1])
	}
	if result[1].Emote.Code != "PepeHands" {
		t.Fatalf("unexpected emote code %s", result[1].Emote.Code)
	}
	if result[0].Text != "hola " || result[2].Text != "!" {
		t.Fatalf("punctuation not preserved: %+v", result)
	}
}

func TestTokenizeExactMatch(t *testing.T) {
	input := []domain.MessageToken{
		{Type: domain.MessageTokenText, Text: "PepeHands"},
	}
	catalogs := []map[string]domain.MessageTokenEmote{
		{
			"PepeHands": {
				Provider: "ffz",
				ID:       "ffz-1",
				Code:     "PepeHands",
				URL:      "https://cdn.example/pepe",
			},
		},
	}

	result := injectThirdPartyTokens(input, catalogs)
	if len(result) != 1 || result[0].Type != domain.MessageTokenEmoteType {
		t.Fatalf("expected a single emote token, got %+v", result)
	}
	if result[0].Emote.Provider != "ffz" {
		t.Fatalf("expected ffz provider, got %s", result[0].Emote.Provider)
	}
}

func TestProviderPriority(t *testing.T) {
	input := []domain.MessageToken{
		{Type: domain.MessageTokenText, Text: "OMEGALUL"},
	}
	catalogs := []map[string]domain.MessageTokenEmote{
		{
			"OMEGALUL": {Provider: "7tv", ID: "seven", Code: "OMEGALUL", URL: "https://7tv"},
		},
		{
			"OMEGALUL": {Provider: "bttv", ID: "bttv", Code: "OMEGALUL", URL: "https://bttv"},
		},
	}

	result := injectThirdPartyTokens(input, catalogs)
	if len(result) != 1 || result[0].Emote == nil {
		t.Fatalf("expected emote token, got %+v", result)
	}
	if result[0].Emote.Provider != "7tv" {
		t.Fatalf("expected first provider to win, got %s", result[0].Emote.Provider)
	}
}
