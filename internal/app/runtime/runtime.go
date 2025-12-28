package runtime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nicklaw5/helix/v2"

	"zhatBot/internal/app"
	"zhatBot/internal/app/events"
	ttsruntime "zhatBot/internal/app/tts/runner"
	"zhatBot/internal/domain"
	"zhatBot/internal/infrastructure/config"
	sqlitestorage "zhatBot/internal/infrastructure/persistence/sqlite"
	twitchinfra "zhatBot/internal/infrastructure/platform/twitch"
	twitchadapter "zhatBot/internal/interface/adapters/twitch"
	ws "zhatBot/internal/interface/api/ws"
	"zhatBot/internal/interface/outs"
	categoryusecase "zhatBot/internal/usecase/category"
	"zhatBot/internal/usecase/commands"
	credentialsusecase "zhatBot/internal/usecase/credentials"
	"zhatBot/internal/usecase/handle_message"
	"zhatBot/internal/usecase/notifications"
	statususecase "zhatBot/internal/usecase/status"
	"zhatBot/internal/usecase/stream"
	ttsusecase "zhatBot/internal/usecase/tts"
)

type Options struct{}

type Runtime struct {
	ctx        context.Context
	cancel     context.CancelFunc
	cfg        *config.Config
	credStore  *sqlitestorage.CredentialStore
	refresher  *credentialsusecase.Refresher
	platform   *app.PlatformManager
	wsServer   *ws.Server
	twitchAd   *twitchadapter.Adapter
	multiOut   *outs.MultiSender
	bus        *events.Bus
	commandSvc *commands.Service
	ttsServ    *ttsusecase.Service
	ttsRunner  *ttsruntime.Runner
	wg         sync.WaitGroup
	started    bool
	status     *statususecase.Resolver
	category   *categoryusecase.Service
	dispatcher func(context.Context, domain.Message) error

	twitchMu            sync.RWMutex
	twitchCancel        context.CancelFunc
	twitchDone          chan struct{}
	twitchBotLogin      string
	twitchBotToken      string
	twitchChannels      []string
	twitchStreamerLogin string
	twitchNoticeHandler twitchadapter.UserNoticeHandler
}

