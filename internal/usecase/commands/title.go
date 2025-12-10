package commands

import (
	"context"
	"strings"

	"zhatBot/internal/domain"
	"zhatBot/internal/usecase/stream"
)

type TitleCommand struct {
	resolver *stream.Resolver
}

func NewTitleCommand(
	resolver *stream.Resolver,
) *TitleCommand {
	return &TitleCommand{
		resolver: resolver,
	}
}

func (c *TitleCommand) Name() string {
	return "title"
}

func (c *TitleCommand) Aliases() []string {
	return []string{}
}

func (c *TitleCommand) SupportsPlatform(p domain.Platform) bool {
	// el mismo comando sirve para varias plataformas
	return p == domain.PlatformTwitch || p == domain.PlatformKick
}

func (c *TitleCommand) Handle(ctx context.Context, cmdCtx *Context) error {
	msg := cmdCtx.Message

	if !msg.IsPlatformAdmin {
		return nil
	}

	if len(cmdCtx.Args) == 0 {
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"Uso: !title <nuevo título>")
	}

	title := strings.Join(cmdCtx.Args, " ")

	// ✅ aquí está la magia
	service := c.resolver.ForPlatform(msg.Platform)
	if service == nil {
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"⚠️ Esta plataforma no soporta cambiar el título.")
	}

	if err := service.SetTitle(ctx, title); err != nil {
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"⚠️ Error al cambiar el título.")
	}

	return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
		"✅ Título actualizado.")
}
