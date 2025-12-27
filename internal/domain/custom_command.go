package domain

import (
	"context"
	"time"
)

type CustomCommand struct {
	Name      string
	Response  string
	Aliases   []string
	Platforms []Platform
	Permissions []CommandAccessRole
	UpdatedAt time.Time
}

type CommandAccessRole string

const (
	CommandAccessEveryone    CommandAccessRole = "everyone"
	CommandAccessFollowers   CommandAccessRole = "followers"
	CommandAccessSubscribers CommandAccessRole = "subscribers"
	CommandAccessModerators  CommandAccessRole = "moderators"
	CommandAccessVIPs        CommandAccessRole = "vips"
	CommandAccessOwner       CommandAccessRole = "owner"
)

type CustomCommandRepository interface {
	UpsertCustomCommand(ctx context.Context, cmd *CustomCommand) error
	GetCustomCommand(ctx context.Context, name string) (*CustomCommand, error)
	ListCustomCommands(ctx context.Context) ([]*CustomCommand, error)
	DeleteCustomCommand(ctx context.Context, name string) error
}
