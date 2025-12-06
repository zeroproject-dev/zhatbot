package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	TwitchUsername string
	TwitchToken    string
	TwitchChannels []string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		TwitchUsername: os.Getenv("TWITCH_BOT_USERNAME"),
		TwitchToken:    os.Getenv("TWITCH_BOT_ACCESS_TOKEN"),
		TwitchChannels: []string{os.Getenv("TWITCH_BOT_CHANNELS")},
	}

	if cfg.TwitchUsername == "" || cfg.TwitchToken == "" {
		log.Println("Advertencia: No se encontraron variables necesarias de Twitch")
	}

	return cfg, nil
}
