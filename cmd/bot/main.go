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

	"zhatBot/internal/domain"
	"zhatBot/internal/infrastructure/config"
	sqlitestorage "zhatBot/internal/infrastructure/persistence/sqlite"
	kickinfra "zhatBot/internal/infrastructure/platform/kick"
	twitchinfra "zhatBot/internal/infrastructure/platform/twitch"
	kickadapter "zhatBot/internal/interface/adapters/kick"
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

	const refreshInterval = 1 * time.Hour
	refresher.Start(ctx, refreshInterval)

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

	kickAccessToken := ""
	if cred, err := credStore.Get(ctx, domain.PlatformKick, "streamer"); err == nil && cred != nil && cred.AccessToken != "" {
		kickAccessToken = cred.AccessToken
	} else if err != nil {
		log.Printf("error obteniendo token de Kick streamer desde DB: %v", err)
	}
	if kickAccessToken == "" {
		log.Println("kick: no hay token de streamer almacenado. Inicia sesión desde el panel web (rol streamer).")
		log.Println("kick: si necesitas el nuevo scope chat:write, revoca la app en Kick (Settings > Connections) y vuelve a autorizar.")
		log.Fatal("kick: token de streamer ausente")
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
	twitchStreamSvc, _ := twitchChannelSvc.(*twitchinfra.TwitchStreamService)

	broadcasterID, err := resolveTwitchBroadcasterID(ctx, c.TwitchClientId, c.TwitchApiToken, c.TwitchUsername)
	if err != nil {
		log.Fatalf("no pude resolver el ID de Twitch: %v", err)
	}

	twitchTitleSvc := twitchinfra.NewTwitchTitleAdapter(
		twitchChannelSvc,
		broadcasterID,
	)

	// kickinfra.NewStreamService espera (KickStreamServiceConfig) y devuelve (svc, error).
	// if kickStreamToken == "" {
	// 	log.Fatal("No hay token de Kick disponible para actualizar el título")
	// }

	kickSvc, err := kickinfra.NewStreamService(
		kickinfra.KickStreamServiceConfig{
			AccessToken: kickAccessToken,
		},
	)
	if err != nil {
		log.Fatalf("error creando KickStreamService: %v", err)
	}
	kickStreamSvc, _ := kickSvc.(*kickinfra.KickStreamService)

	categorySvc := categoryusecase.NewService(categoryusecase.Config{
		Twitch:              twitchChannelSvc,
		TwitchBroadcasterID: broadcasterID,
		Kick:                kickSvc,
	})
	wsConfig.CategoryManager = categorySvc

	var kickAd *kickadapter.Adapter

	refresher.RegisterHook(func(ctx context.Context, cred *domain.Credential) {
		if cred == nil {
			return
		}
		switch cred.Platform {
		case domain.PlatformTwitch:
			if strings.EqualFold(cred.Role, "streamer") && twitchStreamSvc != nil {
				twitchStreamSvc.UpdateAccessToken(cred.AccessToken)
			}
		case domain.PlatformKick:
			if !strings.EqualFold(strings.TrimSpace(cred.Role), "streamer") {
				log.Printf("token refresher: rol Kick inesperado %q ignorado", cred.Role)
				return
			}
			if kickStreamSvc != nil {
				kickStreamSvc.UpdateAccessToken(cred.AccessToken)
			}
			if kickAd != nil {
				kickAd.UpdateAccessToken(cred.AccessToken)
			}
		}
	})

	wsServer := ws.NewServer(wsConfig)

	// ---------- 2) Resolver de servicios por plataforma ----------

	resolver := stream.NewResolver(twitchTitleSvc, kickSvc)

	// ---------- 3) Router de comandos ----------

	customManager, err := commands.NewCustomCommandManager(ctx, credStore)
	if err != nil {
		log.Fatalf("no pude iniciar el gestor de comandos: %v", err)
	}

	router := commands.NewRouter("!")
	router.SetCustomManager(customManager)

	// Comandos genéricos
	router.Register(commands.NewPingCommand())
	router.Register(commands.NewManageCustomCommand(customManager))
	ttsService := ttsusecase.NewService(credStore, wsServer, filepath.Join("data", "tts"))
	wsServer.SetTTSManager(ttsService)
	router.Register(commands.NewTTSCommand(ttsService))

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

	kickBroadcasterID, err := strconv.Atoi(os.Getenv("KICK_BROADCASTER_USER_ID"))
	if err != nil {
		log.Fatalf("KICK_BROADCASTER_USER_ID inválido")
	}

	chatroomID, err := strconv.Atoi(os.Getenv("KICK_CHATROOM_ID"))
	if err != nil {
		log.Fatalf("KICK_CHATROOM_ID inválido")
	}

	kickCfg := kickadapter.Config{
		AccessToken:       kickAccessToken,
		BroadcasterUserID: kickBroadcasterID,
		ChatroomID:        chatroomID,
	}

	kickAd = kickadapter.NewAdapter(kickCfg)

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

	go func() {
		log.Printf("Iniciando servidor WS")
		if err := wsServer.Start(ctx); err != nil && err != context.Canceled {
			log.Printf("ws server error: %v", err)
		}
	}()

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
