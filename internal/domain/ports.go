package domain

import (
	"context"
	"time"
)

type OutgoingMessagePort interface {
	SendMessage(ctx context.Context, platform Platform, channelID, text string) error
}

type MessagePublisher interface {
	PublishMessage(ctx context.Context, msg Message) error
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

type Credential struct {
	Platform     Platform
	Role         string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	UpdatedAt    time.Time
	Metadata     map[string]string
}

type CredentialRepository interface {
	Get(ctx context.Context, platform Platform, role string) (*Credential, error)
	Save(ctx context.Context, cred *Credential) error
	List(ctx context.Context) ([]*Credential, error)
}
