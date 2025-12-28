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
	refresher.RegisterHook(platformMgr.HandleCredentialUpdate)
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

	wsAddr := os.Getenv("CHAT_WS_ADDR")
	if wsAddr == "" {
		wsAddr = ":8080"
	}

	wsConfig := ws.Config{
		Addr:             wsAddr,
		CredentialRepo:   credStore,
		NotificationRepo: credStore,
		CredentialHook:   platformMgr.HandleCredentialUpdate,
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

	var twitchAd *twitchadapter.Adapter
	if twitchCfg.Username == "" || twitchCfg.OAuthToken == "" {
		log.Println("twitch: adaptador deshabilitado hasta que completes el login del bot.")
	} else {
		twitchAd = twitchadapter.NewAdapter(twitchCfg)
		multiOut.Register(domain.PlatformTwitch, twitchAd)
	}
	run.twitchAd = twitchAd

	uc := handle_message.NewInteractor(multiOut, router)

	dispatch := func(ctx context.Context, msg domain.Message) error {
		msgNormalized := msg

		if msgNormalized.ChannelID == "" {
			switch msgNormalized.Platform {
			case domain.PlatformTwitch:
				if len(twitchCfg.Channels) > 0 {
					msgNormalized.ChannelID = twitchCfg.Channels[0]
				}
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
	if twitchAd != nil {
		twitchAd.SetHandler(dispatch)
	}

	run.wg.Add(1)
	go func() {
		defer run.wg.Done()
		log.Printf("Iniciando servidor WS")
		if err := wsServer.Start(runtimeCtx); err != nil && err != context.Canceled {
			log.Printf("ws server error: %v", err)
		}
	}()

	if twitchAd != nil {
		run.wg.Add(1)
		go func() {
			defer run.wg.Done()
			if err := twitchAd.Start(runtimeCtx); err != nil && err != context.Canceled {
				log.Printf("twitch adapter error: %v", err)
			}
		}()
	}

	handleCredentialSnapshot(runtimeCtx, platformMgr, credStore)

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

func loadInitialTokens(ctx context.Context, store *sqlitestorage.CredentialStore, cfg *config.Config) {
	if store == nil {
		return
	}
	if cred, err := store.Get(ctx, domain.PlatformTwitch, "bot"); err == nil && cred != nil && cred.AccessToken != "" {
		cfg.TwitchToken = cred.AccessToken
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

func handleCredentialSnapshot(ctx context.Context, mgr *app.PlatformManager, repo domain.CredentialRepository) {
	if mgr == nil || repo == nil {
		return
	}
	creds, err := repo.List(ctx)
	if err != nil {
		log.Printf("snapshot credentials error: %v", err)
		return
	}
	for _, cred := range creds {
		if cred == nil {
			continue
		}
		mgr.HandleCredentialUpdate(ctx, cred)
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
