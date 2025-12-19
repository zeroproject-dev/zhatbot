package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nicklaw5/helix/v2"

	"zhatBot/internal/app"
	"zhatBot/internal/domain"
	"zhatBot/internal/infrastructure/config"
	sqlitestorage "zhatBot/internal/infrastructure/persistence/sqlite"
	twitchinfra "zhatBot/internal/infrastructure/platform/twitch"
	twitchadapter "zhatBot/internal/interface/adapters/twitch"
	ws "zhatBot/internal/interface/api/ws"
	"zhatBot/internal/interface/outs"
	categoryusecase "zhatBot/internal/usecase/category"
	"zhatBot/internal/usecase/commands"
	credentialsusecase "zhatBot/internal/usecase/credentials"
	"zhatBot/internal/usecase/handle_message"
	"zhatBot/internal/usecase/stream"
	ttsusecase "zhatBot/internal/usecase/tts"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, _ := config.Load()

	dbPath := cfg.DatabasePath
	if dbPath == "" {
		dbPath = "data/zhatbot.db"
	}

	credStore, err := sqlitestorage.NewCredentialStore(dbPath)
	if err != nil {
		log.Fatalf("no se pudo iniciar SQLite: %v", err)
	}
	defer credStore.Close()

	categorySvc := categoryusecase.NewService(categoryusecase.Config{})
	resolver := stream.NewResolver(nil, nil)
	multiOut := outs.NewMultiSender()

	platformMgr := app.NewPlatformManager(app.ManagerConfig{
		Context:  ctx,
		Category: categorySvc,
		Resolver: resolver,
		MultiOut: multiOut,
		Kick: app.KickConfig{
			BroadcasterUserID: envInt("KICK_BROADCASTER_USER_ID"),
			ChatroomID:        envInt("KICK_CHATROOM_ID"),
		},
	})
	defer platformMgr.Shutdown()

	refresher := credentialsusecase.NewRefresher(
		credStore,
		credentialsusecase.TwitchConfig{
			ClientID:     cfg.TwitchClientId,
			ClientSecret: cfg.TwitchClientSecret,
		},
		credentialsusecase.KickConfig{
			ClientID:     cfg.KickClientID,
			ClientSecret: cfg.KickClientSecret,
			RedirectURI:  cfg.KickRedirectURI,
		},
	)
	refresher.RegisterHook(platformMgr.HandleCredentialUpdate)

	if err := refresher.RefreshAll(ctx); err != nil {
		log.Printf("error refrescando tokens: %v", err)
	}

	const refreshInterval = 1 * time.Hour
	refresher.Start(ctx, refreshInterval)

	if cred, err := credStore.Get(ctx, domain.PlatformTwitch, "bot"); err == nil && cred != nil && cred.AccessToken != "" {
		cfg.TwitchToken = cred.AccessToken
	} else if err != nil {
		log.Printf("error obteniendo token de Twitch bot desde DB: %v", err)
	}

	if cred, err := credStore.Get(ctx, domain.PlatformTwitch, "streamer"); err == nil && cred != nil {
		if cred.AccessToken != "" {
			cfg.TwitchApiToken = cred.AccessToken
		}
		if cred.RefreshToken != "" {
			cfg.TwitchApiRefreshToken = cred.RefreshToken
		}
	} else if err != nil {
		log.Printf("error obteniendo token de Twitch streamer desde DB: %v", err)
	}

	twitchCfg := twitchadapter.Config{
		Username:   cfg.TwitchUsername,
		OAuthToken: formatTwitchOAuthToken(cfg.TwitchToken),
		Channels:   cfg.TwitchChannels,
	}

	wsAddr := os.Getenv("CHAT_WS_ADDR")
	if wsAddr == "" {
		wsAddr = ":8080"
	}

	wsConfig := ws.Config{
		Addr:            wsAddr,
		CredentialRepo:  credStore,
		CredentialHook:  platformMgr.HandleCredentialUpdate,
		CategoryManager: categorySvc,
	}

	if cfg.TwitchClientId != "" && cfg.TwitchClientSecret != "" && cfg.TwitchRedirectURI != "" {
		wsConfig.Twitch = &ws.TwitchOAuthConfig{
			ClientID:       cfg.TwitchClientId,
			ClientSecret:   cfg.TwitchClientSecret,
			RedirectURI:    cfg.TwitchRedirectURI,
			BotScopes:      []string{"chat:read", "chat:edit"},
			StreamerScopes: []string{"channel:manage:broadcast"},
		}
	}

	if cfg.KickClientID != "" && cfg.KickClientSecret != "" && cfg.KickRedirectURI != "" {
		wsConfig.Kick = &ws.KickOAuthConfig{
			ClientID:       cfg.KickClientID,
			ClientSecret:   cfg.KickClientSecret,
			RedirectURI:    cfg.KickRedirectURI,
			StreamerScopes: []string{"user:read", "channel:read", "channel:write", "chat:write"},
		}
	}

	wsServer := ws.NewServer(wsConfig)

	var twitchTitleSvc domain.StreamTitleService
	if cfg.TwitchClientId != "" && cfg.TwitchApiToken != "" {
		twitchAPIService, err := twitchinfra.NewStreamService(cfg.TwitchClientId, cfg.TwitchApiToken)
		if err != nil {
			log.Printf("no se pudo iniciar el servicio de Twitch: %v", err)
		} else {
			broadcasterID, err := resolveTwitchBroadcasterID(ctx, cfg.TwitchClientId, cfg.TwitchApiToken, cfg.TwitchUsername)
			if err != nil {
				log.Printf("no pude resolver el ID de Twitch: %v", err)
			} else {
				categorySvc.SetTwitchService(twitchAPIService, broadcasterID)
				twitchTitleSvc = twitchinfra.NewTwitchTitleAdapter(twitchAPIService, broadcasterID)
			}
		}
	}

	if twitchTitleSvc != nil {
		resolver.Set(domain.PlatformTwitch, twitchTitleSvc)
	}

	customManager, err := commands.NewCustomCommandManager(ctx, credStore)
	if err != nil {
		log.Fatalf("no pude iniciar el gestor de comandos: %v", err)
	}

	router := commands.NewRouter("!")
	router.SetCustomManager(customManager)
	router.Register(commands.NewPingCommand())
	router.Register(commands.NewManageCustomCommand(customManager))

	ttsService := ttsusecase.NewService(credStore, wsServer, filepath.Join("data", "tts"))
	wsServer.SetTTSManager(ttsService)
	router.Register(commands.NewTTSCommand(ttsService))

	router.Register(commands.NewTitleCommand(resolver))

	var twitchAd *twitchadapter.Adapter
	if twitchCfg.Username == "" || twitchCfg.OAuthToken == "" {
		log.Println("twitch: adaptador deshabilitado hasta que completes el login del bot.")
	} else {
		twitchAd = twitchadapter.NewAdapter(twitchCfg)
		multiOut.Register(domain.PlatformTwitch, twitchAd)
	}

	uc := handle_message.NewInteractor(multiOut, router)

	dispatch := func(ctx context.Context, msg domain.Message) error {
		msgNormalized := msg

		if msgNormalized.ChannelID == "" {
			switch msgNormalized.Platform {
			case domain.PlatformTwitch:
				if len(twitchCfg.Channels) > 0 {
					msgNormalized.ChannelID = twitchCfg.Channels[0]
				}
			case domain.PlatformKick:
				msgNormalized.ChannelID = platformMgr.ChannelID(domain.PlatformKick)
			}
		}

		if msgNormalized.Username == "" {
			msgNormalized.Username = "web-user"
		}

		if err := wsServer.PublishMessage(ctx, msgNormalized); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("ws publish error: %v", err)
		}

		return uc.Handle(ctx, msgNormalized)
	}

	wsServer.SetHandler(dispatch)
	platformMgr.SetHandler(dispatch)
	if twitchAd != nil {
		twitchAd.SetHandler(dispatch)
	}

	go func() {
		log.Printf("Iniciando servidor WS")
		if err := wsServer.Start(ctx); err != nil && err != context.Canceled {
			log.Printf("ws server error: %v", err)
		}
	}()

	log.Println("Iniciando bot...")

	if twitchAd != nil {
		go func() {
			if err := twitchAd.Start(ctx); err != nil && err != context.Canceled {
				log.Printf("twitch adapter error: %v", err)
			}
		}()
	}

	handleCredentialSnapshot(ctx, platformMgr, credStore)

	<-ctx.Done()

	log.Println("Bot apagado.")
}