func Start(ctx context.Context, _ Options) (*Runtime, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	runtimeCtx, cancel := context.WithCancel(ctx)

	cfg, err := config.Load()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("load config: %w", err)
	}

	dbPath := cfg.DatabasePath
	if strings.TrimSpace(dbPath) == "" {
		dbPath = filepath.Join("data", "zhatbot.db")
	}

	credStore, err := sqlitestorage.NewCredentialStore(dbPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("sqlite: %w", err)
	}

	categorySvc := categoryusecase.NewService(categoryusecase.Config{})
	resolver := stream.NewResolver(nil, nil)
	multiOut := outs.NewMultiSender()
	eventLogger := notifications.NewEventLogger()
	statusResolver := statususecase.NewResolver()

	customManager, err := commands.NewCustomCommandManager(runtimeCtx, credStore)
	if err != nil {
		cancel()
		credStore.Close()
		return nil, fmt.Errorf("custom commands: %w", err)
	}

	bus := events.NewBus()

	commandSvc := commands.NewService(customManager)

	run := &Runtime{
		ctx:        runtimeCtx,
		cancel:     cancel,
		cfg:        cfg,
		credStore:  credStore,
		multiOut:   multiOut,
		bus:        bus,
		commandSvc: commandSvc,
		status:     statusResolver,
		category:   categorySvc,
	}

	platformMgr := app.NewPlatformManager(app.ManagerConfig{
		Context:  runtimeCtx,
		Category: categorySvc,
		Resolver: resolver,
		Status:   statusResolver,
		MultiOut: multiOut,
		Kick: app.KickConfig{
			BroadcasterUserID: envInt("KICK_BROADCASTER_USER_ID"),
			ChatroomID:        envInt("KICK_CHATROOM_ID"),
			EventHandler:      eventLogger.HandleKickMessage,
		},
	})
	run.platform = platformMgr

	refresher := credentialsusecase.NewRefresher(
		credStore,
		credentialsusecase.TwitchConfig{
			ClientID:     cfg.TwitchClientId,
			ClientSecret: cfg.TwitchClientSecret,
		},
		credentialsusecase.KickConfig{
			ClientID:     cfg.KickClientID,
			ClientSecret: cfg.KickClientSecret,
			RedirectURI:  cfg.KickRedirectURI,
		},
	)
	refresher.RegisterHook(run.handleCredentialUpdate)
	run.refresher = refresher

	if err := refresher.RefreshAll(runtimeCtx); err != nil {
		log.Printf("error refrescando tokens: %v", err)
	}

	const refreshInterval = 1 * time.Hour
	refresher.Start(runtimeCtx, refreshInterval)

	loadInitialTokens(runtimeCtx, credStore, cfg)

	twitchCfg := twitchadapter.Config{
		Username:          cfg.TwitchUsername,
		OAuthToken:        formatTwitchOAuthToken(cfg.TwitchToken),
		Channels:          cfg.TwitchChannels,
		UserNoticeHandler: eventLogger.HandleTwitchUserNotice,
	}
	run.initTwitchState(twitchCfg)

	wsAddr := os.Getenv("CHAT_WS_ADDR")
	if wsAddr == "" {
		wsAddr = ":8080"
	}

	wsConfig := ws.Config{
		Addr:             wsAddr,
		CredentialRepo:   credStore,
		NotificationRepo: credStore,
		CredentialHook:   run.handleCredentialUpdate,
		CategoryManager:  categorySvc,
		StatusResolver:   statusResolver,
		CommandManager:   customManager,
		CommandService:   commandSvc,
	}

	if cfg.TwitchClientId != "" && cfg.TwitchClientSecret != "" && cfg.TwitchRedirectURI != "" {
		wsConfig.Twitch = &ws.TwitchOAuthConfig{
			ClientID:       cfg.TwitchClientId,
			ClientSecret:   cfg.TwitchClientSecret,
			RedirectURI:    cfg.TwitchRedirectURI,
			BotScopes:      []string{"chat:read", "chat:edit"},
			StreamerScopes: []string{"channel:manage:broadcast"},
		}
	}

	if cfg.KickClientID != "" && cfg.KickClientSecret != "" && cfg.KickRedirectURI != "" {
		wsConfig.Kick = &ws.KickOAuthConfig{
			ClientID:       cfg.KickClientID,
			ClientSecret:   cfg.KickClientSecret,
			RedirectURI:    cfg.KickRedirectURI,
			StreamerScopes: []string{"user:read", "channel:read", "channel:write", "chat:write"},
		}
	}

	wsServer := ws.NewServer(wsConfig)
	run.wsServer = wsServer

	var twitchTitleSvc domain.StreamTitleService
	var twitchAPIService domain.TwitchChannelService
	var twitchBroadcasterID string
	if cfg.TwitchClientId != "" && cfg.TwitchApiToken != "" {
		service, err := twitchinfra.NewStreamService(cfg.TwitchClientId, cfg.TwitchApiToken)
		if err != nil {
			log.Printf("no se pudo iniciar el servicio de Twitch: %v", err)
		} else {
			broadcasterID, err := resolveTwitchBroadcasterID(runtimeCtx, cfg.TwitchClientId, cfg.TwitchApiToken, cfg.TwitchUsername)
			if err != nil {
				log.Printf("no pude resolver el ID de Twitch: %v", err)
			} else {
				twitchAPIService = service
				twitchBroadcasterID = broadcasterID
				categorySvc.SetTwitchService(twitchAPIService, broadcasterID)
				twitchTitleSvc = twitchinfra.NewTwitchTitleAdapter(twitchAPIService, broadcasterID)
				statusResolver.Set(domain.PlatformTwitch, twitchinfra.NewTwitchStatusAdapter(twitchAPIService, broadcasterID))
			}
		}
	}

	if twitchTitleSvc != nil {
		resolver.Set(domain.PlatformTwitch, twitchTitleSvc)
		if twitchAPIService != nil && twitchBroadcasterID != "" {
			customManager.SetAudienceResolver(commands.NewTwitchAudienceResolver(twitchAPIService, twitchBroadcasterID))
		}
	}

	router := commands.NewRouter("!")
	router.SetCustomManager(customManager)
	router.Register(commands.NewPingCommand())
	router.Register(commands.NewManageCustomCommand(customManager))

	ttsService := ttsusecase.NewService(credStore, filepath.Join("data", "tts"))
	ttsRunner := ttsruntime.New(ttsruntime.Config{
		Service:   ttsService,
		Publisher: wsServer,
		Bus:       bus,
	})
	ttsService.SetQueue(ttsRunner)
	wsServer.SetTTSManager(ttsService)
	wsServer.SetTTSStatusProvider(ttsRunner)
	router.Register(commands.NewTTSCommand(ttsService))
	run.ttsServ = ttsService
	run.ttsRunner = ttsRunner

	router.Register(commands.NewTitleCommand(resolver))

	uc := handle_message.NewInteractor(multiOut, router)

	dispatch := func(ctx context.Context, msg domain.Message) error {
		msgNormalized := msg

		if msgNormalized.ChannelID == "" {
			switch msgNormalized.Platform {
			case domain.PlatformTwitch:
				msgNormalized.ChannelID = run.defaultTwitchChannel()
			case domain.PlatformKick:
				msgNormalized.ChannelID = platformMgr.ChannelID(domain.PlatformKick)
			}
		}

		if msgNormalized.Username == "" {
			msgNormalized.Username = "web-user"
		}

		if err := wsServer.PublishMessage(ctx, msgNormalized); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("ws publish error: %v", err)
		}

		if bus != nil {
			bus.Publish(events.TopicChatMessage, events.NewChatMessageDTO(msgNormalized))
		}

		return uc.Handle(ctx, msgNormalized)
	}
	run.dispatcher = dispatch

	wsServer.SetHandler(dispatch)
	platformMgr.SetHandler(dispatch)
	run.syncTwitchAdapter()
	run.wg.Add(1)
	go func() {
		defer run.wg.Done()
		log.Printf("Iniciando servidor WS")
		if err := wsServer.Start(runtimeCtx); err != nil && err != context.Canceled {
			log.Printf("ws server error: %v", err)
		}
	}()

	run.handleCredentialSnapshot(runtimeCtx)

	if ttsRunner != nil {
		ttsRunner.Start(runtimeCtx)
	}

	run.started = true
	log.Println("Iniciando bot...")
	return run, nil
}

