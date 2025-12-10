package commands

import (
	"context"
	"log"
	"strings"

	"zhatBot/internal/domain"
)

type CategoryCommand struct {
	TwitchSvc     domain.TwitchChannelService
	BroadcasterID string
}

func NewCategoryCommand(svc domain.TwitchChannelService, broadcasterID string) *CategoryCommand {
	return &CategoryCommand{
		TwitchSvc:     svc,
		BroadcasterID: broadcasterID,
	}
}

func (c *CategoryCommand) Name() string      { return "category" }
func (c *CategoryCommand) Aliases() []string { return []string{"game"} } // opcional

func (c *CategoryCommand) SupportsPlatform(p domain.Platform) bool {
	return p == domain.PlatformTwitch // TODO: add more platforms
}

func (c *CategoryCommand) Handle(ctx context.Context, cmdCtx *Context) error {
	msg := cmdCtx.Message

	// 1) Solo el broadcaster (owner del canal) puede usarlo
	if !msg.IsPlatformOwner {
		// return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
		// "❌ Solo el dueño del canal puede cambiar la categoría.")
		return nil
	}

	// 2) Necesitamos el nombre de la categoría/juego
	if len(cmdCtx.Args) == 0 {
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"Uso: !category Nombre del juego/categoría\nEjemplo: !category Just Chatting")
	}

	gameName := strings.TrimSpace(strings.Join(cmdCtx.Args, " "))

	if err := c.TwitchSvc.UpdateCategory(ctx, c.BroadcasterID, gameName); err != nil {
		log.Printf("error actualizando categoría: %v", err)
		if strings.Contains(strings.ToLower(err.Error()), "game not found") {
			return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
				"😢 No encontré esa categoría/juego en Twitch: "+gameName)
		}

		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"😢 No pude cambiar la categoría, revisa los permisos del token (channel:manage:broadcast).")
	}

	return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
		"✅ Categoría actualizada a: "+gameName)
}
