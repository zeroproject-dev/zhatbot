package commands

import (
	"context"

	"zhatBot/internal/domain"
)

type PingCommand struct{}

func NewPingCommand() *PingCommand {
	return &PingCommand{}
}

func (c *PingCommand) Name() string {
	return "ping"
}

func (c *PingCommand) Aliases() []string {
	return []string{}
}

func (c *PingCommand) SupportsPlatform(p domain.Platform) bool {
	switch p {
	case domain.PlatformTwitch:
		return true
	default:
		return false
	}
}

func (c *PingCommand) Handle(ctx context.Context, cmdCtx *Context) error {
	msg := cmdCtx.Message

	response := "pong desde " + string(msg.Platform)

	return cmdCtx.Out.SendMessage(ctx, msg.Platform, msg.ChannelID, response)
}
