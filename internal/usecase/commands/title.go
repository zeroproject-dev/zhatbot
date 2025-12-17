package commands

import (
	"context"
	"log"
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

	services := c.resolver.All()
	if len(services) == 0 {
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"⚠️ Esta plataforma no soporta cambiar el título.")
	}

	var failed bool
	for _, svc := range services {
		if err := svc.SetTitle(ctx, title); err != nil {
			log.Printf("title command: error setting title: %v", err)
			failed = true
		}
	}

	if failed {
		return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
			"⚠️ No pude cambiar el título en alguna plataforma.")
	}

	return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID,
		"✅ Título actualizado.")
}
