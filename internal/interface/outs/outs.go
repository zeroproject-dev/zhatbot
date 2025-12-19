package outs

import (
	"context"
	"fmt"
	"sync"

	"zhatBot/internal/domain"
)

// Sender es la interfaz que deben implementar los adapters de salida (Twitch, Kick, etc.)
type Sender interface {
	// platform: de qué plataforma viene el mensaje original (Twitch, Kick, ...)
	// channelID: canal al que hay que responder (ej. "#zeroproject" en Twitch)
	SendMessage(ctx context.Context, platform domain.Platform, channelID, text string) error
}

// MultiSender enruta los mensajes al sender correcto según la plataforma.
type MultiSender struct {
	mu      sync.RWMutex
	senders map[domain.Platform]Sender
}

// NewMultiSender crea un MultiSender vacío.
func NewMultiSender() *MultiSender {
	return &MultiSender{
		senders: make(map[domain.Platform]Sender),
	}
}

// Register asocia una plataforma con un Sender concreto (ej. TwitchAdapter, KickAdapter).
func (m *MultiSender) Register(platform domain.Platform, sender Sender) {
	if m == nil || sender == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.senders[platform] = sender
}

// Unregister elimina el sender de una plataforma.
func (m *MultiSender) Unregister(platform domain.Platform) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.senders, platform)
}

// SendMessage busca el sender para esa plataforma y delega el envío.
func (m *MultiSender) SendMessage(ctx context.Context, platform domain.Platform, channelID, text string) error {
	if m == nil {
		return fmt.Errorf("no hay multi sender configurado")
	}
	m.mu.RLock()
	sender, ok := m.senders[platform]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no hay sender registrado para la plataforma %s", platform)
	}

	return sender.SendMessage(ctx, platform, channelID, text)
}
