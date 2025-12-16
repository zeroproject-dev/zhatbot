package ws

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

const (
	twitchAuthorizeURL = "https://id.twitch.tv/oauth2/authorize"
	twitchTokenURL     = "https://id.twitch.tv/oauth2/token"
)

type Config struct {
	Addr           string
	CredentialRepo domain.CredentialRepository
	Twitch         *TwitchOAuthConfig
	Kick           *KickOAuthConfig
}

type TwitchOAuthConfig struct {
	ClientID       string
	ClientSecret   string
	RedirectURI    string
	BotScopes      []string
	StreamerScopes []string
}

type KickOAuthConfig struct {
	ClientID       string
	ClientSecret   string
	RedirectURI    string
	BotScopes      []string
	StreamerScopes []string
}

func (c *Config) addr() string {
	if c == nil || c.Addr == "" {
		return ":8080"
	}
	return c.Addr
}

func (c *TwitchOAuthConfig) enabled() bool {
	return c != nil && c.ClientID != "" && c.ClientSecret != "" && c.RedirectURI != ""
}

func (c *KickOAuthConfig) enabled() bool {
	return c != nil && c.ClientID != "" && c.ClientSecret != "" && c.RedirectURI != ""
}

func (c *TwitchOAuthConfig) scopesForRole(role string) []string {
	role = normalizeRole(role)
	if role == "streamer" {
		if len(c.StreamerScopes) > 0 {
			return c.StreamerScopes
		}
		return []string{"channel:manage:broadcast"}
	}

	if len(c.BotScopes) > 0 {
		return c.BotScopes
	}
	return []string{"chat:read", "chat:edit"}
}

func (c *KickOAuthConfig) scopesForRole(role string) []kicksdk.OAuthScope {
	role = normalizeRole(role)
	var scopes []string
	switch role {
	case "streamer":
		if len(c.StreamerScopes) > 0 {
			scopes = c.StreamerScopes
		} else {
			scopes = []string{
				string(kicksdk.ScopeUserRead),
				string(kicksdk.ScopeChannelRead),
				string(kicksdk.ScopeChannelWrite),
			}
		}
	default:
		if len(c.BotScopes) > 0 {
			scopes = c.BotScopes
		} else {
			scopes = []string{
				string(kicksdk.ScopeUserRead),
				string(kicksdk.ScopeChannelRead),
				string(kicksdk.ScopeChannelWrite),
			}
		}
	}

	result := make([]kicksdk.OAuthScope, 0, len(scopes))
	for _, sc := range scopes {
		result = append(result, kicksdk.OAuthScope(sc))
	}
	return result
}

type apiHandlers struct {
	credRepo domain.CredentialRepository
	state    *oauthStateStore

	httpClient *http.Client

	twitchCfg *TwitchOAuthConfig
	kickCfg   *KickOAuthConfig
	kickOAuth *kicksdk.Client
}

func newAPIHandlers(cfg Config) *apiHandlers {
	var kickClient *kicksdk.Client
	if cfg.Kick != nil && cfg.Kick.enabled() {
		kickClient = kicksdk.NewClient(
			kicksdk.WithCredentials(kicksdk.Credentials{
				ClientID:     cfg.Kick.ClientID,
				ClientSecret: cfg.Kick.ClientSecret,
				RedirectURI:  cfg.Kick.RedirectURI,
			}),
		)
	}

	return &apiHandlers{
		credRepo: cfg.CredentialRepo,
		state:    newOAuthStateStore(),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		twitchCfg: cfg.Twitch,
		kickCfg:   cfg.Kick,
		kickOAuth: kickClient,
	}
}

func (a *apiHandlers) register(mux *http.ServeMux) {
	if a == nil || mux == nil {
		return
	}

	mux.HandleFunc("/api/oauth/status", a.withCORS(a.handleStatus))

	if a.twitchCfg != nil && a.twitchCfg.enabled() {
		mux.HandleFunc("/api/oauth/twitch/start", a.withCORS(a.handleTwitchStart))
		mux.HandleFunc("/api/oauth/twitch/callback", a.handleTwitchCallback)
	}

	if a.kickCfg != nil && a.kickCfg.enabled() && a.kickOAuth != nil {
		mux.HandleFunc("/api/oauth/kick/start", a.withCORS(a.handleKickStart))
		mux.HandleFunc("/api/oauth/kick/callback", a.handleKickCallback)
	}
}

