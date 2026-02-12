package domain

type Platform string

const (
	PlatformTwitch Platform = "twitch"
	PlatformKick   Platform = "kick"
	// luego agregarás: discord, telegram, etc.
)

type Message struct {
	Platform  Platform
	ChannelID string
	UserID    string
	Username  string
	Text      string
	IsPrivate bool

	// Flags que vienen de la plataforma (los rellenamos en el adapter)
	IsPlatformOwner bool
	IsPlatformAdmin bool
	IsPlatformMod   bool
	IsPlatformVip   bool
	IsSubscriber    bool

	Tokens []MessageToken
}

type MessageTokenType string

const (
	MessageTokenText      MessageTokenType = "text"
	MessageTokenEmoteType MessageTokenType = "emote"
)

type MessageToken struct {
	Type  MessageTokenType   `json:"type"`
	Text  string             `json:"text,omitempty"`
	Emote *MessageTokenEmote `json:"emote,omitempty"`
}

type MessageTokenEmote struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Code     string `json:"code"`
	URL      string `json:"url"`
	URL2x    string `json:"url2x,omitempty"`
	URL3x    string `json:"url3x,omitempty"`
	Animated bool   `json:"animated"`
}
