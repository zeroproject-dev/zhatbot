package app

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"zhatBot/internal/domain"
	kickinfra "zhatBot/internal/infrastructure/platform/kick"
	kickadapter "zhatBot/internal/interface/adapters/kick"
	"zhatBot/internal/interface/outs"
	categoryusecase "zhatBot/internal/usecase/category"
	"zhatBot/internal/usecase/stream"
)

type MessageHandler func(ctx context.Context, msg domain.Message) error

type ManagerConfig struct {
	Context  context.Context
	Category *categoryusecase.Service
	Resolver *stream.Resolver
	MultiOut *outs.MultiSender
	Kick     KickConfig
}

type KickConfig struct {
	BroadcasterUserID int
	ChatroomID        int
}

type PlatformManager struct {
	ctx      context.Context
	category *categoryusecase.Service
	resolver *stream.Resolver
	multiOut *outs.MultiSender

	handlerMu sync.RWMutex
	handler   MessageHandler

	kickCfg KickConfig

	mu   sync.RWMutex
	kick *kickRuntime
}

type kickRuntime struct {
	cancel    context.CancelFunc
	adapter   *kickadapter.Adapter
	streamSvc domain.KickStreamService
	rawSvc    *kickinfra.KickStreamService
	channelID string
}

func NewPlatformManager(cfg ManagerConfig) *PlatformManager {
	ctx := cfg.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return &PlatformManager{
		ctx:      ctx,
		category: cfg.Category,
		resolver: cfg.Resolver,
		multiOut: cfg.MultiOut,
		kickCfg:  cfg.Kick,
	}
}

func (m *PlatformManager) SetHandler(handler MessageHandler) {
	m.handlerMu.Lock()
	m.handler = handler
	m.handlerMu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.kick != nil && handler != nil {
		m.kick.adapter.SetHandler(adaptKickHandler(handler))
	}
}

func (m *PlatformManager) ChannelID(platform domain.Platform) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	switch platform {
	case domain.PlatformKick:
		if m.kick != nil {
			return m.kick.channelID
		}
	default:
	}
	return ""
}

func (m *PlatformManager) HandleCredentialUpdate(ctx context.Context, cred *domain.Credential) {
	if cred == nil {
		return
	}

	switch cred.Platform {
	case domain.PlatformKick:
		if !strings.EqualFold(strings.TrimSpace(cred.Role), "streamer") {
			return
		}
		token := strings.TrimSpace(cred.AccessToken)
		if token == "" {
			m.disableKick()
			return
		}
		if err := m.enableKick(token); err != nil {
			log.Printf("kick manager: no se pudo iniciar Kick: %v", err)
		}
	default:
	}
}

func (m *PlatformManager) Shutdown() {
	m.disableKick()
}

func (m *PlatformManager) enableKick(token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.kick != nil {
		m.kick.adapter.UpdateAccessToken(token)
		if m.kick.rawSvc != nil {
			m.kick.rawSvc.UpdateAccessToken(token)
		}
		return nil
	}

	if m.kickCfg.BroadcasterUserID == 0 || m.kickCfg.ChatroomID == 0 {
		return fmt.Errorf("kick manager: faltan KICK_BROADCASTER_USER_ID o KICK_CHATROOM_ID")
	}

	streamSvcIface, err := kickinfra.NewStreamService(
		kickinfra.KickStreamServiceConfig{
			AccessToken: token,
		},
	)
	if err != nil {
		return fmt.Errorf("kick manager: %w", err)
	}

	rawSvc, _ := streamSvcIface.(*kickinfra.KickStreamService)

	adapter := kickadapter.NewAdapter(kickadapter.Config{
		AccessToken:       token,
		BroadcasterUserID: m.kickCfg.BroadcasterUserID,
		ChatroomID:        m.kickCfg.ChatroomID,
	})

	multiOut := m.multiOut
	if multiOut != nil {
		multiOut.Register(domain.PlatformKick, adapter)
	}
	if m.resolver != nil {
		m.resolver.Set(domain.PlatformKick, streamSvcIface)
	}
	if m.category != nil {
		m.category.SetKickService(streamSvcIface)
	}

	handler := m.getHandler()
	if handler != nil {
		adapter.SetHandler(adaptKickHandler(handler))
	}

	ctx, cancel := context.WithCancel(m.ctx)
	go func() {
		if err := adapter.Start(ctx); err != nil && err != context.Canceled {
			log.Printf("kick manager: adapter terminó con error: %v", err)
		}
	}()

	m.kick = &kickRuntime{
		cancel:    cancel,
		adapter:   adapter,
		streamSvc: streamSvcIface,
		rawSvc:    rawSvc,
		channelID: strconv.Itoa(m.kickCfg.ChatroomID),
	}

	log.Println("kick manager: Kick habilitado.")
	return nil
}

func (m *PlatformManager) disableKick() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.kick == nil {
		return
	}
	m.kick.cancel()
	if m.multiOut != nil {
		m.multiOut.Unregister(domain.PlatformKick)
	}
	if m.resolver != nil {
		m.resolver.Set(domain.PlatformKick, nil)
	}
	if m.category != nil {
		m.category.SetKickService(nil)
	}
	m.kick = nil
	log.Println("kick manager: Kick deshabilitado.")
}

func (m *PlatformManager) getHandler() MessageHandler {
	m.handlerMu.RLock()
	defer m.handlerMu.RUnlock()
	return m.handler
}

func adaptKickHandler(handler MessageHandler) kickadapter.MessageHandler {
	if handler == nil {
		return nil
	}
	return func(ctx context.Context, msg domain.Message) error {
		return handler(ctx, msg)
	}
}