func envInt(key string) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("%s inválido (%q)", key, v)
		return 0
	}
	return n
}

func handleCredentialSnapshot(ctx context.Context, mgr *app.PlatformManager, repo domain.CredentialRepository) {
	if mgr == nil || repo == nil {
		return
	}
	creds, err := repo.List(ctx)
	if err != nil {
		log.Printf("snapshot credentials error: %v", err)
		return
	}
	for _, cred := range creds {
		if cred == nil {
			continue
		}
		mgr.HandleCredentialUpdate(ctx, cred)
	}
}

func formatTwitchOAuthToken(token string) string {
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, "oauth:") {
		return token
	}
	return "oauth:" + token
}

func resolveTwitchBroadcasterID(ctx context.Context, clientID, accessToken, username string) (string, error) {
	if strings.TrimSpace(clientID) == "" {
		return "", fmt.Errorf("twitch client id vacío")
	}
	if strings.TrimSpace(accessToken) == "" {
		return "", fmt.Errorf("twitch access token vacío")
	}
	if strings.TrimSpace(username) == "" {
		return "", fmt.Errorf("twitch username vacío")
	}

	client, err := helix.NewClient(&helix.Options{
		ClientID:        clientID,
		UserAccessToken: accessToken,
	})
	if err != nil {
		return "", fmt.Errorf("helix: NewClient: %w", err)
	}

	resp, err := client.GetUsers(&helix.UsersParams{
		Logins: []string{username},
	})
	if err != nil {
		return "", fmt.Errorf("helix: GetUsers: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("helix: GetUsers failed (%d: %s) %s",
			resp.StatusCode, resp.Error, resp.ErrorMessage)
	}

	if len(resp.Data.Users) == 0 {
		return "", fmt.Errorf("usuario de Twitch no encontrado: %s", username)
	}

	return resp.Data.Users[0].ID, nil
}
