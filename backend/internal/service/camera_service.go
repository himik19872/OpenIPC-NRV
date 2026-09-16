package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nvr/backend/internal/domain"
	"github.com/nvr/backend/internal/repository/postgres"
	"github.com/rs/zerolog/log"
)

type CameraService struct {
	repo        *postgres.CameraRepo
	mediamtxAPI string // MediaMTX API base URL, например "http://localhost:9997"
	client      *http.Client
	ssh         *CameraSSH
	ptz         *PTZServiceClient
}

func NewCameraService(repo *postgres.CameraRepo, mediamtxAPI string) *CameraService {
	if mediamtxAPI == "" {
		mediamtxAPI = "http://localhost:9997"
	}
	return &CameraService{
		repo:        repo,
		mediamtxAPI: mediamtxAPI,
		client:      &http.Client{Timeout: 5 * time.Second},
		ssh:         NewCameraSSH(),
		ptz:         NewPTZServiceClient(),
	}
}

// --- PTZ (ONVIF) ---

// PTZStatus возвращает текущее положение поворотной камеры.
// Если камера не поддерживает ONVIF PTZ, вернётся supports_ptz=false.
func (s *CameraService) PTZStatus(ctx context.Context, id uuid.UUID) (*PTZStatus, error) {
	cam, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	username, password := credentialsFromSettings(cam.Settings)

	st, err := s.ptz.GetStatus(ctx, cam.IP, username, password)
	if err != nil {
		// Не все камеры поворотные — это не ошибка сервера,
		// а признак отсутствия поддержки.
		return &PTZStatus{Supports: false}, nil
	}
	return st, nil
}

// PTZMove запускает движение камеры на заданной скорости.
func (s *CameraService) PTZMove(ctx context.Context, id uuid.UUID, pan, tilt, zoom float64, durationMs int) error {
	cam, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	username, password := credentialsFromSettings(cam.Settings)

	// Ограничиваем скорость и время: защита от случайного «улёта» камеры.
	pan = clampFloat(pan, -1, 1)
	tilt = clampFloat(tilt, -1, 1)
	zoom = clampFloat(zoom, -1, 1)
	duration := time.Duration(clampInt(durationMs, 100, 5000)) * time.Millisecond

	return s.ptz.Move(ctx, cam.IP, username, password, pan, tilt, zoom, duration)
}

// PTZStop останавливает движение камеры.
func (s *CameraService) PTZStop(ctx context.Context, id uuid.UUID) error {
	cam, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	username, password := credentialsFromSettings(cam.Settings)
	return s.ptz.Stop(ctx, cam.IP, username, password)
}

// PTZPresets возвращает список сохранённых позиций.
func (s *CameraService) PTZPresets(ctx context.Context, id uuid.UUID) ([]PTZPreset, error) {
	cam, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	username, password := credentialsFromSettings(cam.Settings)
	return s.ptz.GetPresets(ctx, cam.IP, username, password)
}

