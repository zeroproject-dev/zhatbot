// internal/interface/adapters/kick/kick_adapter.go
package kickadapter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	kicksdk "github.com/glichtv/kick-sdk"
	kickchatwrapper "github.com/johanvandegriff/kick-chat-wrapper"

	"zhatBot/internal/domain"
)

type Config struct {
	// Token del BOT de Kick (de tu flujo OAuth)
	AccessToken string

	// ID del usuario broadcaster (tu cuenta de Kick)
	BroadcasterUserID int

	// ID del chatroom (no es el mismo que el userID)
	// lo sacas de: https://kick.com/api/v2/channels/{slug}, campo "chatroom":{"id":...}
	ChatroomID int

	// EventHandler permite interceptar cualquier mensaje crudo del chatroom (subs, tips, etc.)
	EventHandler EventHandler
}

type MessageHandler func(ctx context.Context, msg domain.Message) error
type EventHandler func(msg kickchatwrapper.ChatMessage)

type Adapter struct {
	cfg     Config
	handler MessageHandler

	mu  sync.RWMutex
	sdk *kicksdk.Client
	ws  *kickchatwrapper.Client
}

func NewAdapter(cfg Config) *Adapter {
	return &Adapter{cfg: cfg}
}

func (a *Adapter) SetHandler(h MessageHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.handler = h
}

func (a *Adapter) Start(ctx context.Context) error {
	if a.cfg.AccessToken == "" {
		return errors.New("kick: AccessToken vacío")
	}
	if a.cfg.ChatroomID == 0 {
		return errors.New("kick: ChatroomID no configurado")
	}
	if a.cfg.BroadcasterUserID == 0 {
		return errors.New("kick: BroadcasterUserID no configurado")
	}

	// Cliente para enviar mensajes (REST / SDK oficial)
	sdkClient := kicksdk.NewClient(
		kicksdk.WithAccessTokens(kicksdk.AccessTokens{
			UserAccessToken: a.cfg.AccessToken,
		}),
	)

	// Cliente WebSocket para escuchar el chat
	wsClient, err := kickchatwrapper.NewClient()
	if err != nil {
		return fmt.Errorf("kick: error creando ws client: %w", err)
	}

	if err := wsClient.JoinChannelByID(a.cfg.ChatroomID); err != nil {
		return fmt.Errorf("kick: JoinChannelByID: %w", err)
	}

	msgChan := wsClient.ListenForMessages()

	a.mu.Lock()
	a.sdk = sdkClient
	a.ws = wsClient
	a.mu.Unlock()

	log.Printf("kick: conectado al chatroom %d (broadcasterUserID=%d)", a.cfg.ChatroomID, a.cfg.BroadcasterUserID)

	// Goroutine para leer mensajes del websocket y mandarlos a tu usecase
	go func() {
		for {
			select {
			case m, ok := <-msgChan:
				if !ok {
					log.Println("kick: canal de mensajes cerrado")
					return
				}

				if h := a.cfg.EventHandler; h != nil {
					go h(m)
				}

				a.mu.RLock()
				handler := a.handler
				a.mu.RUnlock()
				if handler == nil {
					continue
				}

				dmsg := mapChatMessageToDomain(m, a.cfg.BroadcasterUserID)

				if err := handler(ctx, dmsg); err != nil {
					log.Printf("kick: error en handler: %v", err)
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	// Esperar a que cierren el contexto (igual que en Twitch)
	<-ctx.Done()

	a.mu.Lock()
	if a.ws != nil {
		a.ws.Close()
	}
	a.mu.Unlock()

	return ctx.Err()
}

func (a *Adapter) SendMessage(ctx context.Context, platform domain.Platform, channelID, text string) error {
	if platform != domain.PlatformKick {
		return fmt.Errorf("kick adapter no soporta plataforma %s", platform)
	}

	a.mu.RLock()
	client := a.sdk
	a.mu.RUnlock()

	if client == nil {
		return errors.New("kick: cliente SDK no inicializado (Start no llamado o falló)")
	}
	if text == "" {
		return nil
	}
	if a.cfg.BroadcasterUserID == 0 {
		return errors.New("kick: BroadcasterUserID no configurado")
	}

	log.Printf("Kick -> Chat.PostMessage(broadcasterUserID=%d): %s", a.cfg.BroadcasterUserID, text)

	resp, err := client.Chat().PostMessage(ctx, kicksdk.PostChatMessageInput{
		BroadcasterUserID: a.cfg.BroadcasterUserID,
		Content:           text,
		PosterType:        kicksdk.MessagePosterUser,
	})
	if err != nil {
		return fmt.Errorf("kick: error enviando mensaje de chat: %w", err)
	}

	if !resp.Payload.IsSent {
		meta := resp.ResponseMetadata
		log.Printf(
			"kick: PostMessage rechazado (status=%d, message_id=%s, kick_message=%q, kick_error=%q, description=%q)",
			meta.StatusCode,
			resp.Payload.MessageID,
			meta.KickMessage,
			meta.KickError,
			meta.KickErrorDescription,
		)
		return fmt.Errorf("kick: mensaje no fue aceptado por la API (status %d)", meta.StatusCode)
	}

	log.Printf("kick: mensaje entregado (message_id=%s)", resp.Payload.MessageID)
	return nil
}

func (a *Adapter) UpdateAccessToken(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.cfg.AccessToken = token
	if a.sdk != nil {
		a.sdk = kicksdk.NewClient(
			kicksdk.WithAccessTokens(kicksdk.AccessTokens{
				UserAccessToken: token,
			}),
		)
	}
}

func mapChatMessageToDomain(m kickchatwrapper.ChatMessage, broadcasterUserID int) domain.Message {
	// TODO: log.Println(m)
	sender := m.Sender

	isOwner := sender.ID == broadcasterUserID

	var isMod, isVip bool
	for _, b := range sender.Identity.Badges {
		switch strings.ToLower(b.Type) {
		case "moderator":
			isMod = true
		case "vip":
			isVip = true
		case "broadcaster":
			// a veces Kick marca esto en badges también
			isMod = true
		}
	}

	return domain.Message{
		Platform:  domain.PlatformKick,
		ChannelID: strconv.Itoa(m.ChatroomID), // o puedes guardar el slug en Config si quieres
		UserID:    strconv.Itoa(sender.ID),
		Username:  sender.Username,
		Text:      m.Content,

		IsPrivate: false,

		IsPlatformOwner: isOwner,
		IsPlatformAdmin: isOwner || isMod,
		IsPlatformMod:   isMod,
		IsPlatformVip:   isVip,
	}
}
