package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nvr/backend/internal/domain"
	"github.com/nvr/backend/internal/repository/postgres"
	"github.com/rs/zerolog/log"
)

// CameraStatusMonitor периодически опрашивает MediaMTX и синхронизирует
// поле status камер в БД с реальным состоянием потоков.
//
// MediaMTX считает путь "готовым" (ready=true), когда источник подключён
// и отдаёт хотя бы один трек. Это и есть критерий online.
type CameraStatusMonitor struct {
	repo        *postgres.CameraRepo
	mediamtxAPI string // http://host:9997
	client      *http.Client
	interval    time.Duration
	wasOnline   map[string]bool // предыдущее состояние — чтобы логировать переходы
	pruning     map[string]bool // пути, удаление которых уже не удалось (не повторяем)
	// onRestore перерегистрирует пути камеры в MediaMTX. Задан функцией,
	// чтобы монитор не зависел от сервиса камер напрямую.
	onRestore func(cam domain.Camera) error
}

// WithRestore подключает восстановление пропавших путей камер.
// Без него рестарт MediaMTX оставит камеры без потока.
func (m *CameraStatusMonitor) WithRestore(fn func(cam domain.Camera) error) *CameraStatusMonitor {
	m.onRestore = fn
	return m
}

func NewCameraStatusMonitor(repo *postgres.CameraRepo, mediamtxAPI string) *CameraStatusMonitor {
	if mediamtxAPI == "" {
		mediamtxAPI = "http://localhost:9997"
	}
	return &CameraStatusMonitor{
		repo:        repo,
		mediamtxAPI: mediamtxAPI,
		client:      &http.Client{Timeout: 5 * time.Second},
		interval:    15 * time.Second,
		wasOnline:   make(map[string]bool),
		pruning:     make(map[string]bool),
	}
}

// Start запускает цикл мониторинга в текущей горутине. Блокируется до отмены ctx.
func (m *CameraStatusMonitor) Start(ctx context.Context) {
	// Первый прогон сразу, чтобы статусы обновились при старте сервера.
	m.syncOnce(ctx)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.syncOnce(ctx)
		}
	}
}

// syncOnce одна итерация: получаем состояние всех путей MediaMTX,
// проставляем статусы камерам в БД и убираем висячие пути.
func (m *CameraStatusMonitor) syncOnce(ctx context.Context) {
	allPaths, err := m.fetchPaths(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("camera status monitor: failed to fetch MediaMTX paths")
		return
	}

	// ready-состояние по имени пути
	ready := make(map[string]bool, len(allPaths))
	for name, r := range allPaths {
		if r {
			ready[name] = true
		}
	}

	cameras, err := m.repo.ListForStatusCheck(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("camera status monitor: failed to list cameras")
		return
	}

	// Ожидаемые имена путей: <uuid> и <uuid>_sub для каждой камеры.
	// Путь звука (<uuid>_audio) сюда НЕ входит: его создаёт сервис аудио,
	// и удалять его как «висячий» нельзя.
	expected := make(map[string]bool, len(cameras)*2)
	for _, cam := range cameras {
		expected[cam.ID.String()] = true
		expected[cam.ID.String()+"_sub"] = true
	}

	// Восстанавливаем пропавшие пути: MediaMTX хранит их в памяти и теряет
	// при своём перезапуске. Без восстановления камеры остаются без потока
	// до ручного вмешательства.
	m.restoreMissingPaths(ctx, cameras, expected, allPaths)

	m.pruneOrphanPaths(ctx, allPaths, expected)

	for _, cam := range cameras {
		// Для основной камеры путь в MediaMTX назван её UUID.
		online := ready[cam.ID.String()]

		// Не трогаем статус, выставленный вручную (например "recording").
		if cam.Status == "recording" {
			continue
		}
		newStatus := "offline"
		if online {
			newStatus = "online"
		}
		if cam.Status == newStatus {
			continue
		}

		if err := m.repo.UpdateStatus(ctx, cam.ID, newStatus); err != nil {
			log.Warn().Err(err).Str("camera", cam.Name).Msg("camera status monitor: update failed")
			continue
		}

		// Логируем только смену состояния, чтобы не засорять логи.
		prev, seen := m.wasOnline[cam.ID.String()]
		if !seen || prev != online {
			log.Info().
				Str("camera", cam.Name).
				Str("ip", cam.IP).
				Str("status", newStatus).
				Msg("camera status changed")
		}
		m.wasOnline[cam.ID.String()] = online
	}
}