// PTZGotoPreset переходит к сохранённой позиции.
func (s *CameraService) PTZGotoPreset(ctx context.Context, id uuid.UUID, presetToken string) error {
	cam, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	username, password := credentialsFromSettings(cam.Settings)
	return s.ptz.GotoPreset(ctx, cam.IP, username, password, presetToken)
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// RestartStreamer перезапускает стример камеры (Majestic), после чего
// пересоздаёт путь в MediaMTX, чтобы поток поднялся без ожидания.
func (s *CameraService) RestartStreamer(ctx context.Context, id uuid.UUID) (*CommandResult, error) {
	cam, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	username, password := credentialsFromSettings(cam.Settings)
	res, err := s.ssh.RestartMajestic(ctx, cam.IP, username, password)
	if err != nil {
		return res, err
	}

	// Пересоздаём путь: MediaMTX сам переподключится к перезапущенному RTSP.
	go func() {
		time.Sleep(2 * time.Second)
		s.reconnectStream(cam)
	}()

	return res, nil
}

// RebootCamera перезагружает камеру.
func (s *CameraService) RebootCamera(ctx context.Context, id uuid.UUID) (*CommandResult, error) {
	cam, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	username, password := credentialsFromSettings(cam.Settings)
	res, err := s.ssh.Reboot(ctx, cam.IP, username, password)
	if err != nil {
		return res, err
	}

	// После перезагрузки камера вернётся примерно через минуту —
	// пересоздаём путь, чтобы поток восстановился автоматически.
	go func() {
		time.Sleep(60 * time.Second)
		s.reconnectStream(cam)
	}()

	return res, nil
}

// reconnectStream пересоздаёт пути камеры в MediaMTX.
func (s *CameraService) reconnectStream(cam *domain.Camera) {
	_ = s.removeMediaMTXPath(cam.ID.String())
	_ = s.removeMediaMTXPath(cam.ID.String() + "_sub")
	time.Sleep(500 * time.Millisecond)
	s.registerStreams(cam, "", "")
	log.Info().Str("camera", cam.Name).Str("ip", cam.IP).Msg("stream path recreated")
}

// credentialsFromSettings извлекает логин/пароль камеры из settings.
func credentialsFromSettings(settings map[string]any) (string, string) {
	username, password := "", ""
	if settings == nil {
		return username, password
	}
	if u, ok := settings["username"].(string); ok {
		username = u
	}
	if p, ok := settings["password"].(string); ok {
		password = p
	}
	return username, password
}

func (s *CameraService) List(ctx context.Context) ([]domain.Camera, error) {
	return s.repo.List(ctx)
}

func (s *CameraService) Get(ctx context.Context, id uuid.UUID) (*domain.Camera, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *CameraService) Create(ctx context.Context, req domain.CreateCameraRequest) (*domain.Camera, error) {
	cam := &domain.Camera{
		ID:         uuid.New(),
		Name:       req.Name,
		RTSPUrl:    req.RTSPUrl,
		MainStream: req.MainStream,
		SubStream:  req.SubStream,
		IP:         req.IP,
		MAC:        req.MAC,
		Firmware:   req.Firmware,
		Status:     "offline",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if req.WGIP != "" {
		cam.WGIP = req.WGIP
	}
	if req.SiteID != "" {
		siteID, err := uuid.Parse(req.SiteID)
		if err == nil {
			cam.SiteID = &siteID
		}
	}
	// Сохраняем креды камеры в settings (JSONB)
	if req.Username != "" || req.Password != "" {
		if cam.Settings == nil {
			cam.Settings = make(map[string]any)
		}
		cam.Settings["username"] = req.Username
		cam.Settings["password"] = req.Password
	}
	// Признак поддержки PTZ храним в settings, чтобы не менять схему БД.
	if req.PTZ {
		if cam.Settings == nil {
			cam.Settings = make(map[string]any)
		}
		cam.Settings["ptz"] = true
		cam.PTZ = true
	}

	if err := s.repo.Create(ctx, cam); err != nil {
		return nil, err
	}

	// Регистрируем RTSP-источники в MediaMTX
	s.registerStreams(cam, req.Username, req.Password)

	return cam, nil
}

// registerStreams регистрирует основной и дополнительный потоки камеры в MediaMTX.
// Креды берутся из settings, если не переданы явно.
func (s *CameraService) registerStreams(cam *domain.Camera, username, password string) {
	if username == "" && password == "" && cam.Settings != nil {
		if u, ok := cam.Settings["username"].(string); ok {
			username = u
		}
		if p, ok := cam.Settings["password"].(string); ok {
			password = p
		}
	}

	mainRTSP := cam.MainStream
	if mainRTSP == "" {
		mainRTSP = cam.RTSPUrl
	}
	if mainRTSP != "" {
		go s.addMediaMTXPath(cam.ID.String(), embedCredentials(mainRTSP, username, password))
	}
	if cam.SubStream != "" {
		go s.addMediaMTXPath(cam.ID.String()+"_sub", embedCredentials(cam.SubStream, username, password))
	}
}

// RestoreStreams перерегистрирует пути всех камер в MediaMTX.
// Нужно после старта сервера, если MediaMTX был перезапущен и потерял конфигурацию
// (пути хранятся в памяти MediaMTX и не сохраняются между перезапусками).
func (s *CameraService) RestoreStreams(ctx context.Context) {
	cameras, err := s.repo.List(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to list cameras for stream restore")
		return
	}

	restored := 0
	for i := range cameras {
		cam := cameras[i]
		mainRTSP := cam.MainStream
		if mainRTSP == "" {
			mainRTSP = cam.RTSPUrl
		}
		if mainRTSP == "" {
			continue
		}
		s.registerStreams(&cam, "", "")
		restored++
	}
	log.Info().Int("cameras", restored).Msg("MediaMTX streams restore requested")
}

// embedCredentials вставляет логин/пароль в RTSP URL (rtsp://user:pass@host/...)
func embedCredentials(rtspURL, username, password string) string {
	if rtspURL == "" || (username == "" && password == "") {
		return rtspURL
	}
	// Если креды уже есть в URL — не трогаем
	if strings.Contains(rtspURL, "@") {
		return rtspURL
	}
	// Вставляем user:pass@ после rtsp://
	if strings.HasPrefix(rtspURL, "rtsp://") {
		rest := strings.TrimPrefix(rtspURL, "rtsp://")
		creds := username
		if password != "" {
			creds += ":" + password
		}
		return "rtsp://" + creds + "@" + rest
	}
	return rtspURL
}

// addMediaMTXPath регистрирует RTSP-источник в MediaMTX
func (s *CameraService) addMediaMTXPath(pathName, rtspSource string) {
	// MediaMTX API: POST /v3/config/paths/add/{name}
	url := fmt.Sprintf("%s/v3/config/paths/add/%s", s.mediamtxAPI, pathName)
	payload := map[string]interface{}{
		"name":           pathName,
		"source":         rtspSource,
		"sourceOnDemand": false,
		// TCP исключает потери RTP-пакетов: для 4K-потоков (3840x2160) UDP
		// не справляется, и HLS-муксер MediaMTX падает с
		// "unable to extract DTS: too many reordered frames".
		"rtspTransport": "tcp",
	}
	body, _ := json.Marshal(payload)

	resp, err := s.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Warn().Err(err).Str("path", pathName).Str("source", rtspSource).Msg("failed to add MediaMTX path")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Info().Str("path", pathName).Str("source", rtspSource).Msg("MediaMTX path added")
	} else {
		log.Warn().Str("path", pathName).Int("status", resp.StatusCode).Msg("MediaMTX returned non-OK status")
	}
}

// removeMediaMTXPath удаляет путь из MediaMTX.
// В MediaMTX v3 используется DELETE /v3/config/paths/delete/{name}.
// (POST .../remove/{name} не существует и возвращает 404.)
func (s *CameraService) removeMediaMTXPath(pathName string) error {
	url := fmt.Sprintf("%s/v3/config/paths/delete/%s", s.mediamtxAPI, pathName)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		log.Warn().Err(err).Str("path", pathName).Msg("failed to remove MediaMTX path")
		return err
	}
	defer resp.Body.Close()

	// 404 означает, что пути и так нет — для нас это успех (идемпотентность).
	if resp.StatusCode == http.StatusNotFound {
		log.Debug().Str("path", pathName).Msg("MediaMTX path already absent")
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("mediamtx returned status %d", resp.StatusCode)
		log.Warn().Err(err).Str("path", pathName).Msg("failed to remove MediaMTX path")
		return err
	}

	log.Info().Str("path", pathName).Msg("MediaMTX path removed")
	return nil
}

