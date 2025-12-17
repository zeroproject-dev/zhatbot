package commands

import (
	"context"
	"strings"

	"zhatBot/internal/domain"
)

type Router struct {
	prefix   string
	cmdIndex map[string]Command
	customs  *CustomCommandManager
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

func (r *Router) SetCustomManager(manager *CustomCommandManager) {
	r.customs = manager
	if manager != nil {
		manager.SetReservedChecker(r.isReservedCommand)
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
		return r.handleDynamic(ctx, cmdName, msg, out)
	}

	if !cmd.SupportsPlatform(msg.Platform) {
		if handled, err := r.tryCustom(ctx, cmdName, msg, out); handled {
			return err
		}
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

func (r *Router) handleDynamic(ctx context.Context, trigger string, msg domain.Message, out domain.OutgoingMessagePort) error {
	if handled, err := r.tryCustom(ctx, trigger, msg, out); handled {
		return err
	}
	return out.SendMessage(ctx, msg.Platform, msg.ChannelID, "Comando no encontrado")
}

func (r *Router) tryCustom(ctx context.Context, trigger string, msg domain.Message, out domain.OutgoingMessagePort) (bool, error) {
	if r.customs == nil {
		return false, nil
	}
	return r.customs.TryHandle(ctx, trigger, msg, out)
}

func (r *Router) isReservedCommand(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	_, ok := r.cmdIndex[name]
	return ok
}