func (r *Runtime) Stop() error {
	if r == nil || !r.started {
		return nil
	}
	r.cancel()
	r.stopTwitchAdapter()
	r.platform.Shutdown()
	if r.ttsRunner != nil {
		_ = r.ttsRunner.Close()
	}
	r.wg.Wait()
	if r.credStore != nil {
		if err := r.credStore.Close(); err != nil {
			return err
		}
	}
	r.started = false
	return nil
}

func (r *Runtime) Bus() *events.Bus {
	if r == nil {
		return nil
	}
	return r.bus
}

func (r *Runtime) CommandService() *commands.Service {
	if r == nil {
		return nil
	}
	return r.commandSvc
}

func (r *Runtime) TTSService() *ttsusecase.Service {
	if r == nil {
		return nil
	}
	return r.ttsServ
}

func (r *Runtime) TTSRunner() *ttsruntime.Runner {
	if r == nil {
		return nil
	}
	return r.ttsRunner
}

func (r *Runtime) NotificationRepo() domain.NotificationRepository {
	if r == nil {
		return nil
	}
	return r.credStore
}

func (r *Runtime) StreamStatusResolver() *statususecase.Resolver {
	if r == nil {
		return nil
	}
	return r.status
}

func (r *Runtime) CategoryService() *categoryusecase.Service {
	if r == nil {
		return nil
	}
	return r.category
}

func (r *Runtime) DispatchMessage(ctx context.Context, msg domain.Message) error {
	if r == nil || r.dispatcher == nil {
		return fmt.Errorf("dispatcher unavailable")
	}
	if ctx == nil {
		ctx = r.ctx
	}
	return r.dispatcher(ctx, msg)
}

func (r *Runtime) Config() *config.Config {
	if r == nil {
		return nil
	}
	return r.cfg
}

func (r *Runtime) CredentialRepo() domain.CredentialRepository {
	if r == nil {
		return nil
	}
	return r.credStore
}

func (r *Runtime) NotifyCredentialUpdate(ctx context.Context, cred *domain.Credential) {
	r.handleCredentialUpdate(ctx, cred)
}

func (r *Runtime) OAuthStart(ctx context.Context, platform domain.Platform, role string) (string, error) {
	if r == nil || r.wsServer == nil {
		return "", fmt.Errorf("oauth server unavailable")
	}
	if ctx == nil {
		ctx = r.ctx
	}
	return r.wsServer.OAuthStart(ctx, platform, role)
}

func (r *Runtime) OAuthStatus(ctx context.Context) (ws.OAuthStatus, error) {
	if r == nil || r.wsServer == nil {
		return ws.OAuthStatus{}, fmt.Errorf("oauth server unavailable")
	}
	if ctx == nil {
		ctx = r.ctx
	}
	return r.wsServer.OAuthStatus(ctx)
}

func (r *Runtime) OAuthLogout(ctx context.Context, platform domain.Platform, role string) error {
	if r == nil || r.wsServer == nil {
		return fmt.Errorf("oauth server unavailable")
	}
	if ctx == nil {
		ctx = r.ctx
	}
	return r.wsServer.OAuthLogout(ctx, platform, role)
}

