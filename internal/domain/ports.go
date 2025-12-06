package domain

import "context"

type OutgoingMessagePort interface {
	SendMessage(ctx context.Context, platform Platform, channelID, text string) error
}

// IncomingMessagePort Para consumir mensajes entrantes (lo usan los usecases)
type IncomingMessagePort interface {
	// Tal vez no haga falta aquí; los adaptadores empujan los mensajes hacia usecases
}

// RoleRepository Repositorio de roles
type RoleRepository interface {
	GetByUser(ctx context.Context, platform Platform, userID string) (*Role, error)
	SetForUser(ctx context.Context, platform Platform, userID string, roleName string) error
}
