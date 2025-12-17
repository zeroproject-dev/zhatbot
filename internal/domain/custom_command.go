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
	UpdatedAt time.Time
}

type CustomCommandRepository interface {
	UpsertCustomCommand(ctx context.Context, cmd *CustomCommand) error
	GetCustomCommand(ctx context.Context, name string) (*CustomCommand, error)
	ListCustomCommands(ctx context.Context) ([]*CustomCommand, error)
	DeleteCustomCommand(ctx context.Context, name string) error
}
