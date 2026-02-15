package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"zhatBot/internal/domain"
)

func (a *App) startLocalHTTPServer() error {
	a.httpMu.Lock()
	defer a.httpMu.Unlock()

	if a.httpServer != nil {
		return nil
	}
	if a.runtime == nil {
		return fmt.Errorf("runtime unavailable")
	}

	cfg := a.runtime.Config()
	if cfg == nil {
		return fmt.Errorf("config unavailable")
	}

	port := preferredHTTPPort(cfg)
	listener, boundPort, err := listenLoopbackWithFallback(port)
	if err != nil {
		return err
	}

	assets, err := fs.Sub(embedded, "appassets")
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("assets not available: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/callback/", func(w http.ResponseWriter, r *http.Request) {
		provider := strings.TrimPrefix(r.URL.Path, "/oauth/callback/")
		switch strings.ToLower(strings.TrimSpace(provider)) {
		case string(domain.PlatformTwitch):
			a.handleOAuthCallback(r.Context(), domain.PlatformTwitch, w, r)
		case string(domain.PlatformKick):
			a.handleOAuthCallback(r.Context(), domain.PlatformKick, w, r)
		default:
			writeOAuthHTML(w, false, "Proveedor inválido.")
		}
	})
	mux.Handle("/", spaHandler(assets))

	server := &http.Server{
		Handler: mux,
	}

	a.httpServer = server
	a.httpListener = listener
	a.httpBaseURL = fmt.Sprintf("http://localhost:%d", boundPort)

	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("desktop http server error: %v", err)
		}
	}()

	log.Printf("desktop http server running at %s", a.httpBaseURL)
	return nil
}

func (a *App) stopLocalHTTPServer() {
	a.httpMu.Lock()
	defer a.httpMu.Unlock()

	if a.httpServer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = a.httpServer.Shutdown(ctx)
	a.httpServer = nil
	a.httpListener = nil
	a.httpBaseURL = ""
}

func spaHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		if entry, err := fs.Stat(assets, path); err == nil && !entry.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		r2 := r.Clone(r.Context())
		r2.URL.Path = "/index.html"
		fileServer.ServeHTTP(w, r2)
	})
}
