package commands

import (
	"context"

	"zhatBot/internal/domain"
)

type Command interface {
	Name() string
	Aliases() []string
	SupportsPlatform(p domain.Platform) bool
	Handle(ctx context.Context, c *Context) error
}

type Context struct {
	Message domain.Message
	Out     domain.OutgoingMessagePort

	Raw  string
	Args []string
}
