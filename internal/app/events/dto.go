package events

import (
	"time"

	"zhatBot/internal/domain"
)

// ChatMessageDTO describe el payload que se envía al frontend a través del bus/eventos.
type ChatMessageDTO struct {
	Platform        string `json:"platform"`
	ChannelID       string `json:"channel_id"`
	UserID          string `json:"user_id"`
	Username        string `json:"username"`
	Text            string `json:"text"`
	IsPrivate       bool   `json:"is_private"`
	IsPlatformOwner bool   `json:"is_platform_owner"`
	IsPlatformAdmin bool   `json:"is_platform_admin"`
	IsPlatformMod   bool   `json:"is_platform_mod"`
	IsPlatformVip   bool   `json:"is_platform_vip"`
	IsSubscriber    bool   `json:"is_subscriber"`
	Timestamp       string `json:"timestamp"`
}

// NewChatMessageDTO crea un DTO serializable a partir de domain.Message.
func NewChatMessageDTO(msg domain.Message) ChatMessageDTO {
	return ChatMessageDTO{
		Platform:        string(msg.Platform),
		ChannelID:       msg.ChannelID,
		UserID:          msg.UserID,
		Username:        msg.Username,
		Text:            msg.Text,
		IsPrivate:       msg.IsPrivate,
		IsPlatformOwner: msg.IsPlatformOwner,
		IsPlatformAdmin: msg.IsPlatformAdmin,
		IsPlatformMod:   msg.IsPlatformMod,
		IsPlatformVip:   msg.IsPlatformVip,
		IsSubscriber:    msg.IsSubscriber,
		Timestamp:       time.Now().UTC().Format(time.RFC3339Nano),
	}
}
