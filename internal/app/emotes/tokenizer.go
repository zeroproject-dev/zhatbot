package emotes

import (
	"strings"
	"unicode"

	"zhatBot/internal/domain"
)

func injectThirdPartyTokens(tokens []domain.MessageToken, catalogs []map[string]domain.MessageTokenEmote) []domain.MessageToken {
	if len(tokens) == 0 || len(catalogs) == 0 {
		return tokens
	}

	out := make([]domain.MessageToken, 0, len(tokens))
	for _, token := range tokens {
		if token.Type != domain.MessageTokenText || token.Text == "" {
			out = append(out, token)
			continue
		}
		out = append(out, tokenizeText(token.Text, catalogs)...)
	}
	return compactTextTokens(out)
}

func tokenizeText(text string, catalogs []map[string]domain.MessageTokenEmote) []domain.MessageToken {
	if strings.TrimSpace(text) == "" {
		return []domain.MessageToken{{Type: domain.MessageTokenText, Text: text}}
	}

	segments := splitSegments(text)
	var result []domain.MessageToken

	for i := 0; i < len(segments); i++ {
		seg := segments[i]
		if seg.kind == segmentWhitespace {
			result = append(result, domain.MessageToken{Type: domain.MessageTokenText, Text: seg.text})
			continue
		}

		builder := seg.text
		if emote, ok := lookupEmote(builder, catalogs); ok {
			result = append(result, emoteToken(emote))
			continue
		}

		consumed := 1
		found := false
		for j := i + consumed; j < len(segments); j++ {
			if segments[j].kind == segmentWhitespace {
				break
			}
			builder += segments[j].text
			consumed++
			if emote, ok := lookupEmote(builder, catalogs); ok {
				result = append(result, emoteToken(emote))
				i += consumed - 1
				found = true
				break
			}
		}

		if !found {
			result = append(result, domain.MessageToken{Type: domain.MessageTokenText, Text: seg.text})
		}
	}

	return result
}

func compactTextTokens(tokens []domain.MessageToken) []domain.MessageToken {
	if len(tokens) < 2 {
		return tokens
	}
	result := make([]domain.MessageToken, 0, len(tokens))
	for _, token := range tokens {
		if token.Type == domain.MessageTokenText {
			if len(result) > 0 && result[len(result)-1].Type == domain.MessageTokenText {
				result[len(result)-1].Text += token.Text
				continue
			}
		}
		result = append(result, token)
	}
	return result
}

func lookupEmote(code string, catalogs []map[string]domain.MessageTokenEmote) (domain.MessageTokenEmote, bool) {
	for _, catalog := range catalogs {
		if len(catalog) == 0 {
			continue
		}
		if emote, ok := catalog[code]; ok {
			return emote, true
		}
	}
	return domain.MessageTokenEmote{}, false
}

func emoteToken(emote domain.MessageTokenEmote) domain.MessageToken {
	return domain.MessageToken{
		Type:  domain.MessageTokenEmoteType,
		Emote: &emote,
	}
}

type segmentKind int

const (
	segmentWhitespace segmentKind = iota
	segmentWord
	segmentSymbol
)

type segment struct {
	text string
	kind segmentKind
}

func splitSegments(value string) []segment {
	var segments []segment
	var builder strings.Builder
	currentKind := segmentKind(-1)

	flush := func() {
		if builder.Len() == 0 {
			return
		}
		segments = append(segments, segment{
			text: builder.String(),
			kind: currentKind,
		})
		builder.Reset()
	}

	for _, r := range value {
		kind := classifyRune(r)
		if kind == segmentSymbol {
			flush()
			segments = append(segments, segment{
				text: string(r),
				kind: kind,
			})
			currentKind = segmentKind(-1)
			continue
		}
		if currentKind != kind {
			flush()
			currentKind = kind
		}
		builder.WriteRune(r)
	}
	flush()

	if len(segments) == 0 {
		return []segment{{text: value, kind: segmentWord}}
	}
	return segments
}

func classifyRune(r rune) segmentKind {
	if unicode.IsSpace(r) {
		return segmentWhitespace
	}
	if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' {
		return segmentWord
	}
	return segmentSymbol
}
