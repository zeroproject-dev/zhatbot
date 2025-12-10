package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"zhatBot/internal/domain"
	"zhatBot/internal/infrastructure/config"
	kickinfra "zhatBot/internal/infrastructure/platform/kick"
	twitchinfra "zhatBot/internal/infrastructure/platform/twitch"
	kickadapter "zhatBot/internal/interface/adapters/kick"
	twitchadapter "zhatBot/internal/interface/adapters/twitch"
	"zhatBot/internal/interface/outs"
	"zhatBot/internal/usecase/commands"
	"zhatBot/internal/usecase/handle_message"
	"zhatBot/internal/usecase/stream"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c, _ := config.Load()

	cfg := twitchadapter.Config{
		Username:   c.TwitchUsername,
		OAuthToken: c.TwitchToken,
		Channels:   c.TwitchChannels,
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

	twitchTitleSvc := twitchinfra.NewTwitchTitleAdapter(
		twitchChannelSvc,
		c.TwitchBroadcasterId,
	)

	// kickinfra.NewStreamService espera (KickStreamServiceConfig) y devuelve (svc, error).
	kickSvc, err := kickinfra.NewStreamService(
		kickinfra.KickStreamServiceConfig{
			AccessToken: os.Getenv("KICK_BOT_TOKEN"),
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

	kickCfg := kickadapter.Config{
		AccessToken:       os.Getenv("KICK_BOT_TOKEN"),
		BroadcasterUserID: broadcasterID,
		ChatroomID:        chatroomID,
	}

	kickAd := kickadapter.NewAdapter(kickCfg)

	multiOut := outs.NewMultiSender()
	multiOut.Register(domain.PlatformTwitch, twitchAd)
	multiOut.Register(domain.PlatformKick, kickAd)

	uc := handle_message.NewInteractor(multiOut, router)

	twitchAd.SetHandler(uc.Handle)
	kickAd.SetHandler(uc.Handle)

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