// restoreMissingPaths перерегистрирует в MediaMTX пути, которых там нет.
//
// MediaMTX хранит конфигурацию путей в памяти и теряет её при перезапуске.
// Раньше пути восстанавливались только один раз — при старте backend,
// поэтому рестарт MediaMTX оставлял камеры без потока до перезапуска backend.
func (m *CameraStatusMonitor) restoreMissingPaths(ctx context.Context, cameras []domain.Camera, expected map[string]bool, paths map[string]bool) {
	restored := 0
	for _, cam := range cameras {
		for _, name := range []string{cam.ID.String(), cam.ID.String() + "_sub"} {
			if !expected[name] || paths[name] {
				continue
			}
			// Путь пропал — просим сервис камер зарегистрировать его заново.
			if m.onRestore == nil {
				continue
			}
			if err := m.onRestore(cam); err != nil {
				log.Warn().Err(err).Str("camera", cam.Name).Str("path", name).
					Msg("camera status monitor: не удалось восстановить путь")
				continue
			}
			restored++
		}
	}
	if restored > 0 {
		log.Info().Int("paths", restored).Msg("пути камер восстановлены в MediaMTX")
	}
}

// pruneOrphanPaths удаляет пути MediaMTX, для которых нет камеры в БД.
// Пути со статусом starting (запрос на удаление уже отправлен) повторно не трогаем —
// так мы избегаем бесконечных повторов, если MediaMTX не может удалить путь
// (например, он ещё активен как publisher).
func (m *CameraStatusMonitor) pruneOrphanPaths(ctx context.Context, paths map[string]bool, expected map[string]bool) {
	for name := range paths {
		if expected[name] || m.pruning[name] {
			continue
		}
		// Пути звука (<uuid>_audio) создаёт сервис аудио, и они не относятся
		// к камерам напрямую. Служебные пути не удаляем, иначе звук будет
		// постоянно пропадать и создаваться заново.
		if strings.HasSuffix(name, "_audio") || strings.HasSuffix(name, "_talk") {
			continue
		}
		// Внешние адреса (cameras/{N}/streaming/{main|sub}) публикует сервис
		// внешнего доступа, и в списке ожидаемых их нет — там только UUID
		// камер. Без этой проверки монитор считал бы их сиротами и удалял
		// каждые 15 секунд, поэтому внешние системы не могли подключиться.
		if IsExternalRTSVPath(name) {
			continue
		}

		log.Info().Str("path", name).Msg("removing orphan MediaMTX path")
		if err := m.deletePath(ctx, name); err != nil {
			// Помечаем, чтобы не спамить попытками каждые 15 секунд.
			m.pruning[name] = true
			log.Warn().Err(err).Str("path", name).Msg("failed to remove orphan path, will retry later")
			continue
		}
		delete(m.pruning, name)
	}
}

// deletePath удаляет путь через MediaMTX API.
func (m *CameraStatusMonitor) deletePath(ctx context.Context, name string) error {
	url := fmt.Sprintf("%s/v3/config/paths/delete/%s", m.mediamtxAPI, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 404 — пути уже нет, это успех.
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mediamtx returned status %d", resp.StatusCode)
	}
	return nil
}

// fetchPaths возвращает карту "имя пути -> ready" для всех путей MediaMTX.
func (m *CameraStatusMonitor) fetchPaths(ctx context.Context) (map[string]bool, error) {
	url := fmt.Sprintf("%s/v3/paths/list", m.mediamtxAPI)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mediamtx returned status %d", resp.StatusCode)
	}

	var payload struct {
		Items []struct {
			Name  string `json:"name"`
			Ready bool   `json:"ready"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	paths := make(map[string]bool, len(payload.Items))
	for _, item := range payload.Items {
		paths[item.Name] = item.Ready
	}
	return paths, nil
}
