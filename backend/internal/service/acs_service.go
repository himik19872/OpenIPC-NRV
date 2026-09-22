package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nvr/backend/internal/domain"
	"github.com/nvr/backend/internal/repository/postgres"
	"github.com/nvr/backend/internal/service/acs"
	"github.com/rs/zerolog/log"
)

type ACSService struct {
	manager    *acs.Manager
	repo       *postgres.ACSRepo
	cardRepo   *postgres.ACSCardRepo
	cameraRepo *postgres.CameraRepo
	eventRepo  *postgres.EventRepo

	// cameraSvc даёт RTSP-поток камеры для съёмки по событию доступа.
	cameraSvc *CameraService
	// recordingMgr и recorderSvc используются при съёмке клипа.
	// Задаются отдельно, потому что запись видео инициализируется позже
	// сервиса СКУД и не нужна для снимков.
	recordingMgr *RecordingManager
	recorderSvc  *RecorderService
	// storageSvc сохраняет снимки событий.
	storageSvc *StorageService

	// Сбор событий: по одной отменяемой подписке на каждый контроллер.
	subMu    sync.Mutex
	subs     map[uuid.UUID]context.CancelFunc
	stopOnce sync.Once

	// Кэш проверок доступности: время последнего опроса каждого контроллера.
	statusMu sync.Mutex
	statusAt map[uuid.UUID]time.Time

	// Ожидание сохранения клипов: менеджер записи не возвращает
	// идентификатор записи, поэтому связь устанавливается по факту сохранения.
	pendingMu    sync.Mutex
	pendingClips map[uuid.UUID][]pendingClip
}

// statusTTL — как долго доверять результату проверки доступности.
// Короткий интервал даёт свежий статус, но не превращает каждое
// обновление страницы в опрос всех контроллеров.
const statusTTL = 15 * time.Second

func NewACSService(manager *acs.Manager, cardRepo *postgres.ACSCardRepo, cameraRepo *postgres.CameraRepo, eventRepo *postgres.EventRepo) *ACSService {
	return &ACSService{
		manager:    manager,
		repo:       manager.Repo(),
		cardRepo:   cardRepo,
		cameraRepo: cameraRepo,
		eventRepo:  eventRepo,
		subs:       make(map[uuid.UUID]context.CancelFunc),
	}
}

// WithCapture подключает сервисы, нужные для съёмки по событиям доступа.
//
// Вызывается после инициализации записи видео: она создаётся позже сервиса
// СКУД, а снимки работают и без неё.
func (s *ACSService) WithCapture(cameraSvc *CameraService, storageSvc *StorageService,
	recorderSvc *RecorderService, recordingMgr *RecordingManager) *ACSService {
	s.cameraSvc = cameraSvc
	s.storageSvc = storageSvc
	s.recorderSvc = recorderSvc
	s.recordingMgr = recordingMgr
	return s
}

// StartEventCollectors запускает фоновый сбор событий со всех контроллеров.
// Вызывается один раз при старте сервера: адаптеры умеют отдавать поток
// событий, но без этого вызова он никем не читался.
func (s *ACSService) StartEventCollectors(ctx context.Context) {
	controllers, err := s.repo.List(ctx)
	if err != nil {
		log.Error().Err(err).Msg("не удалось получить контроллеры СКУД для подписки")
		return
	}
	for _, ctrl := range controllers {
		s.subscribeController(ctx, ctrl)
	}
	log.Info().Int("контроллеров", len(controllers)).Msg("сбор событий СКУД запущен")
}

