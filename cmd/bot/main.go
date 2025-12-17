package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"zhatBot/internal/domain"
	"zhatBot/internal/infrastructure/config"
	sqlitestorage "zhatBot/internal/infrastructure/persistence/sqlite"
	kickinfra "zhatBot/internal/infrastructure/platform/kick"
	twitchinfra "zhatBot/internal/infrastructure/platform/twitch"
	kickadapter "zhatBot/internal/interface/adapters/kick"
	twitchadapter "zhatBot/internal/interface/adapters/twitch"
	ws "zhatBot/internal/interface/api/ws"
	"zhatBot/internal/interface/outs"
	"zhatBot/internal/usecase/commands"
	credentialsusecase "zhatBot/internal/usecase/credentials"
	"zhatBot/internal/usecase/handle_message"
	"zhatBot/internal/usecase/stream"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c, _ := config.Load()

	dbPath := c.DatabasePath
	if dbPath == "" {
		dbPath = "data/zhatbot.db"
	}

	credStore, err := sqlitestorage.NewCredentialStore(dbPath)
	if err != nil {
		log.Fatalf("no se pudo iniciar SQLite: %v", err)
	}
	defer credStore.Close()

	refresher := credentialsusecase.NewRefresher(
		credStore,
		credentialsusecase.TwitchConfig{
			ClientID:     c.TwitchClientId,
			ClientSecret: c.TwitchClientSecret,
		},
		credentialsusecase.KickConfig{
			ClientID:     c.KickClientID,
			ClientSecret: c.KickClientSecret,
			RedirectURI:  c.KickRedirectURI,
		},
	)

	if err := refresher.RefreshAll(ctx); err != nil {
		log.Printf("error refrescando tokens: %v", err)
	}

	if cred, err := credStore.Get(ctx, domain.PlatformTwitch, "bot"); err == nil && cred != nil && cred.AccessToken != "" {
		c.TwitchToken = cred.AccessToken
	} else if err != nil {
		log.Printf("error obteniendo token de Twitch bot desde DB: %v", err)
	}

	if cred, err := credStore.Get(ctx, domain.PlatformTwitch, "streamer"); err == nil && cred != nil {
		if cred.AccessToken != "" {
			c.TwitchApiToken = cred.AccessToken
		}
		if cred.RefreshToken != "" {
			c.TwitchApiRefreshToken = cred.RefreshToken
		}
	} else if err != nil {
		log.Printf("error obteniendo token de Twitch streamer desde DB: %v", err)
	}

	kickChatToken := os.Getenv("KICK_BOT_TOKEN")
	if cred, err := credStore.Get(ctx, domain.PlatformKick, "bot"); err == nil && cred != nil && cred.AccessToken != "" {
		kickChatToken = cred.AccessToken
	} else if err != nil {
		log.Printf("error obteniendo token de Kick bot desde DB: %v", err)
	}

	kickStreamToken := kickChatToken
	if cred, err := credStore.Get(ctx, domain.PlatformKick, "streamer"); err == nil && cred != nil && cred.AccessToken != "" {
		kickStreamToken = cred.AccessToken
	} else if err != nil {
		log.Printf("error obteniendo token de Kick streamer desde DB: %v", err)
	}

	cfg := twitchadapter.Config{
		Username:   c.TwitchUsername,
		OAuthToken: formatTwitchOAuthToken(c.TwitchToken),
		Channels:   c.TwitchChannels,
	}

	wsAddr := os.Getenv("CHAT_WS_ADDR")
	if wsAddr == "" {
		wsAddr = ":8080"
	}

	wsConfig := ws.Config{
		Addr:           wsAddr,
		CredentialRepo: credStore,
	}

	if c.TwitchClientId != "" && c.TwitchClientSecret != "" && c.TwitchRedirectURI != "" {
		wsConfig.Twitch = &ws.TwitchOAuthConfig{
			ClientID:       c.TwitchClientId,
			ClientSecret:   c.TwitchClientSecret,
			RedirectURI:    c.TwitchRedirectURI,
			BotScopes:      []string{"chat:read", "chat:edit"},
			StreamerScopes: []string{"channel:manage:broadcast"},
		}
	}

	if c.KickClientID != "" && c.KickClientSecret != "" && c.KickRedirectURI != "" {
		wsConfig.Kick = &ws.KickOAuthConfig{
			ClientID:       c.KickClientID,
			ClientSecret:   c.KickClientSecret,
			RedirectURI:    c.KickRedirectURI,
			BotScopes:      []string{"user:read", "channel:read", "channel:write"},
			StreamerScopes: []string{"user:read", "channel:read", "channel:write"},
		}
	}

	wsServer := ws.NewServer(wsConfig)

	go func() {
		log.Printf("Iniciando servidor WS")
		if err := wsServer.Start(ctx); err != nil && err != context.Canceled {
			log.Printf("ws server error: %v", err)
		}
	}()

	// ---------- 1) Crear servicios de stream por plataforma ----------

	// twitchinfra.NewStreamService espera (string, string) y devuelve (svc, error).
	// Normalmente serían algo tipo (clientID, accessToken).
	twitchChannelSvc, err := twitchinfra.NewStreamService(
		c.TwitchClientId,
		c.TwitchApiToken,
	)
	if err != nil {
		log.Fatal(err)
	}

	twitchTitleSvc := twitchinfra.NewTwitchTitleAdapter(
		twitchChannelSvc,
		c.TwitchBroadcasterId,
	)

	// kickinfra.NewStreamService espera (KickStreamServiceConfig) y devuelve (svc, error).
	if kickStreamToken == "" {
		log.Fatal("No hay token de Kick disponible para actualizar el título")
	}

	kickSvc, err := kickinfra.NewStreamService(
		kickinfra.KickStreamServiceConfig{
			AccessToken: kickStreamToken,
			// si tu struct tiene más campos (RefreshToken, OwnerID...), añádelos aquí.
			// RefreshToken: os.Getenv("KICK_BOT_REFRESH_TOKEN"),
		},
	)
	if err != nil {
		log.Fatalf("error creando KickStreamService: %v", err)
	}

	// ---------- 2) Resolver de servicios por plataforma ----------

	resolver := stream.NewResolver(twitchTitleSvc, kickSvc)

	// ---------- 3) Router de comandos ----------

	router := commands.NewRouter("!")

	// Comandos genéricos
	router.Register(commands.NewPingCommand())

	// Comando title (único, multi-plataforma)
	router.Register(
		commands.NewTitleCommand(
			resolver,
		),
	)

	// ---------- 4) Validar config de Twitch ----------

	if cfg.Username == "" || cfg.OAuthToken == "" {
		log.Fatal("TWITCH_BOT_USERNAME o TWITCH_BOT_OAUTH_TOKEN no configurados")
	}

	// ---------- 5) Adapter de Twitch ----------

	twitchAd := twitchadapter.NewAdapter(cfg)

	broadcasterID, err := strconv.Atoi(os.Getenv("KICK_BROADCASTER_USER_ID"))
	if err != nil {
		log.Fatalf("KICK_BROADCASTER_USER_ID inválido")
	}

	chatroomID, err := strconv.Atoi(os.Getenv("KICK_CHATROOM_ID"))
	if err != nil {
		log.Fatalf("KICK_CHATROOM_ID inválido")
	}

	if kickChatToken == "" {
		log.Fatal("No hay token de Kick disponible para el chat")
	}

	kickCfg := kickadapter.Config{
		AccessToken:       kickChatToken,
		BroadcasterUserID: broadcasterID,
		ChatroomID:        chatroomID,
	}

	kickAd := kickadapter.NewAdapter(kickCfg)

	multiOut := outs.NewMultiSender()
	multiOut.Register(domain.PlatformTwitch, twitchAd)
	multiOut.Register(domain.PlatformKick, kickAd)

	uc := handle_message.NewInteractor(multiOut, router)

	kickChannelID := strconv.Itoa(chatroomID)

	dispatch := func(ctx context.Context, msg domain.Message) error {
		msgNormalized := msg

		if msgNormalized.ChannelID == "" {
			switch msgNormalized.Platform {
			case domain.PlatformTwitch:
				if len(cfg.Channels) > 0 {
					msgNormalized.ChannelID = cfg.Channels[0]
				}
			case domain.PlatformKick:
				msgNormalized.ChannelID = kickChannelID
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
	twitchAd.SetHandler(dispatch)
	kickAd.SetHandler(dispatch)

	log.Println("Iniciando bot...")

	go func() {
		if err := twitchAd.Start(ctx); err != nil && err != context.Canceled {
			log.Printf("twitch adapter error: %v", err)
		}
	}()

	go func() {
		if err := kickAd.Start(ctx); err != nil && err != context.Canceled {
			log.Printf("kick adapter error: %v", err)
		}
	}()

	<-ctx.Done()

	log.Println("Bot apagado.")
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
