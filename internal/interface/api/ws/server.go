package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"zhatBot/internal/domain"
)

// Server expone un endpoint WebSocket y retransmite cada domain.Message como JSON.
type Server struct {
	addr     string
	upgrader websocket.Upgrader

	mu      sync.RWMutex
	clients map[*wsClient]struct{}
	handler MessageHandler

	httpSrv *http.Server
	api     *apiHandlers
}

type MessageHandler func(ctx context.Context, msg domain.Message) error

type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *wsClient) writeJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(v)
}

// NewServer crea un servidor WebSocket escuchando en addr (ej. ":8080").
func NewServer(cfg Config) *Server {
	server := &Server{
		addr: cfg.addr(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		clients: make(map[*wsClient]struct{}),
		api:     newAPIHandlers(cfg),
	}

	return server
}

// Start levanta el HTTP server y se bloquea hasta que el contexto se cancela.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/chat", func(w http.ResponseWriter, r *http.Request) {
		s.handleWS(ctx, w, r)
	})
	if s.api != nil {
		s.api.register(mux)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			setCORSHeaders(w)
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:    s.addr,
		Handler: handler,
	}

	s.mu.Lock()
	s.httpSrv = srv
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("ws: shutdown error: %v", err)
		}
	}()

	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

func (s *Server) handleWS(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws: upgrade error: %v", err)
		return
	}

	client := &wsClient{conn: conn}

	s.mu.Lock()
	s.clients[client] = struct{}{}
	clientCount := len(s.clients)
	s.mu.Unlock()

	log.Printf("ws: nueva conexión desde %s (%d clientes activos)", r.RemoteAddr, clientCount)

	go s.handleClient(ctx, client)
}

func (s *Server) handleClient(ctx context.Context, client *wsClient) {
	defer func() {
		client.conn.Close()

		s.mu.Lock()
		delete(s.clients, client)
		clientCount := len(s.clients)
		s.mu.Unlock()

		log.Printf("ws: conexión cerrada (%d clientes activos)", clientCount)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgType, data, err := client.conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				log.Printf("ws: read error: %v", err)
			}
			return
		}

		if msgType != websocket.TextMessage {
			continue
		}

		if err := s.dispatchIncoming(ctx, data); err != nil {
			log.Printf("ws: incoming dispatch error: %v", err)
		}
	}
}

func (s *Server) dispatchIncoming(ctx context.Context, data []byte) error {
	handler := s.getHandler()
	if handler == nil {
		return nil
	}

	payload := incomingPayload{}
	if err := json.Unmarshal(data, &payload); err != nil {
		payload.Text = strings.TrimSpace(string(data))
	} else {
		payload.Text = strings.TrimSpace(payload.Text)
	}

	if payload.Text == "" {
		return fmt.Errorf("ws: empty incoming text")
	}

	platform := normalizePlatform(payload.Platform)
	channelID := strings.TrimSpace(payload.ChannelID)
	userID := strings.TrimSpace(payload.UserID)
	username := strings.TrimSpace(payload.Username)

	if platform == "" {
		platform = domain.PlatformTwitch
	}
	if channelID == "" {
		channelID = ""
	}
	if username == "" {
		username = "web-user"
	}
	if userID == "" {
		userID = "web"
	}

	msg := domain.Message{
		Platform:        platform,
		ChannelID:       channelID,
		UserID:          userID,
		Username:        username,
		Text:            payload.Text,
		IsPrivate:       payload.IsPrivate,
		IsPlatformOwner: true,
		IsPlatformAdmin: true,
		IsPlatformMod:   true,
		IsPlatformVip:   true,
	}

	return handler(ctx, msg)
}

func (s *Server) getHandler() MessageHandler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.handler
}

func (s *Server) SetHandler(h MessageHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = h
}

// SetTTSManager allows wiring the TTS manager after server construction so the
// HTTP API can expose the related endpoints.
func (s *Server) SetTTSManager(m TTSManager) {
	if s == nil || s.api == nil {
		return
	}
	s.api.setTTSManager(m)
}

func (s *Server) SetTTSStatusProvider(p TTSStatusReporter) {
	if s == nil || s.api == nil {
		return
	}
	s.api.setTTSStatusProvider(p)
}

type incomingPayload struct {
	Text      string `json:"text"`
	Platform  string `json:"platform"`
	ChannelID string `json:"channel_id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	IsPrivate bool   `json:"is_private"`
}

func normalizePlatform(p string) domain.Platform {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case string(domain.PlatformTwitch):
		return domain.PlatformTwitch
	case string(domain.PlatformKick):
		return domain.PlatformKick
	default:
		return ""
	}
}

// PublishMessage cumple con domain.MessagePublisher enviando el payload a cada cliente WS.
func (s *Server) PublishMessage(ctx context.Context, msg domain.Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	s.mu.RLock()
	clients := make([]*wsClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	clientCount := len(clients)
	s.mu.RUnlock()

	log.Printf("ws: enviando mensaje a %d clientes", clientCount)

	for _, c := range clients {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := c.writeJSON(json.RawMessage(payload)); err != nil {
			log.Printf("ws: removing client due to write error: %v", err)
			s.mu.Lock()
			delete(s.clients, c)
			s.mu.Unlock()
			c.conn.Close()
		}
	}

	return nil
}

func (s *Server) PublishTTSEvent(ctx context.Context, event domain.TTSEvent) error {
	envelope := struct {
		Type string          `json:"type"`
		Data domain.TTSEvent `json:"data"`
	}{
		Type: "tts",
		Data: event,
	}

	payload, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	s.mu.RLock()
	clients := make([]*wsClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.RUnlock()

	for _, c := range clients {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := c.writeJSON(json.RawMessage(payload)); err != nil {
			log.Printf("ws: removing client due to write error: %v", err)
			s.mu.Lock()
			delete(s.clients, c)
			s.mu.Unlock()
			c.conn.Close()
		}
	}

	return nil
}

var _ domain.TTSEventPublisher = (*Server)(nil)