func loadInitialTokens(ctx context.Context, store *sqlitestorage.CredentialStore, cfg *config.Config) {
	if store == nil {
		return
	}
	if cred, err := store.Get(ctx, domain.PlatformTwitch, "bot"); err == nil && cred != nil {
		if cred.AccessToken != "" {
			cfg.TwitchToken = cred.AccessToken
		}
		if login := strings.TrimSpace(cred.Metadata["login"]); login != "" {
			cfg.TwitchUsername = login
			if len(cfg.TwitchChannels) == 0 {
				cfg.TwitchChannels = []string{ensureTwitchChannel(login)}
			}
		}
		log.Printf("twitch: bot credential present=%v user=%s", cred.AccessToken != "", cfg.TwitchUsername)
	} else if err != nil {
		log.Printf("error obteniendo token de Twitch bot desde DB: %v", err)
	}

	if cred, err := store.Get(ctx, domain.PlatformTwitch, "streamer"); err == nil && cred != nil {
		if cred.AccessToken != "" {
			cfg.TwitchApiToken = cred.AccessToken
		}
		if cred.RefreshToken != "" {
			cfg.TwitchApiRefreshToken = cred.RefreshToken
		}
	} else if err != nil {
		log.Printf("error obteniendo token de Twitch streamer desde DB: %v", err)
	}
}

func (r *Runtime) handleCredentialSnapshot(ctx context.Context) {
	if r == nil || r.credStore == nil {
		return
	}
	creds, err := r.credStore.List(ctx)
	if err != nil {
		log.Printf("snapshot credentials error: %v", err)
		return
	}
	for _, cred := range creds {
		if cred == nil {
			continue
		}
		r.handleCredentialUpdate(ctx, cred)
	}
}

func envInt(key string) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("%s inválido (%q)", key, v)
		return 0
	}
	return n
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

func (r *Runtime) handleCredentialUpdate(ctx context.Context, cred *domain.Credential) {
	if r == nil || cred == nil {
		return
	}
	if ctx == nil {
		ctx = r.ctx
	}
	if r.platform != nil {
		r.platform.HandleCredentialUpdate(ctx, cred)
	}
	if cred.Platform == domain.PlatformTwitch {
		r.applyTwitchCredential(cred)
	}
}

func (r *Runtime) initTwitchState(cfg twitchadapter.Config) {
	r.twitchMu.Lock()
	defer r.twitchMu.Unlock()
	r.twitchBotLogin = strings.TrimSpace(cfg.Username)
	r.twitchBotToken = strings.TrimSpace(cfg.OAuthToken)
	r.twitchChannels = sanitizeTwitchChannels(cfg.Channels)
	r.twitchNoticeHandler = cfg.UserNoticeHandler
	if len(r.twitchChannels) == 0 && r.twitchBotLogin != "" {
		r.twitchChannels = []string{ensureTwitchChannel(r.twitchBotLogin)}
	}
	if r.cfg != nil {
		r.cfg.TwitchChannels = append([]string(nil), r.twitchChannels...)
	}
}

func (r *Runtime) defaultTwitchChannel() string {
	r.twitchMu.RLock()
	defer r.twitchMu.RUnlock()
	if len(r.twitchChannels) == 0 {
		return ""
	}
	return r.twitchChannels[0]
}

func (r *Runtime) applyTwitchCredential(cred *domain.Credential) {
	if cred == nil {
		return
	}
	login := strings.TrimSpace(cred.Metadata["login"])
	role := strings.ToLower(strings.TrimSpace(cred.Role))
	changed := false

	r.twitchMu.Lock()
	switch role {
	case "bot":
		token := formatTwitchOAuthToken(cred.AccessToken)
		if token != "" && token != r.twitchBotToken {
			r.twitchBotToken = token
			if r.cfg != nil {
				r.cfg.TwitchToken = cred.AccessToken
			}
			changed = true
		}
		if login != "" && !strings.EqualFold(login, r.twitchBotLogin) {
			r.twitchBotLogin = login
			if r.cfg != nil {
				r.cfg.TwitchUsername = login
			}
			changed = true
		}
		if len(r.twitchChannels) == 0 && login != "" {
			r.twitchChannels = []string{ensureTwitchChannel(login)}
			if r.cfg != nil {
				r.cfg.TwitchChannels = append([]string(nil), r.twitchChannels...)
			}
			changed = true
		}
	case "streamer":
		if login != "" && !strings.EqualFold(login, r.twitchStreamerLogin) {
			r.twitchStreamerLogin = login
			changed = true
		}
		if len(r.twitchChannels) == 0 && login != "" {
			r.twitchChannels = []string{ensureTwitchChannel(login)}
			if r.cfg != nil {
				r.cfg.TwitchChannels = append([]string(nil), r.twitchChannels...)
			}
			changed = true
		}
		if r.cfg != nil {
			if cred.AccessToken != "" {
				r.cfg.TwitchApiToken = cred.AccessToken
			}
			if cred.RefreshToken != "" {
				r.cfg.TwitchApiRefreshToken = cred.RefreshToken
			}
		}
	}
	r.twitchMu.Unlock()

	if changed {
		log.Printf("twitch: bot credential updated (user=%s channels=%v)", r.twitchBotLogin, r.twitchChannels)
		r.syncTwitchAdapter()
	}
}

