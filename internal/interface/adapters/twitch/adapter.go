// Package twitchadapter adapter for twitch
package twitchadapter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/adeithe/go-twitch/irc"

	"zhatBot/internal/domain"
)

type Config struct {
	Username   string
	OAuthToken string
	Channels   []string
}

type MessageHandler func(ctx context.Context, msg domain.Message) error

type Adapter struct {
	cfg     Config
	handler MessageHandler

	mu   sync.RWMutex
	conn *irc.Conn
}

func NewAdapter(cfg Config) *Adapter {
	return &Adapter{cfg: cfg}
}

func (a *Adapter) SetHandler(h MessageHandler) {
	a.handler = h
}

func (a *Adapter) Start(ctx context.Context) error {
	if len(a.cfg.Channels) == 0 {
		return errors.New("twitch: no hay canales configurados")
	}
	if a.cfg.Username == "" || a.cfg.OAuthToken == "" {
		return errors.New("twitch: username u oauth token vacíos")
	}

	// 🔹 Usamos UNA sola conexión simple, sin sharding
	conn := &irc.Conn{}

	if err := conn.SetLogin(a.cfg.Username, a.cfg.OAuthToken); err != nil {
		return fmt.Errorf("twitch: SetLogin: %w", err)
	}

	conn.OnMessage(func(cm irc.ChatMessage) {
		// log.Printf("[Twitch] %s: %s", cm.Sender.DisplayName, cm.Text)

		a.mu.RLock()
		handler := a.handler
		a.mu.RUnlock()
		if handler == nil {
			return
		}

		msg := mapChatMessageToDomain(cm)
		if err := handler(ctx, msg); err != nil {
			log.Printf("twitch: error en handler: %v", err)
		}
	})

	if err := conn.Connect(); err != nil {
		return fmt.Errorf("twitch: Connect: %w", err)
	}

	if err := conn.Join(a.cfg.Channels...); err != nil {
		return fmt.Errorf("twitch: Join: %w", err)
	}

	a.mu.Lock()
	a.conn = conn
	a.mu.Unlock()

	log.Printf("twitch: conectado como %s a canales %v", a.cfg.Username, a.cfg.Channels)

	<-ctx.Done()

	a.mu.Lock()
	if a.conn != nil {
		a.conn.Close()
	}
	a.mu.Unlock()

	return ctx.Err()
}

func (a *Adapter) SendMessage(ctx context.Context, platform domain.Platform, channelID, text string) error {
	if platform != domain.PlatformTwitch {
		return fmt.Errorf("twitch adapter no soporta plataforma %s", platform)
	}

	a.mu.RLock()
	conn := a.conn
	a.mu.RUnlock()

	if conn == nil || !conn.IsConnected() {
		return errors.New("twitch: conexión no inicializada o cerrada")
	}

	log.Printf("Twitch -> Say(%s): %s", channelID, text)
	return conn.Say(channelID, text)
}

func mapChatMessageToDomain(cm irc.ChatMessage) domain.Message {
	sender := cm.Sender

	return domain.Message{
		Platform: domain.PlatformTwitch,
		// ChannelID: strconv.FormatInt(cm.ChannelID, 10),
		ChannelID: cm.Channel,
		UserID:    strconv.FormatInt(sender.ID, 10),
		Username:  sender.DisplayName,
		Text:      cm.Text,

		IsPrivate: false,

		IsPlatformOwner: sender.IsBroadcaster,
		IsPlatformAdmin: sender.IsBroadcaster || sender.IsModerator,
		IsPlatformMod:   sender.IsModerator,
		IsPlatformVip:   sender.IsVIP,
	}
}
