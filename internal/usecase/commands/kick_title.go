package commands

import (
	"context"
	"strings"

	"zhatBot/internal/domain"
)

type KickTitleCommand struct {
	StreamService domain.KickStreamService
	OwnerName     string
}

func NewKickTitleCommand(svc domain.KickStreamService, owner string) *KickTitleCommand {
	return &KickTitleCommand{
		StreamService: svc,
		OwnerName:     owner,
	}
}

func (c *KickTitleCommand) Name() string      { return "title" }
func (c *KickTitleCommand) Aliases() []string { return []string{} }

func (c *KickTitleCommand) SupportsPlatform(p domain.Platform) bool {
	return p == domain.PlatformKick
}

func (c *KickTitleCommand) Handle(ctx context.Context, cmdCtx *Context) error {
	msg := cmdCtx.Message

	// Solo el owner del canal en Kick
	if !strings.EqualFold(msg.Username, c.OwnerName) {
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"❌ Solo el dueño del canal puede cambiar el título en Kick.")
	}

	if len(cmdCtx.Args) == 0 {
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"Uso: !title <nuevo título>")
	}

	newTitle := strings.Join(cmdCtx.Args, " ")

	if err := c.StreamService.SetTitle(ctx, newTitle); err != nil {
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"⚠️ No pude cambiar el título en Kick.")
	}

	return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
		"✅ Título actualizado en Kick.")
}
