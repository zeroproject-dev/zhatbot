package commands

import (
	"context"
	"strings"

	"zhatBot/internal/domain"
)

type KickCategoryCommand struct {
	StreamService domain.KickStreamService
	OwnerName     string
}

func NewKickCategoryCommand(svc domain.KickStreamService, owner string) *KickCategoryCommand {
	return &KickCategoryCommand{
		StreamService: svc,
		OwnerName:     owner,
	}
}

func (c *KickCategoryCommand) Name() string      { return "category" }
func (c *KickCategoryCommand) Aliases() []string { return []string{"game"} }

func (c *KickCategoryCommand) SupportsPlatform(p domain.Platform) bool {
	return p == domain.PlatformKick
}

func (c *KickCategoryCommand) Handle(ctx context.Context, cmdCtx *Context) error {
	msg := cmdCtx.Message

	if !strings.EqualFold(msg.Username, c.OwnerName) {
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"❌ Solo el dueño del canal puede cambiar la categoría en Kick.")
	}

	if len(cmdCtx.Args) == 0 {
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"Uso: !category <nombre de la categoría>")
	}

	name := strings.Join(cmdCtx.Args, " ")

	if err := c.StreamService.SetCategory(ctx, name); err != nil {
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"⚠️ No pude cambiar la categoría en Kick.")
	}

	return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
		"✅ Categoría actualizada en Kick.")
}