func (s *CameraService) Update(ctx context.Context, id uuid.UUID, req domain.UpdateCameraRequest) (*domain.Camera, error) {
	cam, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	changed := false
	if req.Name != nil {
		cam.Name = *req.Name
	}
	if req.RTSPUrl != nil {
		cam.RTSPUrl = *req.RTSPUrl
		changed = true
	}
	if req.MainStream != nil {
		cam.MainStream = *req.MainStream
		changed = true
	}
	if req.SubStream != nil {
		cam.SubStream = *req.SubStream
		changed = true
	}
	if req.IP != nil {
		cam.IP = *req.IP
	}
	if req.MAC != nil {
		cam.MAC = *req.MAC
	}
	if req.Firmware != nil {
		cam.Firmware = *req.Firmware
	}
	if req.Status != nil {
		cam.Status = *req.Status
	}
	// Обновляем креды в settings
	if req.Username != nil || req.Password != nil {
		if cam.Settings == nil {
			cam.Settings = make(map[string]any)
		}
		if req.Username != nil {
			cam.Settings["username"] = *req.Username
		}
		if req.Password != nil {
			cam.Settings["password"] = *req.Password
		}
		changed = true
	}
	// Переключение поддержки PTZ
	if req.PTZ != nil {
		if cam.Settings == nil {
			cam.Settings = make(map[string]any)
		}
		cam.Settings["ptz"] = *req.PTZ
		cam.PTZ = *req.PTZ
	}

	cam.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, cam); err != nil {
		return nil, err
	}

	// Если изменились потоки или креды — перерегистрируем в MediaMTX.
	// Удаляем синхронно, чтобы не было гонки: add после remove внутри MediaMTX.
	if changed {
		_ = s.removeMediaMTXPath(cam.ID.String())
		_ = s.removeMediaMTXPath(cam.ID.String() + "_sub")

		username := ""
		password := ""
		if cam.Settings != nil {
			if u, ok := cam.Settings["username"].(string); ok {
				username = u
			}
			if p, ok := cam.Settings["password"].(string); ok {
				password = p
			}
		}
		mainRTSP := cam.MainStream
		if mainRTSP == "" {
			mainRTSP = cam.RTSPUrl
		}
		mainRTSP = embedCredentials(mainRTSP, username, password)
		if mainRTSP != "" {
			go s.addMediaMTXPath(cam.ID.String(), mainRTSP)
		}
		if cam.SubStream != "" {
			subRTSP := embedCredentials(cam.SubStream, username, password)
			go s.addMediaMTXPath(cam.ID.String()+"_sub", subRTSP)
		}
	}

	return cam, nil
}

func (s *CameraService) Delete(ctx context.Context, id uuid.UUID) error {
	// Пути удаляем синхронно и до удаления записи из БД: так мы гарантируем,
	// что не останется висячих путей, даже если запрос прервётся.
	// Ошибки логируются внутри removeMediaMTXPath и не блокируют удаление.
	_ = s.removeMediaMTXPath(id.String())
	_ = s.removeMediaMTXPath(id.String() + "_sub")
	return s.repo.Delete(ctx, id)
}