// subscribeController подписывается на события одного контроллера и пишет их
// в БД. Переподключение — с задержкой, чтобы недоступный контроллер не
// превратился в busy-loop.
func (s *ACSService) subscribeController(parent context.Context, ctrl domain.ACSController) {
	s.subMu.Lock()
	if _, exists := s.subs[ctrl.ID]; exists {
		s.subMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.subs[ctrl.ID] = cancel
	s.subMu.Unlock()

	go func() {
		defer func() {
			s.subMu.Lock()
			delete(s.subs, ctrl.ID)
			s.subMu.Unlock()
		}()

		for ctx.Err() == nil {
			adapter, err := s.manager.GetAdapter(ctrl.Vendor, &ctrl)
			if err != nil {
				log.Error().Err(err).Str("vendor", ctrl.Vendor).
					Msg("нет адаптера для контроллера СКУД")
				return
			}

			ch, err := adapter.SubscribeEvents(ctx)
			if err != nil {
				log.Warn().Err(err).Str("контроллер", ctrl.Name).
					Msg("не удалось подписаться на события СКУД, повтор через 30с")
				if !sleepCtx(ctx, 30*time.Second) {
					return
				}
				continue
			}

			func() {
				for {
					select {
					case <-ctx.Done():
						return
					case ev, ok := <-ch:
						if !ok {
							return
						}
						ev.ControllerID = ctrl.ID

						// Настройки перечитываем из БД: контроллер мог быть
						// отредактирован после старта подписки (сменили
						// камеру, режим съёмки или адрес), а в горутине
						// лежит копия на момент запуска.
						fresh := ctrl
						if c, err := s.repo.GetByID(ctx, ctrl.ID); err == nil {
							fresh = *c
						}

						// Камеру из настроек контроллера привязываем к
						// событию: по ней потом открывается запись.
						ev.CameraID = fresh.CameraID

						if err := s.repo.CreateEvent(ctx, &ev); err != nil {
							log.Error().Err(err).Str("тип", ev.EventType).
								Msg("не удалось сохранить событие СКУД")
						} else {
							log.Info().Str("контроллер", fresh.Name).
								Str("событие", ev.EventType).
								Str("карта", ev.CardNumber).
								Msg("событие СКУД")

							// Съёмка идёт в фоне и не задерживает опрос
							// журнала: клип собирается несколько секунд.
							s.CaptureForEvent(ev, fresh)
						}
					}
				}
			}()

			if !sleepCtx(ctx, 30*time.Second) {
				return
			}
		}
	}()
}

// sleepCtx ждёт указанный интервал или завершения контекста.
// Возвращает false, если контекст отменён.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// Stop останавливает все подписки на события СКУД.
func (s *ACSService) Stop() {
	s.stopOnce.Do(func() {
		s.subMu.Lock()
		defer s.subMu.Unlock()
		for id, cancel := range s.subs {
			cancel()
			delete(s.subs, id)
		}
	})
}

func (s *ACSService) ListControllers(ctx context.Context) ([]domain.ACSController, error) {
	controllers, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	// Статус в БД выставляется только при создании, поэтому сам по себе он
	// всегда «offline». Опросим контроллеры, чтобы интерфейс показывал
	// реальное состояние, а не застывшее значение из базы.
	s.refreshStatuses(ctx, controllers)
	return controllers, nil
}

// refreshStatuses опрашивает контроллеры параллельно и обновляет статус.
//
// Результат кэшируется на statusTTL: список контроллеров запрашивается
// часто при обновлении страницы, а лишние обращения к устройствам в
// локальной сети ни к чему. Контроллеры, к которым обратиться не удалось,
// считаются офлайн.
func (s *ACSService) refreshStatuses(ctx context.Context, controllers []domain.ACSController) {
	if len(controllers) == 0 {
		return
	}

	var wg sync.WaitGroup
	for i := range controllers {
		ctrl := &controllers[i]

		s.statusMu.Lock()
		last, ok := s.statusAt[ctrl.ID]
		s.statusMu.Unlock()
		if ok && time.Since(last) < statusTTL {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			// Свой таймаут на контроллер: недоступное устройство не должно
			// задерживать ответ по остальным.
			pingCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
			defer cancel()

			status := "offline"
			if adapter, err := s.manager.GetAdapter(ctrl.Vendor, ctrl); err == nil {
				if err := adapter.Ping(pingCtx); err == nil {
					status = "online"
				}
			}

			s.repo.SetStatus(pingCtx, ctrl.ID, status)

			s.statusMu.Lock()
			if s.statusAt == nil {
				s.statusAt = make(map[uuid.UUID]time.Time)
			}
			s.statusAt[ctrl.ID] = time.Now()
			s.statusMu.Unlock()

			ctrl.Status = status
		}()
	}
	wg.Wait()
}

func (s *ACSService) GetController(ctx context.Context, id uuid.UUID) (*domain.ACSController, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *ACSService) CreateController(ctx context.Context, req domain.CreateACSControllerRequest) (*domain.ACSController, error) {
	ctrl := &domain.ACSController{
		ID:        uuid.New(),
		Name:      req.Name,
		Vendor:    req.Vendor,
		IP:        req.IP,
		Port:      req.Port,
		Status:    "offline",
		CreatedAt: time.Now(),
		Credentials: map[string]any{
			"login":    req.Login,
			"password": req.Password,
		},
	}
	if req.SiteID != "" {
		siteID, err := uuid.Parse(req.SiteID)
		if err == nil {
			ctrl.SiteID = &siteID
		}
	}
	if err := s.repo.Create(ctx, ctrl); err != nil {
		return nil, err
	}
	return ctrl, nil
}

func (s *ACSService) DeleteController(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

// UpdateController изменяет параметры контроллера.
//
// Пароль применяется только при непустом значении: интерфейс не отдаёт
// пароль обратно, поэтому при сохранении без правки поля пришла бы пустая
// строка, которая затёрла бы рабочий пароль. Пустое значение означает
// «оставить прежний».
func (s *ACSService) UpdateController(ctx context.Context, id uuid.UUID, req domain.UpdateACSControllerRequest) (*domain.ACSController, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("не задано имя контроллера")
	}
	if req.IP == "" {
		return nil, fmt.Errorf("не задан IP-адрес контроллера")
	}
	if req.Port < 1 || req.Port > 65535 {
		return nil, fmt.Errorf("порт должен быть от 1 до 65535")
	}

	ctrl, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("контроллер не найден: %w", err)
	}

	ctrl.Name = req.Name
	ctrl.IP = req.IP
	ctrl.Port = req.Port
	if req.SiteID != "" {
		if siteID, err := uuid.Parse(req.SiteID); err == nil {
			ctrl.SiteID = &siteID
		}
	}
	if req.Login != "" || req.Password != "" {
		login := req.Login
		if login == "" {
			login = "admin"
		}
		ctrl.Credentials = map[string]any{
			"login":    login,
			"password": req.Password,
		}
	}

	// Привязка камеры: пустая строка отвязывает камеру и выключает съёмку,
	// потому что без камеры снимать нечего.
	if req.CameraID == "" {
		ctrl.CameraID = nil
	} else {
		camID, err := uuid.Parse(req.CameraID)
		if err != nil {
			return nil, fmt.Errorf("неверный идентификатор камеры")
		}
		if _, err := s.cameraRepo.GetByID(ctx, camID); err != nil {
			return nil, fmt.Errorf("камера не найдена")
		}
		ctrl.CameraID = &camID
	}

	switch req.CaptureMode {
	case "", "off":
		ctrl.CaptureMode = "off"
	case "snapshot", "clip":
		ctrl.CaptureMode = req.CaptureMode
	default:
		return nil, fmt.Errorf("неверный режим съёмки: %s", req.CaptureMode)
	}

	// Без камеры съёмка невозможна — не даём сохранить несогласованное
	// состояние, иначе оператор будет ждать снимков, которых не будет.
	if ctrl.CaptureMode != "off" && ctrl.CameraID == nil {
		return nil, fmt.Errorf("для съёмки по событиям нужно выбрать камеру")
	}

	ctrl.CaptureEvents = req.CaptureEvents
	if ctrl.CaptureEvents == nil {
		ctrl.CaptureEvents = []string{}
	}
	for _, e := range ctrl.CaptureEvents {
		if _, ok := captureEvents[e]; !ok {
			return nil, fmt.Errorf("неизвестное событие для съёмки: %s", e)
		}
	}

	ctrl.ClipSeconds = req.ClipSeconds
	if ctrl.ClipSeconds == 0 {
		ctrl.ClipSeconds = 5
	}
	if ctrl.ClipSeconds < 1 || ctrl.ClipSeconds > 60 {
		return nil, fmt.Errorf("длительность клипа должна быть от 1 до 60 секунд")
	}

	if err := s.repo.Update(ctx, ctrl, req.Password); err != nil {
		return nil, fmt.Errorf("сохранить контроллер: %w", err)
	}

	// Сменились адрес, порт или пароль — прежний кэш статуса неактуален,
	// иначе интерфейс показывал бы старое состояние до истечения TTL.
	s.statusMu.Lock()
	delete(s.statusAt, id)
	s.statusMu.Unlock()

	log.Info().Str("контроллер", ctrl.Name).Str("ip", ctrl.IP).
		Msg("параметры контроллера СКУД обновлены")
	return ctrl, nil
}

func (s *ACSService) ListEvents(ctx context.Context, page, pageSize int) ([]domain.ACSEvent, int64, error) {
	return s.repo.ListEvents(ctx, page, pageSize)
}

func (s *ACSService) OpenDoor(ctx context.Context, controllerID uuid.UUID, doorID string) error {
	ctrl, err := s.repo.GetByID(ctx, controllerID)
	if err != nil {
		return fmt.Errorf("controller not found: %w", err)
	}

	adapter, err := s.manager.GetAdapter(ctrl.Vendor, ctrl)
	if err != nil {
		return fmt.Errorf("no adapter for vendor %s: %w", ctrl.Vendor, err)
	}

	return adapter.OpenDoor(ctx, doorID)
}