func (r *Runtime) syncTwitchAdapter() {
	r.twitchMu.RLock()
	cfg := twitchadapter.Config{
		Username:          r.twitchBotLogin,
		OAuthToken:        r.twitchBotToken,
		Channels:          append([]string(nil), r.twitchChannels...),
		UserNoticeHandler: r.twitchNoticeHandler,
	}
	running := r.twitchAd != nil
	r.twitchMu.RUnlock()

	if cfg.Username == "" || cfg.OAuthToken == "" || len(cfg.Channels) == 0 {
		log.Printf("twitch: bot credential present=%v user=%s channels=%d", cfg.OAuthToken != "", cfg.Username, len(cfg.Channels))
		if running {
			r.stopTwitchAdapter()
		} else {
			log.Println("twitch: adaptador deshabilitado hasta que completes el login del bot.")
		}
		return
	}

	if running {
		r.stopTwitchAdapter()
	}
	r.startTwitchAdapter(cfg)
}

func (r *Runtime) startTwitchAdapter(cfg twitchadapter.Config) {
	if r == nil {
		return
	}
	log.Printf("twitch: starting IRC client (user=%s channels=%v)", cfg.Username, cfg.Channels)
	adapter := twitchadapter.NewAdapter(cfg)
	if handler := r.dispatcher; handler != nil {
		adapter.SetHandler(handler)
	}
	ctx, cancel := context.WithCancel(r.ctx)
	done := make(chan struct{})

	r.twitchMu.Lock()
	r.twitchAd = adapter
	r.twitchCancel = cancel
	r.twitchDone = done
	r.twitchMu.Unlock()

	if r.multiOut != nil {
		r.multiOut.Register(domain.PlatformTwitch, adapter)
	}
	r.publishTwitchConnected(cfg)

	go func() {
		defer close(done)
		if err := adapter.Start(ctx); err != nil && err != context.Canceled {
			log.Printf("twitch: adapter error: %v", err)
			r.publishTwitchError(err.Error())
		}
	}()
}

func (r *Runtime) stopTwitchAdapter() {
	r.twitchMu.Lock()
	cancel := r.twitchCancel
	done := r.twitchDone
	hasAdapter := r.twitchAd != nil
	r.twitchAd = nil
	r.twitchCancel = nil
	r.twitchDone = nil
	r.twitchMu.Unlock()

	if hasAdapter && r.multiOut != nil {
		log.Println("twitch: stopping IRC client")
		r.multiOut.Unregister(domain.PlatformTwitch)
	}
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (r *Runtime) publishTwitchConnected(cfg twitchadapter.Config) {
	if r == nil || r.bus == nil {
		return
	}
	payload := events.TwitchBotEventDTO{
		Username: cfg.Username,
		Channels: append([]string(nil), cfg.Channels...),
	}
	r.bus.Publish(events.TopicTwitchBotConnected, payload)
}

func (r *Runtime) publishTwitchError(message string) {
	if r == nil || r.bus == nil {
		return
	}
	r.twitchMu.RLock()
	payload := events.TwitchBotEventDTO{
		Username: r.twitchBotLogin,
		Channels: append([]string(nil), r.twitchChannels...),
		Message:  message,
	}
	r.twitchMu.RUnlock()
	r.bus.Publish(events.TopicTwitchBotError, payload)
}

func sanitizeTwitchChannels(input []string) []string {
	var result []string
	seen := make(map[string]struct{})
	for _, raw := range input {
		parts := strings.Split(raw, ",")
		for _, part := range parts {
			channel := ensureTwitchChannel(part)
			if channel == "" {
				continue
			}
			if _, ok := seen[channel]; ok {
				continue
			}
			seen[channel] = struct{}{}
			result = append(result, channel)
		}
	}
	return result
}

func ensureTwitchChannel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "#") {
		value = "#" + value
	}
	return strings.ToLower(value)
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
