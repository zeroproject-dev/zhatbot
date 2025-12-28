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
	"strconv"
	"strings"
	"sync"
	"time"

	kicksdk "github.com/glichtv/kick-sdk"

	"zhatBot/internal/app/events"
	"zhatBot/internal/domain"
	commandsusecase "zhatBot/internal/usecase/commands"
	statususecase "zhatBot/internal/usecase/status"
	ttsusecase "zhatBot/internal/usecase/tts"
)

const (
	twitchAuthorizeURL = "https://id.twitch.tv/oauth2/authorize"
	twitchTokenURL     = "https://id.twitch.tv/oauth2/token"
)

type Config struct {
	Addr             string
	CredentialRepo   domain.CredentialRepository
	NotificationRepo domain.NotificationRepository
	CredentialHook   CredentialHook
	Twitch           *TwitchOAuthConfig
	Kick             *KickOAuthConfig
	CategoryManager  CategoryManager
	TTSManager       TTSManager
	TTSRunnerStatus  TTSStatusReporter
	StatusResolver   *statususecase.Resolver
	CommandManager   *commandsusecase.CustomCommandManager
	CommandService   *commandsusecase.Service
}

type CategoryManager interface {
	Search(ctx context.Context, platform domain.Platform, query string) ([]domain.CategoryOption, error)
	Update(ctx context.Context, platform domain.Platform, categoryName string) error
}

type TTSManager interface {
	ListVoices() []ttsusecase.VoiceOption
	CurrentVoice(ctx context.Context) ttsusecase.VoiceOption
	Enabled(ctx context.Context) bool
	SetVoice(ctx context.Context, code string) (ttsusecase.VoiceOption, error)
	SetEnabled(ctx context.Context, enabled bool) error
}

type TTSStatusReporter interface {
	Status() events.TTSStatusDTO
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
				string(kicksdk.ScopeChatWrite),
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
				string(kicksdk.ScopeChatWrite),
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
	credRepo      domain.CredentialRepository
	notifications domain.NotificationRepository
	state         *oauthStateStore

	httpClient *http.Client

	twitchCfg  *TwitchOAuthConfig
	kickCfg    *KickOAuthConfig
	kickOAuth  *kicksdk.Client
	category   CategoryManager
	tts        TTSManager
	ttsStatus  TTSStatusReporter
	status     *statususecase.Resolver
	commands   *commandsusecase.CustomCommandManager
	commandSvc *commandsusecase.Service
	hook       CredentialHook
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
		credRepo:      cfg.CredentialRepo,
		notifications: cfg.NotificationRepo,
		state:         newOAuthStateStore(),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		twitchCfg:  cfg.Twitch,
		kickCfg:    cfg.Kick,
		kickOAuth:  kickClient,
		category:   cfg.CategoryManager,
		tts:        cfg.TTSManager,
		ttsStatus:  cfg.TTSRunnerStatus,
		status:     cfg.StatusResolver,
		commands:   cfg.CommandManager,
		commandSvc: cfg.CommandService,
		hook:       cfg.CredentialHook,
	}
}

func (a *apiHandlers) register(mux *http.ServeMux) {
	if a == nil || mux == nil {
		return
	}

	mux.HandleFunc("/api/oauth/status", a.withCORS(a.handleStatus))
	mux.HandleFunc("/api/oauth/logout", a.withCORS(a.handleLogout))
	if a.category != nil {
		mux.HandleFunc("/api/categories/search", a.withCORS(a.handleCategorySearch))
		mux.HandleFunc("/api/categories/update", a.withCORS(a.handleCategoryUpdate))
	}
	if a.tts != nil {
		mux.HandleFunc("/api/tts/status", a.withCORS(a.handleTTSStatus))
		mux.HandleFunc("/api/tts/settings", a.withCORS(a.handleTTSUpdate))
	}
	if a.notifications != nil {
		mux.HandleFunc("/api/notifications", a.withCORS(a.handleNotifications))
	}
	if a.status != nil {
		mux.HandleFunc("/api/streams/status", a.withCORS(a.handleStreamStatus))
	}
	if a.commandSvc != nil {
		mux.HandleFunc("/api/commands", a.withCORS(a.handleCommands))
	}

	if a.twitchCfg != nil && a.twitchCfg.enabled() {
		mux.HandleFunc("/api/oauth/twitch/start", a.withCORS(a.handleTwitchStart))
		mux.HandleFunc("/api/oauth/twitch/callback", a.handleTwitchCallback)
	}

	if a.kickCfg != nil && a.kickCfg.enabled() && a.kickOAuth != nil {
		mux.HandleFunc("/api/oauth/kick/start", a.withCORS(a.handleKickStart))
		mux.HandleFunc("/api/oauth/kick/callback", a.handleKickCallback)
	}
}

