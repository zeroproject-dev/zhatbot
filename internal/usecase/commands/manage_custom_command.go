package commands

import (
	"context"
	"fmt"
	"strings"

	"zhatBot/internal/domain"
)

type ManageCustomCommand struct {
	manager *CustomCommandManager
}

func NewManageCustomCommand(manager *CustomCommandManager) *ManageCustomCommand {
	return &ManageCustomCommand{manager: manager}
}

func (c *ManageCustomCommand) Name() string {
	return "command"
}

func (c *ManageCustomCommand) Aliases() []string {
	return []string{}
}

func (c *ManageCustomCommand) SupportsPlatform(domain.Platform) bool {
	return true
}

func (c *ManageCustomCommand) Handle(ctx context.Context, cmdCtx *Context) error {
	if c.manager == nil {
		return nil
	}
	if !cmdCtx.Message.IsPlatformAdmin {
		return nil
	}

	raw := strings.TrimSpace(cmdCtx.Raw)
	if raw == "" {
		return c.usage(ctx, cmdCtx)
	}

	if !strings.HasPrefix(strings.ToLower(raw), c.Name()) {
		return c.usage(ctx, cmdCtx)
	}

	payload := strings.TrimSpace(raw[len(c.Name()):])
	if payload == "" {
		return c.usage(ctx, cmdCtx)
	}

	name, rest, found := strings.Cut(payload, " ")
	if !found {
		return c.usage(ctx, cmdCtx)
	}
	name = strings.TrimSpace(name)
	rest = strings.TrimSpace(rest)
	if name == "" {
		return c.usage(ctx, cmdCtx)
	}

	var aliases []string
	var platforms []domain.Platform
	var responseText string
	var hasResponse bool
	var hasAliases bool
	var hasPlatforms bool
	action := ""

	for {
		token, remaining := cutNext(rest)
		if token == "" {
			break
		}

		lower := strings.ToLower(token)
		switch {
		case strings.HasPrefix(lower, "aliases:"):
			hasAliases = true
			aliases = parseCSV(token[len("aliases:"):])
			rest = remaining
			continue
		case strings.HasPrefix(lower, "platforms:"):
			hasPlatforms = true
			platforms = parsePlatforms(token[len("platforms:"):])
			rest = remaining
			continue
		case strings.HasPrefix(lower, "action:"):
			action = strings.TrimSpace(token[len("action:"):])
			rest = remaining
			continue
		default:
			responseText = token
			if strings.TrimSpace(remaining) != "" {
				responseText += " " + strings.TrimSpace(remaining)
			}
			hasResponse = true
			rest = ""
		}
		break
	}

	if !hasResponse && rest != "" && !strings.EqualFold(strings.TrimSpace(action), "delete") {
		responseText = rest
		responseText = strings.TrimSpace(responseText)
		hasResponse = responseText != ""
	}

	var responsePtr *string
	if hasResponse {
		trimmed := strings.TrimSpace(responseText)
		responsePtr = &trimmed
	}

	if strings.EqualFold(action, "delete") {
		deleted, err := c.manager.Delete(ctx, name)
		if err != nil {
			return cmdCtx.Out.SendMessage(ctx, cmdCtx.Message.Platform, cmdCtx.Message.ChannelID,
				fmt.Sprintf("⚠️ %v", err))
		}
		if !deleted {
			return cmdCtx.Out.SendMessage(ctx, cmdCtx.Message.Platform, cmdCtx.Message.ChannelID,
				"⚠️ Comando no encontrado.")
		}
		return cmdCtx.Out.SendMessage(ctx, cmdCtx.Message.Platform, cmdCtx.Message.ChannelID,
			fmt.Sprintf("🗑️ Comando %s eliminado.", name))
	}

	result, created, err := c.manager.Upsert(ctx, UpdateCustomCommandInput{
		Name:         name,
		Response:     responsePtr,
		Aliases:      aliases,
		HasAliases:   hasAliases,
		Platforms:    platforms,
		HasPlatforms: hasPlatforms,
	})
	if err != nil {
		return cmdCtx.Out.SendMessage(ctx, cmdCtx.Message.Platform, cmdCtx.Message.ChannelID,
			fmt.Sprintf("⚠️ %v", err))
	}

	actionMsg := "actualizado"
	if created {
		actionMsg = "creado"
	}

	return cmdCtx.Out.SendMessage(ctx, cmdCtx.Message.Platform, cmdCtx.Message.ChannelID,
		fmt.Sprintf("✅ Comando %s %s.", result.Name, actionMsg))
}

func (c *ManageCustomCommand) usage(ctx context.Context, cmdCtx *Context) error {
	return cmdCtx.Out.SendMessage(ctx, cmdCtx.Message.Platform, cmdCtx.Message.ChannelID,
		"Uso: !command <nombre> [aliases:a,b] [platforms:twitch,kick] [action:delete] <respuesta>")
}

func cutNext(input string) (token string, rest string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", ""
	}
	parts := strings.SplitN(input, " ", 2)
	token = parts[0]
	if len(parts) == 2 {
		rest = strings.TrimSpace(parts[1])
	}
	return token, rest
}

func parseCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parsePlatforms(raw string) []domain.Platform {
	var out []domain.Platform
	for _, part := range parseCSV(raw) {
		out = append(out, domain.Platform(strings.ToLower(part)))
	}
	return out
}
