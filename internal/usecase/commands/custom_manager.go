package commands

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"zhatBot/internal/domain"
)

type CustomCommandManager struct {
	repo domain.CustomCommandRepository

	mu          sync.RWMutex
	commands    map[string]*domain.CustomCommand
	aliasToName map[string]string
	isReserved  func(string) bool
}

type UpdateCustomCommandInput struct {
	Name         string
	Response     *string
	Aliases      []string
	HasAliases   bool
	Platforms    []domain.Platform
	HasPlatforms bool
}

func NewCustomCommandManager(ctx context.Context, repo domain.CustomCommandRepository) (*CustomCommandManager, error) {
	mgr := &CustomCommandManager{
		repo:        repo,
		commands:    make(map[string]*domain.CustomCommand),
		aliasToName: make(map[string]string),
	}

	if repo == nil {
		return mgr, nil
	}

	list, err := repo.ListCustomCommands(ctx)
	if err != nil {
		return nil, fmt.Errorf("custom manager: list: %w", err)
	}

	for _, cmd := range list {
		if cmd == nil {
			continue
		}
		name := normalizeCommandName(cmd.Name)
		if name == "" {
			continue
		}
		mgr.commands[name] = cloneCommand(cmd)
	}
	mgr.rebuildAliasesLocked()

	return mgr, nil
}

func (m *CustomCommandManager) rebuildAliasesLocked() {
	m.aliasToName = make(map[string]string)
	for name, cmd := range m.commands {
		for _, alias := range cmd.Aliases {
			aliasKey := normalizeCommandName(alias)
			if aliasKey == "" {
				continue
			}
			m.aliasToName[aliasKey] = name
		}
	}
}

func (m *CustomCommandManager) Find(trigger string) *domain.CustomCommand {
	if m == nil {
		return nil
	}

	key := normalizeCommandName(trigger)
	if key == "" {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if cmd, ok := m.commands[key]; ok {
		return cloneCommand(cmd)
	}
	if canonical, ok := m.aliasToName[key]; ok {
		if cmd, ok := m.commands[canonical]; ok {
			return cloneCommand(cmd)
		}
	}
	return nil
}

func (m *CustomCommandManager) TryHandle(ctx context.Context, trigger string, msg domain.Message, out domain.OutgoingMessagePort) (bool, error) {
	cmd := m.Find(trigger)
	if cmd == nil {
		return false, nil
	}
	if len(cmd.Platforms) > 0 && !containsPlatform(cmd.Platforms, msg.Platform) {
		return false, nil
	}
	if strings.TrimSpace(cmd.Response) == "" {
		return false, nil
	}
	return true, out.SendMessage(ctx, msg.Platform, msg.ChannelID, cmd.Response)
}

func (m *CustomCommandManager) Upsert(ctx context.Context, input UpdateCustomCommandInput) (*domain.CustomCommand, bool, error) {
	if m == nil {
		return nil, false, fmt.Errorf("custom manager: nil")
	}
	name := normalizeCommandName(input.Name)
	if name == "" {
		return nil, false, fmt.Errorf("nombre inválido")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing := m.commands[name]
	created := false
	if existing == nil {
		existing = &domain.CustomCommand{
			Name: name,
		}
		created = true
	}

	if input.Response != nil {
		existing.Response = strings.TrimSpace(*input.Response)
	}
	if existing.Response == "" {
		return nil, false, fmt.Errorf("el contenido del comando es obligatorio")
	}

	proposedAliases := existing.Aliases
	if input.HasAliases {
		proposedAliases = normalizeAliasList(input.Aliases)
	}
	if err := m.ensureNoConflicts(name, created, proposedAliases, input.HasAliases); err != nil {
		return nil, false, err
	}

	if input.HasAliases {
		existing.Aliases = proposedAliases
	}
	if input.HasPlatforms {
		existing.Platforms = normalizePlatformList(input.Platforms)
	}
	existing.UpdatedAt = time.Now()

	if m.repo != nil {
		if err := m.repo.UpsertCustomCommand(ctx, existing); err != nil {
			return nil, false, err
		}
	}

	m.commands[name] = cloneCommand(existing)
	m.rebuildAliasesLocked()

	return cloneCommand(existing), created, nil
}

func (m *CustomCommandManager) Delete(ctx context.Context, name string) (bool, error) {
	if m == nil {
		return false, fmt.Errorf("custom manager nil")
	}
	key := normalizeCommandName(name)
	if key == "" {
		return false, fmt.Errorf("nombre inválido")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.commands[key]; !ok {
		return false, nil
	}

	if m.repo != nil {
		if err := m.repo.DeleteCustomCommand(ctx, key); err != nil {
			return false, err
		}
	}

	delete(m.commands, key)
	m.rebuildAliasesLocked()
	return true, nil
}

func normalizeCommandName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (m *CustomCommandManager) ensureNoConflicts(name string, created bool, aliases []string, hasAliases bool) error {
	if created && m.isReserved != nil && m.isReserved(name) {
		return fmt.Errorf("el nombre %q está reservado por otro comando", name)
	}

	if hasAliases && m.isReserved != nil {
		for _, alias := range aliases {
			if alias == "" {
				continue
			}
			if m.isReserved(alias) {
				return fmt.Errorf("el alias %q está reservado por otro comando", alias)
			}
		}
	}

	for existingName, cmd := range m.commands {
		if existingName == name {
			if created {
				return fmt.Errorf("ya existe un comando con ese nombre")
			}
			continue
		}
		if hasAliases {
			for _, alias := range aliases {
				if alias == "" {
					continue
				}
				if alias == existingName {
					return fmt.Errorf("el alias %q coincide con otro comando", alias)
				}
				for _, otherAlias := range cmd.Aliases {
					if alias == normalizeCommandName(otherAlias) {
						return fmt.Errorf("el alias %q ya está en uso", alias)
					}
				}
			}
		}
	}

	return nil
}

func (m *CustomCommandManager) SetReservedChecker(fn func(string) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isReserved = fn
}

func normalizeAliasList(values []string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, v := range values {
		key := normalizeCommandName(v)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func normalizePlatformList(values []domain.Platform) []domain.Platform {
	var out []domain.Platform
	seen := make(map[domain.Platform]struct{})
	for _, v := range values {
		val := domain.Platform(strings.ToLower(strings.TrimSpace(string(v))))
		if val == "" {
			continue
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	return out
}

func containsPlatform(list []domain.Platform, platform domain.Platform) bool {
	for _, p := range list {
		if p == platform {
			return true
		}
	}
	return false
}

func cloneCommand(cmd *domain.CustomCommand) *domain.CustomCommand {
	if cmd == nil {
		return nil
	}
	copyCmd := *cmd
	if cmd.Aliases != nil {
		copyCmd.Aliases = append([]string(nil), cmd.Aliases...)
	}
	if cmd.Platforms != nil {
		copyCmd.Platforms = append([]domain.Platform(nil), cmd.Platforms...)
	}
	return &copyCmd
}
