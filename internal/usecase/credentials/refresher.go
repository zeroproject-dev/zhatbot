package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	kicksdk "github.com/glichtv/kick-sdk"

	"zhatBot/internal/domain"
)

type TwitchConfig struct {
	ClientID     string
	ClientSecret string
}

type KickConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

type Refresher struct {
	repo      domain.CredentialRepository
	twitchCfg TwitchConfig
	kickCfg   KickConfig
	kickCli   *kicksdk.Client
	httpCli   *http.Client

	hooksMu sync.RWMutex
	hooks   []CredentialHook
}

type CredentialHook func(ctx context.Context, cred *domain.Credential)

func NewRefresher(repo domain.CredentialRepository, twitchCfg TwitchConfig, kickCfg KickConfig) *Refresher {
	var kickClient *kicksdk.Client
	if kickCfg.ClientID != "" && kickCfg.ClientSecret != "" && kickCfg.RedirectURI != "" {
		kickClient = kicksdk.NewClient(
			kicksdk.WithCredentials(kicksdk.Credentials{
				ClientID:     kickCfg.ClientID,
				ClientSecret: kickCfg.ClientSecret,
				RedirectURI:  kickCfg.RedirectURI,
			}),
		)
	}

	return &Refresher{
		repo:      repo,
		twitchCfg: twitchCfg,
		kickCfg:   kickCfg,
		kickCli:   kickClient,
		httpCli: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (r *Refresher) RegisterHook(h CredentialHook) {
	if h == nil {
		return
	}
	r.hooksMu.Lock()
	defer r.hooksMu.Unlock()
	r.hooks = append(r.hooks, h)
}

func (r *Refresher) notifyHooks(ctx context.Context, cred *domain.Credential) {
	if cred == nil {
		return
	}
	r.hooksMu.RLock()
	hooks := append([]CredentialHook(nil), r.hooks...)
	r.hooksMu.RUnlock()
	for _, h := range hooks {
		h(ctx, cred)
	}
}

func (r *Refresher) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.RefreshAll(ctx); err != nil {
					log.Printf("token refresher: %v", err)
				}
			}
		}
	}()
}

func (r *Refresher) RefreshAll(ctx context.Context) error {
	if r.repo == nil {
		return nil
	}

	creds, err := r.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("refresher: list credentials: %w", err)
	}

	for _, cred := range creds {
		if err := ctx.Err(); err != nil {
			return err
		}

		if cred == nil || cred.RefreshToken == "" {
			continue
		}

		if !needsRefresh(cred) {
			continue
		}

		switch cred.Platform {
		case domain.PlatformTwitch:
			if err := r.refreshTwitch(ctx, cred); err != nil {
				return err
			}
		case domain.PlatformKick:
			if err := r.refreshKick(ctx, cred); err != nil {
				return err
			}
		}
	}

	return nil
}

func needsRefresh(cred *domain.Credential) bool {
	if cred == nil {
		return false
	}
	if cred.ExpiresAt.IsZero() {
		return true
	}
	return time.Until(cred.ExpiresAt) < 10*time.Minute
}

func (r *Refresher) refreshTwitch(ctx context.Context, cred *domain.Credential) error {
	if r.twitchCfg.ClientID == "" || r.twitchCfg.ClientSecret == "" {
		return fmt.Errorf("refresher: twitch config incompleta")
	}

	data := url.Values{}
	data.Set("client_id", r.twitchCfg.ClientID)
	data.Set("client_secret", r.twitchCfg.ClientSecret)
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", cred.RefreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.twitch.tv/oauth2/token", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("refresher: twitch request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := r.httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("refresher: twitch http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("refresher: twitch read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresher: twitch status %d: %s", resp.StatusCode, string(body))
	}

	var payload twitchTokenPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("refresher: twitch decode: %w", err)
	}

	cred.AccessToken = payload.AccessToken
	if payload.RefreshToken != "" {
		cred.RefreshToken = payload.RefreshToken
	}
	cred.ExpiresAt = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	cred.UpdatedAt = time.Now()

	if err := r.repo.Save(ctx, cred); err != nil {
		return err
	}
	r.notifyHooks(ctx, cred)
	return nil
}

func (r *Refresher) refreshKick(ctx context.Context, cred *domain.Credential) error {
	if r.kickCli == nil {
		return fmt.Errorf("refresher: kick config incompleta")
	}

	resp, err := r.kickCli.OAuth().RefreshToken(ctx, kicksdk.RefreshTokenInput{
		RefreshToken: cred.RefreshToken,
		GrantType:    "refresh_token",
	})
	if err != nil {
		return fmt.Errorf("refresher: kick refresh: %w", err)
	}

	payload := resp.Payload
	cred.AccessToken = payload.AccessToken
	if payload.RefreshToken != "" {
		cred.RefreshToken = payload.RefreshToken
	}
	cred.ExpiresAt = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	cred.UpdatedAt = time.Now()

	if err := r.repo.Save(ctx, cred); err != nil {
		return err
	}
	r.notifyHooks(ctx, cred)
	return nil
}

type twitchTokenPayload struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}
