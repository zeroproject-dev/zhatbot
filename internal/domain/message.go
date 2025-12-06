package domain

type Platform string

const (
	PlatformTwitch Platform = "twitch"
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
}