func (a *apiHandlers) setTTSManager(manager TTSManager) {
	if a == nil {
		return
	}
	a.tts = manager
}

func (a *apiHandlers) setTTSStatusProvider(provider TTSStatusReporter) {
	if a == nil {
		return
	}
	a.ttsStatus = provider
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
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
}

type oauthStartRequest struct {
	Role string `json:"role"`
}

type oauthStartResponse struct {
	URL string `json:"url"`
}

type CredentialStatus struct {
	HasAccessToken  bool      `json:"has_access_token"`
	HasRefreshToken bool      `json:"has_refresh_token"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
	ExpiresAt       time.Time `json:"expires_at,omitempty"`
}

type OAuthStatus struct {
	Credentials map[string]map[string]CredentialStatus `json:"credentials"`
}

type categorySearchResponse struct {
	Options []domain.CategoryOption `json:"options"`
}

type categoryUpdateRequest struct {
	Platform string `json:"platform"`
	Name     string `json:"name"`
}

type ttsStatusResponse struct {
	Enabled           bool               `json:"enabled"`
	Voice             string             `json:"voice"`
	VoiceLabel        string             `json:"voice_label,omitempty"`
	Voices            []ttsVoiceResponse `json:"voices"`
	RunnerState       string             `json:"runner_state,omitempty"`
	RunnerQueueLength int                `json:"runner_queue_length,omitempty"`
	RunnerCurrentID   string             `json:"runner_current_id,omitempty"`
	RunnerLastError   string             `json:"runner_last_error,omitempty"`
}

type ttsVoiceResponse struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type ttsUpdateRequest struct {
	Voice   string `json:"voice"`
	Enabled *bool  `json:"enabled"`
}

type oauthLogoutRequest struct {
	Platform string `json:"platform"`
	Role     string `json:"role"`
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "streamer":
		return "streamer"
	default:
		return "bot"
	}
}

func (a *apiHandlers) oauthStart(platform domain.Platform, role string) (string, error) {
	if a == nil {
		return "", fmt.Errorf("oauth no configurado")
	}
	switch platform {
	case domain.PlatformTwitch:
		return a.startTwitchOAuth(role)
	case domain.PlatformKick:
		return a.startKickOAuth(role)
	default:
		return "", fmt.Errorf("plataforma no soportada")
	}
}

func (a *apiHandlers) startTwitchOAuth(role string) (string, error) {
	if a == nil || a.twitchCfg == nil || !a.twitchCfg.enabled() {
		return "", fmt.Errorf("twitch oauth no disponible")
	}

	role = normalizeRole(role)
	verifier, err := generateCodeVerifier()
	if err != nil {
		return "", err
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
	return authURL, nil
}

func (a *apiHandlers) startKickOAuth(role string) (string, error) {
	if a == nil || a.kickCfg == nil || !a.kickCfg.enabled() || a.kickOAuth == nil {
		return "", fmt.Errorf("kick oauth no disponible")
	}

	role = normalizeRole(role)
	if role != "streamer" {
		log.Printf("kick oauth: role %q solicitado, usando streamer como único rol soportado", role)
	}
	role = "streamer"
	log.Println("kick oauth: si necesitas el scope chat:write, revoca la app en Kick (Settings > Connections) y vuelve a iniciar sesión.")

	verifier, err := generateCodeVerifier()
	if err != nil {
		return "", err
	}

	state := a.state.Add(domain.PlatformKick, role, verifier)
	challenge := generateCodeChallenge(verifier)

	authURL := a.kickOAuth.OAuth().AuthorizationURL(kicksdk.AuthorizationURLInput{
		ResponseType:  "code",
		State:         state,
		Scopes:        a.kickCfg.scopesForRole(role),
		CodeChallenge: challenge,
	})

	return authURL, nil
}

func (a *apiHandlers) oauthStatus(ctx context.Context) (OAuthStatus, error) {
	if a == nil || a.credRepo == nil {
		return OAuthStatus{Credentials: map[string]map[string]CredentialStatus{}}, nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	list, err := a.credRepo.List(ctx)
	if err != nil {
		return OAuthStatus{}, err
	}

	resp := OAuthStatus{
		Credentials: make(map[string]map[string]CredentialStatus),
	}

	for _, cred := range list {
		plat := string(cred.Platform)
		if plat == "" {
			continue
		}
		if _, ok := resp.Credentials[plat]; !ok {
			resp.Credentials[plat] = make(map[string]CredentialStatus)
		}

		resp.Credentials[plat][cred.Role] = CredentialStatus{
			HasAccessToken:  cred.AccessToken != "",
			HasRefreshToken: cred.RefreshToken != "",
			UpdatedAt:       cred.UpdatedAt,
			ExpiresAt:       cred.ExpiresAt,
		}
	}

	return resp, nil
}

func (a *apiHandlers) oauthLogout(ctx context.Context, platform domain.Platform, role string) error {
	if a == nil || a.credRepo == nil {
		return fmt.Errorf("credential store no disponible")
	}
	if platform == "" {
		return fmt.Errorf("plataforma inválida")
	}

	role = normalizeRole(role)
	if platform == domain.PlatformKick {
		role = "streamer"
	}

	if ctx == nil {
		ctx = context.Background()
	}

	if err := a.credRepo.Delete(ctx, platform, role); err != nil {
		return err
	}

	a.notifyCredentialHook(ctx, &domain.Credential{
		Platform: platform,
		Role:     role,
	})
	return nil
}

func (a *apiHandlers) handleCategorySearch(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.category == nil {
		http.NotFound(w, r)
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

	platform := parsePlatformParam(r.URL.Query().Get("platform"))
	if platform == "" {
		writeError(w, http.StatusBadRequest, "invalid platform")
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if query == "" {
		writeError(w, http.StatusBadRequest, "missing query")
		return
	}

	options, err := a.category.Search(r.Context(), platform, query)
	if err != nil {
		log.Printf("category search error: %v", err)
		writeError(w, http.StatusInternalServerError, "category search failed")
		return
	}

	writeJSON(w, http.StatusOK, categorySearchResponse{Options: options})
}

func (a *apiHandlers) handleCategoryUpdate(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.category == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()
	var req categoryUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	platform := parsePlatformParam(req.Platform)
	if platform == "" {
		writeError(w, http.StatusBadRequest, "invalid platform")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing name")
		return
	}

	if err := a.category.Update(r.Context(), platform, name); err != nil {
		log.Printf("category update error: %v", err)
		writeError(w, http.StatusInternalServerError, "category update failed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *apiHandlers) handleTTSStatus(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.tts == nil {
		http.NotFound(w, r)
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

	status := ttsStatusResponse{
		Enabled: a.tts.Enabled(r.Context()),
	}
	current := a.tts.CurrentVoice(r.Context())
	status.Voice = current.Code
	status.VoiceLabel = current.Label

	voices := a.tts.ListVoices()
	status.Voices = make([]ttsVoiceResponse, 0, len(voices))
	for _, v := range voices {
		status.Voices = append(status.Voices, ttsVoiceResponse{Code: v.Code, Label: v.Label})
	}

	if a.ttsStatus != nil {
		runner := a.ttsStatus.Status()
		status.RunnerState = runner.State
		status.RunnerQueueLength = runner.QueueLength
		status.RunnerCurrentID = runner.CurrentID
		status.RunnerLastError = runner.LastError
	}

	writeJSON(w, http.StatusOK, status)
}

func (a *apiHandlers) handleTTSUpdate(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.tts == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()
	var req ttsUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	if strings.TrimSpace(req.Voice) != "" {
		if _, err := a.tts.SetVoice(r.Context(), req.Voice); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	if req.Enabled != nil {
		if err := a.tts.SetEnabled(r.Context(), *req.Enabled); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	status := ttsStatusResponse{
		Enabled: a.tts.Enabled(r.Context()),
	}
	current := a.tts.CurrentVoice(r.Context())
	status.Voice = current.Code
	status.VoiceLabel = current.Label
	voices := a.tts.ListVoices()
	status.Voices = make([]ttsVoiceResponse, 0, len(voices))
	for _, v := range voices {
		status.Voices = append(status.Voices, ttsVoiceResponse{Code: v.Code, Label: v.Label})
	}

	writeJSON(w, http.StatusOK, status)
}

func (a *apiHandlers) handleNotifications(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.handleNotificationsList(w, r)
	case http.MethodPost:
		a.handleNotificationsCreate(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *apiHandlers) handleNotificationsList(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.notifications == nil {
		http.NotFound(w, r)
		return
	}

	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	ctx := r.Context()
	items, err := a.notifications.ListNotifications(ctx, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load notifications")
		return
	}

	writeJSON(w, http.StatusOK, toNotificationResponseList(items))
}

func (a *apiHandlers) handleNotificationsCreate(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.notifications == nil {
		http.NotFound(w, r)
		return
	}

	defer r.Body.Close()

	var payload notificationRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	notificationType := normalizeNotificationType(payload.Type)
	if notificationType == "" {
		writeError(w, http.StatusBadRequest, "invalid type")
		return
	}

	record := &domain.Notification{
		Type:     notificationType,
		Platform: domain.Platform(strings.TrimSpace(payload.Platform)),
		Username: strings.TrimSpace(payload.Username),
		Amount:   payload.Amount,
		Message:  strings.TrimSpace(payload.Message),
		Metadata: payload.Metadata,
	}

	ctx := r.Context()
	saved, err := a.notifications.SaveNotification(ctx, record)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save notification")
		return
	}

	writeJSON(w, http.StatusOK, toNotificationResponse(saved))
}

func (a *apiHandlers) handleStreamStatus(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.status == nil {
		http.NotFound(w, r)
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

	statuses := a.status.Snapshot(r.Context())
	response := make([]streamStatusResponse, 0, len(statuses))
	for _, entry := range statuses {
		response = append(response, streamStatusResponse{
			Platform:    string(entry.Platform),
			IsLive:      entry.IsLive,
			Title:       entry.Title,
			GameTitle:   entry.GameTitle,
			ViewerCount: entry.ViewerCount,
			URL:         entry.URL,
			StartedAt:   formatTime(entry.StartedAt),
		})
	}

	writeJSON(w, http.StatusOK, response)
}

func (a *apiHandlers) handleCommands(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.commandSvc == nil {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		a.handleCommandsList(w, r)
	case http.MethodPost:
		a.handleCommandsSave(w, r)
	case http.MethodDelete:
		a.handleCommandsDelete(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *apiHandlers) handleCommandsList(w http.ResponseWriter, r *http.Request) {
	items, err := a.commandSvc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *apiHandlers) handleCommandsSave(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var payload commandsusecase.CommandMutationDTO
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	result, err := a.commandSvc.Upsert(r.Context(), payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *apiHandlers) handleCommandsDelete(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		var payload struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		name = strings.TrimSpace(payload.Name)
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing name")
		return
	}
	deleted, err := a.commandSvc.Delete(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "command not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

	url, err := a.startTwitchOAuth(req.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start oauth")
		return
	}

	writeJSON(w, http.StatusOK, oauthStartResponse{URL: url})
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

	metadata := make(map[string]string)
	if profile, err := a.fetchTwitchProfile(r.Context(), tokenResp.AccessToken); err == nil {
		if profile.ID != "" {
			metadata["user_id"] = profile.ID
		}
		if profile.Login != "" {
			metadata["login"] = profile.Login
		}
	} else {
		log.Printf("twitch oauth: no pude obtener el perfil: %v", err)
	}

	cred := &domain.Credential{
		Platform:     domain.PlatformTwitch,
		Role:         entry.Role,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Metadata:     metadata,
	}

	if err := a.credRepo.Save(r.Context(), cred); err != nil {
		log.Printf("twitch oauth: saving credential failed: %v", err)
		writeHTML(w, http.StatusInternalServerError, "Could not store credentials.")
		return
	}
	a.notifyCredentialHook(r.Context(), cred)

	writeHTML(w, http.StatusOK, fmt.Sprintf("✅ Tokens guardados para Twitch (%s). Ya puedes cerrar esta ventana.", entry.Role))
}

type twitchTokenResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int64    `json:"expires_in"`
	TokenType    string   `json:"token_type"`
	Scope        []string `json:"scope"`
}

type twitchProfile struct {
	ID    string `json:"id"`
	Login string `json:"login"`
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

func (a *apiHandlers) fetchTwitchProfile(ctx context.Context, accessToken string) (*twitchProfile, error) {
	if a == nil || a.httpClient == nil {
		return nil, fmt.Errorf("http client no configurado")
	}
	if a.twitchCfg == nil || a.twitchCfg.ClientID == "" {
		return nil, fmt.Errorf("twitch client id vacío")
	}
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return nil, fmt.Errorf("access token vacío")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitch.tv/helix/users", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-ID", a.twitchCfg.ClientID)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitch profile request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("twitch profile request failed (%d): %s", resp.StatusCode, string(body))
	}

	var payload struct {
		Data []twitchProfile `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("twitch profile decode: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("twitch profile: respuesta vacía")
	}
	return &payload.Data[0], nil
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

	url, err := a.startKickOAuth(req.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start oauth")
		return
	}

	writeJSON(w, http.StatusOK, oauthStartResponse{URL: url})
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
	if payload.Scope != "" {
		log.Printf("kick oauth: scope otorgado: %s", payload.Scope)
	}
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
	a.notifyCredentialHook(r.Context(), cred)

	writeHTML(w, http.StatusOK, fmt.Sprintf("✅ Tokens guardados para Kick (%s). Ya puedes cerrar esta ventana.", entry.Role))
}

