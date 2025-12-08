// cmd/bot/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"zhatBot/internal/infrastructure/config"
	twitchinfra "zhatBot/internal/infrastructure/platform/twitch"
	twitchadapter "zhatBot/internal/interface/adapters/twitch"
	"zhatBot/internal/usecase/commands"
	"zhatBot/internal/usecase/handle_message"
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

	twitchChannelSvc, err := twitchinfra.NewHelixChannelService(c.TwitchClientId, c.TwitchApiToken)
	if err != nil {
		log.Fatalf("error creando HelixChannelService: %v", err)
	}

	router := commands.NewRouter("!")

	router.Register(commands.NewPingCommand())
	router.Register(commands.NewTitleCommand(twitchChannelSvc, c.TwitchBroadcasterId))
	// aquí luego: router.Register(commands.NewHelpCommand(...))
	// router.Register(commands.NewBanCommand(...))

	if cfg.Username == "" || cfg.OAuthToken == "" {
		log.Fatal("TWITCH_BOT_USERNAME o TWITCH_BOT_OAUTH_TOKEN no configurados")
	}

	twitchAd := twitchadapter.NewAdapter(cfg)

	uc := handle_message.NewInteractor(twitchAd, router)

	twitchAd.SetHandler(uc.Handle)

	log.Println("Iniciando bot de Twitch...")

	if err := twitchAd.Start(ctx); err != nil && err != context.Canceled {
		log.Fatalf("twitch: error en Start: %v", err)
	}

	log.Println("Bot apagado.")
}
