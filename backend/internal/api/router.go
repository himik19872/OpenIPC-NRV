package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/jwtauth/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nvr/backend/internal/api/handlers"
	mw "github.com/nvr/backend/internal/api/middleware"
	miniorepo "github.com/nvr/backend/internal/repository/minio"
	"github.com/nvr/backend/internal/repository/postgres"
	"github.com/nvr/backend/internal/service"
	"github.com/nvr/backend/internal/tunnel"
)

type RouterConfig struct {
	CameraSvc    *service.CameraService
	EventSvc     *service.EventService
	ACSSvc       *service.ACSService
	UserRepo     *postgres.UserRepo
	JWTSecret    string
	WGManager    *tunnel.WireGuardManager
	DB           *pgxpool.Pool
	MediamtxHost string
	Scanner      *service.CameraScanner
	VideoRepo    *miniorepo.VideoRepo
}

func NewRouter(cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(mw.RequestLogger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// JWT
	tokenAuth := jwtauth.New("HS256", []byte(cfg.JWTSecret), nil)

	// Handlers
	authH := handlers.NewAuthHandler(cfg.UserRepo, tokenAuth)
	cameraH := handlers.NewCameraHandler(cfg.CameraSvc)
	streamH := handlers.NewStreamHandler(cfg.CameraSvc, cfg.MediamtxHost, tokenAuth)
	scannerH := handlers.NewScannerHandler(cfg.Scanner)
	ptzH := handlers.NewPTZHandler(cfg.CameraSvc)
	docsH := handlers.NewAPIDocHandler()
	eventH := handlers.NewEventHandler(cfg.EventSvc)
	acsH := handlers.NewACSHandler(cfg.ACSSvc)
	recH := handlers.NewRecordingHandler(cfg.DB, cfg.VideoRepo)
	statsH := handlers.NewStatsHandler(cfg.DB)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// Публичные
		r.Post("/auth/login", authH.Login)
		// Документация API — доступна публично, чтобы клиенты могли
		// построить интеграцию до получения учётных данных.
		r.Get("/docs", docsH.List)

		// HLS-прокси: вне JWT-группы, т.к. hls.js в браузере
		// не может передавать Authorization-заголовок для сегментов.
		// Авторизация проверяется внутри ProxyHLS по ?token= query-параметру.
		r.Get("/cameras/{id}/hls/*", streamH.ProxyHLS)

		// Снапшот: тоже вне JWT-группы, но по другой причине.
		// Кадр вставляется тегом <img> (превью в списке камер), а <img>
		// не умеет отправлять заголовок Authorization. Токен проверяется
		// внутри GetSnapshot из ?jwt= или ?token=.
		r.Get("/cameras/{id}/snapshot", streamH.GetSnapshot)

		// Защищённые
		r.Group(func(r chi.Router) {
			r.Use(jwtauth.Verifier(tokenAuth))
			r.Use(jwtauth.Authenticator(tokenAuth))

			// Камеры
			r.Get("/cameras", cameraH.List)
			r.Post("/cameras", cameraH.Create)
			r.Get("/cameras/{id}", cameraH.Get)
			r.Patch("/cameras/{id}", cameraH.Update)
			r.Delete("/cameras/{id}", cameraH.Delete)

			// Стримы
			r.Get("/cameras/{id}/stream", streamH.GetStream)
			// (snapshot зарегистрирован выше, вне JWT-группы)

			// PTZ (поворотные камеры, ONVIF)
			r.Get("/cameras/{id}/ptz/status", ptzH.Status)
			r.Post("/cameras/{id}/ptz/move", ptzH.Move)
			r.Post("/cameras/{id}/ptz/stop", ptzH.Stop)
			r.Get("/cameras/{id}/ptz/presets", ptzH.Presets)
			r.Post("/cameras/{id}/ptz/presets/goto", ptzH.GotoPreset)

			// Управление камерой (OpenIPC: Majestic + reboot)
			r.Post("/cameras/{id}/restart-streamer", cameraH.RestartStreamer)
			r.Post("/cameras/{id}/reboot", cameraH.Reboot)

			// События
			r.Get("/events", eventH.List)
			r.Get("/events/{id}", eventH.Get)

			// Записи
			r.Get("/recordings", recH.List)
			r.Get("/recordings/{id}", recH.Get)
			r.Delete("/recordings/{id}", recH.Delete)

			// СКУД
			r.Get("/acs/controllers", acsH.ListControllers)
			r.Post("/acs/controllers", acsH.CreateController)
			r.Get("/acs/controllers/{id}", acsH.GetController)
			r.Delete("/acs/controllers/{id}", acsH.DeleteController)
			r.Get("/acs/events", acsH.ListEvents)
			r.Post("/acs/doors/{controllerID}/open", acsH.OpenDoor)

			// Статистика
			r.Get("/stats", statsH.Get)

			// Сканер камер
			r.Post("/scanner/scan", scannerH.Scan)
			r.Post("/scanner/probe", scannerH.Probe)
		})
	})

	return r
}
