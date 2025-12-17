package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	TwitchUsername        string
	TwitchToken           string
	TwitchChannels        []string
	TwitchApiToken        string
	TwitchClientSecret    string
	TwitchClientId        string
	TwitchApiRefreshToken string
	TwitchRedirectURI     string

	KickClientID     string
	KickClientSecret string
	KickRedirectURI  string

	DatabasePath string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		TwitchUsername:        os.Getenv("TWITCH_BOT_USERNAME"),
		TwitchToken:           os.Getenv("TWITCH_BOT_ACCESS_TOKEN"),
		TwitchChannels:        []string{os.Getenv("TWITCH_BOT_CHANNELS")},
		TwitchApiToken:        os.Getenv("TWITCH_API_ACCESS_TOKEN"),
		TwitchClientSecret:    os.Getenv("TWITCH_CLIENT_SECRET"),
		TwitchClientId:        os.Getenv("TWITCH_CLIENT_ID"),
		TwitchApiRefreshToken: os.Getenv("TWITCH_API_REFRESH_TOKEN"),
		TwitchRedirectURI:     os.Getenv("TWITCH_REDIRECT_URI"),

		KickClientID:     os.Getenv("KICK_CLIENT_ID"),
		KickClientSecret: os.Getenv("KICK_CLIENT_SECRET"),
		KickRedirectURI:  os.Getenv("KICK_REDIRECT_URI"),

		DatabasePath: os.Getenv("DATABASE_PATH"),
	}

	if cfg.TwitchUsername == "" {
		log.Println("Advertencia: TWITCH_BOT_USERNAME no configurado")
	}

	return cfg, nil
}
