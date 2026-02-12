// Package twitchadapter adapter for twitch
package twitchadapter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/adeithe/go-twitch/irc"
	"github.com/nicklaw5/helix/v2"

	"zhatBot/internal/app/emotes"
	"zhatBot/internal/domain"
)

type Config struct {
	Username          string
	OAuthToken        string
	Channels          []string
	UserNoticeHandler UserNoticeHandler
	EmoteManager      *emotes.Manager
	HelixClient       *helix.Client
}

type (
	MessageHandler    func(ctx context.Context, msg domain.Message) error
	UserNoticeHandler func(irc.UserNotice)
)

type Adapter struct {
	cfg     Config
	handler MessageHandler

	mu        sync.RWMutex
	conn      *irc.Conn
	emotes    *emotes.Manager
	helix     *helix.Client
	channelID sync.Map // login(lower) -> broadcaster id
}

func NewAdapter(cfg Config) *Adapter {
	return &Adapter{
		cfg:    cfg,
		emotes: cfg.EmoteManager,
		helix:  cfg.HelixClient,
	}
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
		log.Printf("[Twitch] %s: %s - %s", cm.Sender.DisplayName, cm.Text, cm.IRCMessage.Raw)

		a.mu.RLock()
		handler := a.handler
		a.mu.RUnlock()
		if handler == nil {
			return
		}

		msg := a.mapChatMessageToDomain(ctx, cm)
		if err := handler(ctx, msg); err != nil {
			log.Printf("twitch: error en handler: %v", err)
		}
	})
	if a.cfg.UserNoticeHandler != nil {
		conn.OnChannelUserNotice(func(notice irc.UserNotice) {
			a.cfg.UserNoticeHandler(notice)
		})
	}

	if err := conn.Connect(); err != nil {
		return fmt.Errorf("twitch: Connect: %w", err)
	}

	if err := conn.Join(a.cfg.Channels...); err != nil {
		return fmt.Errorf("twitch: Join: %w", err)
	}
	for _, ch := range a.cfg.Channels {
		log.Printf("twitch: joined channel %s", ch)
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

func (a *Adapter) mapChatMessageToDomain(ctx context.Context, cm irc.ChatMessage) domain.Message {
	sender := cm.Sender
	tokens := a.buildMessageTokens(ctx, cm)

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
		IsSubscriber:    sender.IsSubscriber,
		Tokens:          tokens,
	}
}

func (a *Adapter) buildMessageTokens(ctx context.Context, cm irc.ChatMessage) []domain.MessageToken {
	text := cm.Text
	tokens := tokenizeNativeEmotes(text, cm.IRCMessage.Tags["emotes"])
	if a.emotes == nil {
		return tokens
	}
	info := emotes.ChannelInfo{
		TwitchID:    a.resolveChannelID(ctx, cm),
		TwitchLogin: trimChannel(cm.Channel),
	}
	return a.emotes.ResolveThirdPartyTokens(ctx, tokens, info)
}

func (a *Adapter) resolveChannelID(ctx context.Context, cm irc.ChatMessage) string {
	if id := strings.TrimSpace(cm.IRCMessage.Tags["room-id"]); id != "" {
		return id
	}
	if cm.ChannelID > 0 {
		return strconv.FormatInt(cm.ChannelID, 10)
	}

	login := strings.ToLower(trimChannel(cm.Channel))
	if login == "" {
		return ""
	}

	if cached, ok := a.channelID.Load(login); ok {
		if id, ok := cached.(string); ok && id != "" {
			return id
		}
	}

	client := a.helix
	if client == nil {
		return ""
	}

	resp, err := client.GetUsers(&helix.UsersParams{
		Logins: []string{login},
	})
	if err != nil {
		log.Printf("twitch: helix get users (%s) failed: %v", login, err)
		return ""
	}
	if resp.ErrorMessage != "" {
		log.Printf("twitch: helix get users (%s) error: %s", login, resp.ErrorMessage)
		return ""
	}
	if len(resp.Data.Users) == 0 {
		return ""
	}

	id := strings.TrimSpace(resp.Data.Users[0].ID)
	if id != "" {
		a.channelID.Store(login, id)
	}
	return id
}

func trimChannel(channel string) string {
	channel = strings.TrimSpace(channel)
	return strings.TrimPrefix(channel, "#")
}
