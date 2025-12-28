package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"zhatBot/internal/domain"
)

const (
	CommandSourceBuiltin = "builtin"
	CommandSourceCustom  = "custom"
)

type CommandDTO struct {
	Name        string                     `json:"name"`
	Response    string                     `json:"response"`
	Aliases     []string                   `json:"aliases"`
	Platforms   []string                   `json:"platforms"`
	Permissions []domain.CommandAccessRole `json:"permissions"`
	UpdatedAt   string                     `json:"updated_at"`
	Source      string                     `json:"source"`
	Editable    bool                       `json:"editable"`
	Description string                     `json:"description,omitempty"`
	Usage       string                     `json:"usage,omitempty"`
}

type CommandMutationDTO struct {
	Name        string                      `json:"name"`
	Response    *string                     `json:"response,omitempty"`
	Aliases     *[]string                   `json:"aliases,omitempty"`
	Platforms   *[]string                   `json:"platforms,omitempty"`
	Permissions *[]domain.CommandAccessRole `json:"permissions,omitempty"`
}

type Service struct {
	manager *CustomCommandManager
}

func NewService(manager *CustomCommandManager) *Service {
	return &Service{manager: manager}
}

func (s *Service) List(ctx context.Context) ([]CommandDTO, error) {
	_ = ctx
	out := builtinCommandDTOs()
	if s == nil || s.manager == nil {
		return out, nil
	}
	customCommands := s.manager.List()
	for _, cmd := range customCommands {
		out = append(out, commandDTOFromDomain(cmd))
	}
	return out, nil
}

func (s *Service) Upsert(ctx context.Context, input CommandMutationDTO) (CommandDTO, error) {
	if s == nil || s.manager == nil {
		return CommandDTO{}, fmt.Errorf("commands service unavailable")
	}
	update := convertMutationToInput(input)
	result, _, err := s.manager.Upsert(ctx, update)
	if err != nil {
		return CommandDTO{}, err
	}
	return commandDTOFromDomain(result), nil
}

func (s *Service) Delete(ctx context.Context, name string) (bool, error) {
	if s == nil || s.manager == nil {
		return false, fmt.Errorf("commands service unavailable")
	}
	return s.manager.Delete(ctx, name)
}

func commandDTOFromDomain(cmd *domain.CustomCommand) CommandDTO {
	if cmd == nil {
		return CommandDTO{}
	}
	platforms := make([]string, 0, len(cmd.Platforms))
	for _, p := range cmd.Platforms {
		if p == "" {
			continue
		}
		platforms = append(platforms, string(p))
	}
	updated := ""
	if !cmd.UpdatedAt.IsZero() {
		updated = cmd.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return CommandDTO{
		Name:        cmd.Name,
		Response:    cmd.Response,
		Aliases:     append([]string(nil), cmd.Aliases...),
		Platforms:   platforms,
		Permissions: append([]domain.CommandAccessRole(nil), cmd.Permissions...),
		UpdatedAt:   updated,
		Source:      CommandSourceCustom,
		Editable:    true,
	}
}

func builtinCommandDTOs() []CommandDTO {
	catalog := BuiltinCommandCatalog()
	out := make([]CommandDTO, 0, len(catalog))
	for _, item := range catalog {
		platforms := make([]string, 0, len(item.Platforms))
		for _, p := range item.Platforms {
			if p == "" {
				continue
			}
			platforms = append(platforms, string(p))
		}
		out = append(out, CommandDTO{
			Name:        item.Name,
			Aliases:     append([]string(nil), item.Aliases...),
			Platforms:   platforms,
			Permissions: append([]domain.CommandAccessRole(nil), item.Permissions...),
			Source:      CommandSourceBuiltin,
			Editable:    false,
			Description: item.Description,
			Usage:       item.Usage,
		})
	}
	return out
}

func convertMutationToInput(payload CommandMutationDTO) UpdateCustomCommandInput {
	input := UpdateCustomCommandInput{
		Name: payload.Name,
	}
	if payload.Response != nil {
		trimmed := strings.TrimSpace(*payload.Response)
		input.Response = &trimmed
	}
	if payload.Aliases != nil {
		input.HasAliases = true
		input.Aliases = append([]string(nil), *payload.Aliases...)
	}
	if payload.Platforms != nil {
		input.HasPlatforms = true
		for _, item := range *payload.Platforms {
			val := strings.ToLower(strings.TrimSpace(item))
			if val == "" {
				continue
			}
			input.Platforms = append(input.Platforms, domain.Platform(val))
		}
	}
	if payload.Permissions != nil {
		input.HasPermissions = true
		for _, role := range *payload.Permissions {
			val := domain.CommandAccessRole(strings.ToLower(strings.TrimSpace(string(role))))
			if val == "" {
				continue
			}
			input.Permissions = append(input.Permissions, val)
		}
	}
	return input
}
