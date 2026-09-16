package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nvr/backend/internal/api"
	"github.com/nvr/backend/internal/config"
	natspkg "github.com/nvr/backend/internal/nats"
	miniorepo "github.com/nvr/backend/internal/repository/minio"
	"github.com/nvr/backend/internal/repository/postgres"
	"github.com/nvr/backend/internal/service"
	"github.com/nvr/backend/internal/service/acs"
	"github.com/nvr/backend/internal/tunnel"
	"github.com/nvr/backend/pkg/logger"
	"github.com/rs/zerolog/log"
)

func main() {
	// Загрузка конфигурации
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	// Инициализация логгера
	logger.Init(cfg.LogLevel, cfg.LogFormat)

	// Подключение к БД
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	// Запуск миграций
	if err := postgres.RunMigrations(db, "file://migrations"); err != nil {
		log.Fatal().Err(err).Msg("failed to run migrations")
	}

	// Инициализация репозиториев
	cameraRepo := postgres.NewCameraRepo(db)
	eventRepo := postgres.NewEventRepo(db)
	acsRepo := postgres.NewACSRepo(db)
	userRepo := postgres.NewUserRepo(db)

	// Инициализация сервисов
	// MediaMTX API: если MEDIAMTX_HOST = "localhost:8888", то API = "http://localhost:9997"
	mediamtxAPI := "http://" + strings.Replace(cfg.MediamtxHost, "8888", "9997", 1)
	cameraSvc := service.NewCameraService(cameraRepo, mediamtxAPI)
	eventSvc := service.NewEventService(eventRepo)

	// СКУД-адаптеры
	acsManager := acs.NewManager(acsRepo)
	acsSvc := service.NewACSService(acsManager, cameraRepo, eventRepo)

	// WireGuard-менеджер (если указан интерфейс)
	var wgManager *tunnel.WireGuardManager
	if cfg.WGInterface != "" {
		wgManager, err = tunnel.NewWireGuardManager(cfg.WGInterface)
		if err != nil {
			log.Warn().Err(err).Str("iface", cfg.WGInterface).Msg("wireguard manager init failed, continuing without")
		}
	}

	// NATS Detection Subscriber (сохраняет AI детекции в БД)
	if cfg.NatsURL != "" {
		detSub, err := natspkg.NewDetectionSubscriber(cfg.NatsURL, db)
		if err != nil {
			log.Warn().Err(err).Str("url", cfg.NatsURL).Msg("nats detection subscriber init failed, continuing without")
		} else {
			go func() {
				if err := detSub.Start(context.Background()); err != nil {
					log.Error().Err(err).Msg("nats detection subscriber failed")
				}
			}()
			defer detSub.Close()
		}
	}

	// Camera scanner
	scanner := service.NewCameraScanner()

	// MinIO — архив видеозаписей. Не критичен для работы: если хранилище
	// недоступно, сервис стартует и продолжает отдавать live-потоки.
	var videoRepo *miniorepo.VideoRepo
	videoRepo, err = miniorepo.NewVideoRepo(
		cfg.MinioEndpoint, cfg.MinioPublicEndpoint,
		cfg.MinioAccessKey, cfg.MinioSecretKey, cfg.MinioBucket, cfg.MinioUseSSL,
	)
	if err != nil {
		log.Warn().Err(err).
			Str("endpoint", cfg.MinioEndpoint).
			Msg("minio init failed, recordings archive will be unavailable")
		videoRepo = nil
	}

	// Восстанавливаем пути камер в MediaMTX: они хранятся в памяти MediaMTX
	// и теряются при его перезапуске, поэтому перерегистрируем их при старте.
	cameraSvc.RestoreStreams(context.Background())

	// Монитор доступности камер: опрашивает MediaMTX и обновляет status в БД
	statusMonitor := service.NewCameraStatusMonitor(cameraRepo, mediamtxAPI)
	go statusMonitor.Start(context.Background())

	// Инициализация роутера
	router := api.NewRouter(api.RouterConfig{
		CameraSvc:    cameraSvc,
		EventSvc:     eventSvc,
		ACSSvc:       acsSvc,
		UserRepo:     userRepo,
		JWTSecret:    cfg.JWTSecret,
		WGManager:    wgManager,
		DB:           db,
		MediamtxHost: cfg.MediamtxHost,
		Scanner:      scanner,
		VideoRepo:    videoRepo,
	})

	// HTTP-сервер
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info().Int("port", cfg.Port).Msg("server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	<-quit
	log.Info().Msg("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("server forced to shutdown")
	}

	log.Info().Msg("server stopped")
}
