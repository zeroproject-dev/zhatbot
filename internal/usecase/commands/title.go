package commands

import (
	"context"
	"log"
	"strings"

	"zhatBot/internal/domain"
)

type TitleCommand struct {
	TwitchSvc     domain.TwitchChannelService
	BroadcasterID string
}

func NewTitleCommand(svc domain.TwitchChannelService, broadcasterID string) *TitleCommand {
	return &TitleCommand{
		TwitchSvc:     svc,
		BroadcasterID: broadcasterID,
	}
}

func (c *TitleCommand) Name() string      { return "title" }
func (c *TitleCommand) Aliases() []string { return []string{} }

func (c *TitleCommand) SupportsPlatform(p domain.Platform) bool {
	return p == domain.PlatformTwitch // TODO: add to kick and youtube or tiktok
}

func (c *TitleCommand) Handle(ctx context.Context, cmdCtx *Context) error {
	msg := cmdCtx.Message

	// 1) Solo el dueño del canal (broadcaster) puede usarlo
	if !msg.IsPlatformOwner {
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"❌ Solo el dueño del canal puede cambiar el título.")
	}

	// 2) Necesitamos el nuevo título
	if len(cmdCtx.Args) == 0 {
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"Uso: !title Nuevo título de la transmisión")
	}

	newTitle := strings.TrimSpace(strings.Join(cmdCtx.Args, " "))

	// 3) Llamar a la API de Twitch vía servicio Helix
	if err := c.TwitchSvc.UpdateTitle(ctx, c.BroadcasterID, newTitle); err != nil {
		log.Printf("error actualizando título: %v", err)
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"😢 No pude cambiar el título, revisa los permisos del token (channel:manage:broadcast).")
	}

	// 4) Confirmar en el chat
	return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
		"✅ Título actualizado: "+newTitle)
}
