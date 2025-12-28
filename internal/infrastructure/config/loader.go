package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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

const embeddedTwitchClientID = "TWITCH_DESKTOP_CLIENT_ID"

type fileConfig struct {
	TwitchClientID     string `json:"twitch_client_id"`
	TwitchClientSecret string `json:"twitch_client_secret"`
	TwitchRedirectURI  string `json:"twitch_redirect_uri"`
	KickClientID       string `json:"kick_client_id"`
	KickRedirectURI    string `json:"kick_redirect_uri"`
	DatabasePath       string `json:"database_path"`
}

var (
	configFilePath    string
	cachedFileConfig *fileConfig
)

func Load() (*Config, error) {
	loadDevDotEnv()

	jsonCfg, err := loadJSONConfig()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		TwitchUsername:        os.Getenv("TWITCH_BOT_USERNAME"),
		TwitchToken:           os.Getenv("TWITCH_BOT_ACCESS_TOKEN"),
		TwitchChannels:        []string{os.Getenv("TWITCH_BOT_CHANNELS")},
		TwitchApiToken:        os.Getenv("TWITCH_API_ACCESS_TOKEN"),
		TwitchClientSecret:    firstNonEmpty(os.Getenv("TWITCH_CLIENT_SECRET"), jsonCfg.TwitchClientSecret),
		TwitchClientId:        firstNonEmpty(os.Getenv("TWITCH_CLIENT_ID"), jsonCfg.TwitchClientID, embeddedTwitchClientID),
		TwitchApiRefreshToken: os.Getenv("TWITCH_API_REFRESH_TOKEN"),
		TwitchRedirectURI:     firstNonEmpty(os.Getenv("TWITCH_REDIRECT_URI"), jsonCfg.TwitchRedirectURI),

		KickClientID:     firstNonEmpty(os.Getenv("KICK_CLIENT_ID"), jsonCfg.KickClientID),
		KickClientSecret: os.Getenv("KICK_CLIENT_SECRET"),
		KickRedirectURI:  firstNonEmpty(os.Getenv("KICK_REDIRECT_URI"), jsonCfg.KickRedirectURI),

		DatabasePath: firstNonEmpty(os.Getenv("DATABASE_PATH"), jsonCfg.DatabasePath),
	}

	if cfg.TwitchUsername == "" {
		log.Println("Advertencia: TWITCH_BOT_USERNAME no configurado")
	}

	return cfg, nil
}

func ConfigFilePath() string {
	return configFilePath
}

func loadDevDotEnv() {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("ZHATBOT_MODE")))
	if mode != "development" {
		return
	}

	var paths []string
	paths = append(paths, ".env")

	if exePath, err := os.Executable(); err == nil {
		if dir := filepath.Dir(exePath); dir != "" {
			paths = append(paths, filepath.Join(dir, ".env"))
		}
	}

	if dir := configDir(); dir != "" {
		paths = append(paths, filepath.Join(dir, ".env"))
	}

	for _, p := range paths {
		loadDotEnvIfExists(p)
	}
}

func loadDotEnvIfExists(path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err != nil {
		return
	}
	if err := godotenv.Load(path); err != nil {
		log.Printf("warning: could not load %s: %v", path, err)
	}
}

func ensureTemplateConfig(path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err == nil {
		return
	}
	dir := filepath.Dir(path)
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	template := fileConfig{
		TwitchClientSecret: "",
		TwitchRedirectURI:  "http://localhost:17833/oauth/callback/twitch",
		KickRedirectURI:    "http://localhost:17833/oauth/callback/kick",
	}
	data, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		log.Printf("warning: could not marshal config template: %v", err)
		return
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		log.Printf("warning: could not create config template (%s): %v", path, err)
	}
}

func loadJSONConfig() (*fileConfig, error) {
	dir := configDir()
	if dir == "" {
		configFilePath = ""
		return &fileConfig{}, nil
	}

	configFilePath = filepath.Join(dir, "config.json")
	ensureTemplateConfig(configFilePath)
	cachedFileConfig = &fileConfig{
		TwitchClientSecret: "",
		TwitchRedirectURI:  "http://localhost:17833/oauth/callback/twitch",
		KickRedirectURI:    "http://localhost:17833/oauth/callback/kick",
	}

	data, err := os.ReadFile(configFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cachedFileConfig, nil
		}
		return nil, err
	}

	var parsed fileConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	cachedFileConfig = &parsed
	return &parsed, nil
}

func configDir() string {
	switch runtime.GOOS {
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "zhatbot")
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, "Library", "Application Support", "zhatbot")
		}
	}

	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "zhatbot")
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "zhatbot")
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func SaveTwitchSecret(secret string) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return fmt.Errorf("secret cannot be empty")
	}
	path := ConfigFilePath()
	if path == "" {
		dir := configDir()
		if dir == "" {
			return fmt.Errorf("config directory unavailable")
		}
		path = filepath.Join(dir, "config.json")
		configFilePath = path
	}

	cfg := &fileConfig{}
	if cachedFileConfig != nil {
		cfg = cachedFileConfig
	}
	cfgCopy := *cfg
	cfgCopy.TwitchClientSecret = secret
	data, err := json.MarshalIndent(cfgCopy, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	cachedFileConfig = &cfgCopy
	return nil
}
