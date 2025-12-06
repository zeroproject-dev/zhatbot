// Package handle_message
package handle_message

import (
	"context"

	"zhatBot/internal/domain"
	"zhatBot/internal/usecase/commands"
)

type Interactor struct {
	router *commands.Router
	out    domain.OutgoingMessagePort
}

func NewInteractor(out domain.OutgoingMessagePort, router *commands.Router) *Interactor {
	return &Interactor{
		router: router,
		out:    out,
	}
}

func (uc *Interactor) Handle(ctx context.Context, msg domain.Message) error {
	return uc.router.Handle(ctx, msg, uc.out)
}