func (a *apiHandlers) withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
}

type oauthStartRequest struct {
	Role string `json:"role"`
}

type oauthStartResponse struct {
	URL string `json:"url"`
}

type credentialStatus struct {
	HasAccessToken  bool      `json:"has_access_token"`
	HasRefreshToken bool      `json:"has_refresh_token"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
	ExpiresAt       time.Time `json:"expires_at,omitempty"`
}

type statusResponse struct {
	Credentials map[string]map[string]credentialStatus `json:"credentials"`
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "streamer":
		return "streamer"
	default:
		return "bot"
	}
}

func (a *apiHandlers) handleTwitchStart(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.twitchCfg == nil || !a.twitchCfg.enabled() {
		http.NotFound(w, r)
		return
	}

	var req oauthStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	role := normalizeRole(req.Role)
	verifier, err := generateCodeVerifier()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start oauth")
		return
	}

	state := a.state.Add(domain.PlatformTwitch, role, verifier)
	challenge := generateCodeChallenge(verifier)

	q := url.Values{}
	q.Set("client_id", a.twitchCfg.ClientID)
	q.Set("redirect_uri", a.twitchCfg.RedirectURI)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(a.twitchCfg.scopesForRole(role), " "))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")

	authURL := twitchAuthorizeURL + "?" + q.Encode()

	writeJSON(w, http.StatusOK, oauthStartResponse{URL: authURL})
}

func (a *apiHandlers) handleTwitchCallback(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.twitchCfg == nil || !a.twitchCfg.enabled() {
		http.NotFound(w, r)
		return
	}

	if a.credRepo == nil {
		writeHTML(w, http.StatusInternalServerError, "No hay almacenamiento configurado.")
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		writeHTML(w, http.StatusBadRequest, "Missing code or state.")
		return
	}

	entry, ok := a.state.Consume(state)
	if !ok || entry.Platform != domain.PlatformTwitch {
		writeHTML(w, http.StatusBadRequest, "Invalid state.")
		return
	}

	tokenResp, err := a.exchangeTwitchToken(r.Context(), code, entry.CodeVerifier)
	if err != nil {
		log.Printf("twitch oauth: token exchange error: %v", err)
		writeHTML(w, http.StatusInternalServerError, "Token exchange failed.")
		return
	}

	cred := &domain.Credential{
		Platform:     domain.PlatformTwitch,
		Role:         entry.Role,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	if err := a.credRepo.Save(r.Context(), cred); err != nil {
		log.Printf("twitch oauth: saving credential failed: %v", err)
		writeHTML(w, http.StatusInternalServerError, "Could not store credentials.")
		return
	}

	writeHTML(w, http.StatusOK, fmt.Sprintf("✅ Tokens guardados para Twitch (%s). Ya puedes cerrar esta ventana.", entry.Role))
}

type twitchTokenResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int64    `json:"expires_in"`
	TokenType    string   `json:"token_type"`
	Scope        []string `json:"scope"`
}

func (a *apiHandlers) exchangeTwitchToken(ctx context.Context, code, verifier string) (*twitchTokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", a.twitchCfg.ClientID)
	data.Set("client_secret", a.twitchCfg.ClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", a.twitchCfg.RedirectURI)
	data.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, twitchTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch token endpoint error: %s", string(body))
	}

	var payload twitchTokenResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	return &payload, nil
}

func (a *apiHandlers) handleKickStart(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.kickCfg == nil || !a.kickCfg.enabled() || a.kickOAuth == nil {
		http.NotFound(w, r)
		return
	}

	var req oauthStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	role := normalizeRole(req.Role)
	verifier, err := generateCodeVerifier()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start oauth")
		return
	}

	state := a.state.Add(domain.PlatformKick, role, verifier)
	challenge := generateCodeChallenge(verifier)

	authURL := a.kickOAuth.OAuth().AuthorizationURL(kicksdk.AuthorizationURLInput{
		ResponseType:  "code",
		State:         state,
		Scopes:        a.kickCfg.scopesForRole(role),
		CodeChallenge: challenge,
	})

	writeJSON(w, http.StatusOK, oauthStartResponse{URL: authURL})
}

func (a *apiHandlers) handleKickCallback(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.kickCfg == nil || !a.kickCfg.enabled() || a.kickOAuth == nil {
		http.NotFound(w, r)
		return
	}

	if a.credRepo == nil {
		writeHTML(w, http.StatusInternalServerError, "No hay almacenamiento configurado.")
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		writeHTML(w, http.StatusBadRequest, "Missing code or state.")
		return
	}

	entry, ok := a.state.Consume(state)
	if !ok || entry.Platform != domain.PlatformKick {
		writeHTML(w, http.StatusBadRequest, "Invalid state.")
		return
	}

	resp, err := a.kickOAuth.OAuth().ExchangeCode(r.Context(), kicksdk.ExchangeCodeInput{
		Code:         code,
		GrantType:    "authorization_code",
		CodeVerifier: entry.CodeVerifier,
	})
	if err != nil {
		log.Printf("kick oauth: token exchange failed: %v", err)
		writeHTML(w, http.StatusInternalServerError, "Token exchange failed.")
		return
	}

	payload := resp.Payload
	cred := &domain.Credential{
		Platform:     domain.PlatformKick,
		Role:         entry.Role,
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second),
	}

	if err := a.credRepo.Save(r.Context(), cred); err != nil {
		log.Printf("kick oauth: saving credential failed: %v", err)
		writeHTML(w, http.StatusInternalServerError, "Could not store credentials.")
		return
	}

	writeHTML(w, http.StatusOK, fmt.Sprintf("✅ Tokens guardados para Kick (%s). Ya puedes cerrar esta ventana.", entry.Role))
}

func (a *apiHandlers) handleStatus(w http.ResponseWriter, r *http.Request) {
	if a == nil {
		writeJSON(w, http.StatusOK, statusResponse{Credentials: map[string]map[string]credentialStatus{}})
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if a.credRepo == nil {
		writeJSON(w, http.StatusOK, statusResponse{Credentials: map[string]map[string]credentialStatus{}})
		return
	}

	list, err := a.credRepo.List(r.Context())
	if err != nil {
		log.Printf("oauth status: list error: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load credentials")
		return
	}

	resp := statusResponse{
		Credentials: make(map[string]map[string]credentialStatus),
	}

	for _, cred := range list {
		plat := string(cred.Platform)
		if plat == "" {
			continue
		}
		if _, ok := resp.Credentials[plat]; !ok {
			resp.Credentials[plat] = make(map[string]credentialStatus)
		}

		resp.Credentials[plat][cred.Role] = credentialStatus{
			HasAccessToken:  cred.AccessToken != "",
			HasRefreshToken: cred.RefreshToken != "",
			UpdatedAt:       cred.UpdatedAt,
			ExpiresAt:       cred.ExpiresAt,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeHTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func generateCodeVerifier() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func generateCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type oauthStateStore struct {
	mu     sync.Mutex
	values map[string]oauthStateEntry
}

type oauthStateEntry struct {
	Platform     domain.Platform
	Role         string
	CodeVerifier string
	CreatedAt    time.Time
}

func newOAuthStateStore() *oauthStateStore {
	return &oauthStateStore{
		values: make(map[string]oauthStateEntry),
	}
}

func (s *oauthStateStore) Add(platform domain.Platform, role, verifier string) string {
	id := randomStateID()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[id] = oauthStateEntry{
		Platform:     platform,
		Role:         role,
		CodeVerifier: verifier,
		CreatedAt:    time.Now(),
	}
	return id
}

func (s *oauthStateStore) Consume(state string) (oauthStateEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.values[state]
	if !ok {
		return oauthStateEntry{}, false
	}
	delete(s.values, state)

	if time.Since(entry.CreatedAt) > 10*time.Minute {
		return oauthStateEntry{}, false
	}

	return entry, true
}

func randomStateID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("state-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