func (a *apiHandlers) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	status, err := a.oauthStatus(r.Context())
	if err != nil {
		log.Printf("oauth status: list error: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load credentials")
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (a *apiHandlers) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req oauthLogoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	platform := parsePlatformParam(req.Platform)
	if platform == "" {
		writeError(w, http.StatusBadRequest, "invalid platform")
		return
	}

	if err := a.oauthLogout(r.Context(), platform, req.Role); err != nil {
		log.Printf("oauth logout: delete failed (%s/%s): %v", platform, req.Role, err)
		writeError(w, http.StatusInternalServerError, "could not delete credentials")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *apiHandlers) notifyCredentialHook(ctx context.Context, cred *domain.Credential) {
	if a == nil || a.hook == nil || cred == nil {
		return
	}
	a.hook(ctx, cred)
}

type notificationRequest struct {
	Type     string            `json:"type"`
	Platform string            `json:"platform"`
	Username string            `json:"username"`
	Amount   float64           `json:"amount"`
	Message  string            `json:"message"`
	Metadata map[string]string `json:"metadata"`
}

type notificationResponse struct {
	ID        int64             `json:"id"`
	Type      string            `json:"type"`
	Platform  string            `json:"platform"`
	Username  string            `json:"username"`
	Amount    float64           `json:"amount"`
	Message   string            `json:"message"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt string            `json:"created_at"`
}

type streamStatusResponse struct {
	Platform    string `json:"platform"`
	IsLive      bool   `json:"is_live"`
	Title       string `json:"title,omitempty"`
	GameTitle   string `json:"game_title,omitempty"`
	ViewerCount int    `json:"viewer_count,omitempty"`
	URL         string `json:"url,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
}

func toNotificationResponse(item *domain.Notification) notificationResponse {
	if item == nil {
		return notificationResponse{}
	}

	var created string
	if !item.CreatedAt.IsZero() {
		created = item.CreatedAt.UTC().Format(time.RFC3339)
	}

	return notificationResponse{
		ID:        item.ID,
		Type:      string(item.Type),
		Platform:  string(item.Platform),
		Username:  item.Username,
		Amount:    item.Amount,
		Message:   item.Message,
		Metadata:  item.Metadata,
		CreatedAt: created,
	}
}

func toNotificationResponseList(items []*domain.Notification) []notificationResponse {
	out := make([]notificationResponse, 0, len(items))
	for _, item := range items {
		out = append(out, toNotificationResponse(item))
	}
	return out
}

func normalizeNotificationType(value string) domain.NotificationType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(domain.NotificationSubscription):
		return domain.NotificationSubscription
	case string(domain.NotificationDonation):
		return domain.NotificationDonation
	case string(domain.NotificationBits):
		return domain.NotificationBits
	case string(domain.NotificationGiveawayWinner):
		return domain.NotificationGiveawayWinner
	case string(domain.NotificationGeneric):
		return domain.NotificationGeneric
	case "":
		return ""
	default:
		return domain.NotificationGeneric
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func parsePlatformParam(p string) domain.Platform {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case string(domain.PlatformTwitch):
		return domain.PlatformTwitch
	case string(domain.PlatformKick):
		return domain.PlatformKick
	default:
		return ""
	}
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

type CredentialHook func(ctx context.Context, cred *domain.Credential)
