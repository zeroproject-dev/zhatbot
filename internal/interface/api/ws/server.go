package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
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

	httpSrv *http.Server
}

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
func NewServer(addr string) *Server {
	return &Server{
		addr: addr,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		clients: make(map[*wsClient]struct{}),
	}
}

// Start levanta el HTTP server y se bloquea hasta que el contexto se cancela.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/chat", s.handleWS)

	srv := &http.Server{
		Addr:    s.addr,
		Handler: mux,
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

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
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

	go s.waitForClose(client)
}

func (s *Server) waitForClose(client *wsClient) {
	defer func() {
		client.conn.Close()

		s.mu.Lock()
		delete(s.clients, client)
		clientCount := len(s.clients)
		s.mu.Unlock()

		log.Printf("ws: conexión cerrada (%d clientes activos)", clientCount)
	}()

	for {
		if _, _, err := client.conn.ReadMessage(); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				log.Printf("ws: read error: %v", err)
			}
			return
		}
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
