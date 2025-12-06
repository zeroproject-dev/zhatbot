package commands

import (
	"context"
	"strings"

	"zhatBot/internal/domain"
)

type Router struct {
	prefix   string
	cmdIndex map[string]Command
}

func NewRouter(prefix string) *Router {
	return &Router{
		prefix:   prefix,
		cmdIndex: make(map[string]Command),
	}
}

func (r *Router) Register(cmd Command) {
	r.cmdIndex[strings.ToLower(cmd.Name())] = cmd
	for _, alias := range cmd.Aliases() {
		r.cmdIndex[strings.ToLower(alias)] = cmd
	}
}

func (r *Router) Handle(ctx context.Context, msg domain.Message, out domain.OutgoingMessagePort) error {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return nil
	}

	if !strings.HasPrefix(text, r.prefix) {
		return nil
	}

	withoutPrefix := strings.TrimPrefix(text, r.prefix)
	parts := strings.Fields(withoutPrefix)
	if len(parts) == 0 {
		return nil
	}

	cmdName := strings.ToLower(parts[0])
	args := parts[1:]

	cmd, ok := r.cmdIndex[cmdName]
	if !ok {
		return out.SendMessage(ctx, msg.Platform, msg.ChannelID, "Comando no encontrado")
	}

	if !cmd.SupportsPlatform(msg.Platform) {
		return out.SendMessage(ctx, msg.Platform, msg.ChannelID, "Este comando no está disponible aquí.")
	}

	ctxCmd := &Context{
		Message: msg,
		Out:     out,
		Raw:     withoutPrefix,
		Args:    args,
	}

	return cmd.Handle(ctx, ctxCmd)
}
