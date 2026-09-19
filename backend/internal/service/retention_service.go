package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nvr/backend/internal/domain"
	"github.com/rs/zerolog/log"
)

// RetentionService удаляет устаревшие снимки и записи по глубине хранения
// из настроек сервера. Работает и с MinIO, и с локальным диском.
type RetentionService struct {
	storage *StorageService
	db      RetentionDB
}

// RetentionDB — доступ к метаданным архива (реализуется репозиторием postgres).
type RetentionDB interface {
	// ExpiredRecordings возвращает записи старше cutoff: id и путь к файлу.
	ExpiredRecordings(ctx context.Context, cutoff time.Time) ([]domain.ExpiredItem, error)
	// ExpiredEvents возвращает события со снимками старше cutoff.
	ExpiredEvents(ctx context.Context, cutoff time.Time) ([]domain.ExpiredItem, error)
	// DeleteRecordings удаляет записи по id.
	DeleteRecordings(ctx context.Context, ids []string) error
	// ClearEventSnapshots обнуляет ссылки на снимки у событий.
	ClearEventSnapshots(ctx context.Context, ids []string) error
}

func NewRetentionService(storage *StorageService, db RetentionDB) *RetentionService {
	return &RetentionService{storage: storage, db: db}
}

// RunOnce выполняет один проход очистки по текущим настройкам.
// Возвращает число удалённых файлов и освобождённый объём.
func (r *RetentionService) RunOnce(ctx context.Context, settings *domain.ServerSettings) (int, int64) {
	if settings == nil || r.db == nil {
		return 0, 0
	}

	deleted, freed := 0, int64(0)

	// Записи видео
	if settings.Storage.RetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -settings.Storage.RetentionDays)
		items, err := r.db.ExpiredRecordings(ctx, cutoff)
		if err != nil {
			log.Warn().Err(err).Msg("очистка: не удалось получить устаревшие записи")
		} else if len(items) > 0 {
			ids := make([]string, 0, len(items))
			for _, it := range items {
				if sz := r.removeFile(ctx, it.Path); sz > 0 {
					freed += sz
				}
				ids = append(ids, it.ID)
			}
			if err := r.db.DeleteRecordings(ctx, ids); err != nil {
				log.Warn().Err(err).Msg("очистка: не удалось удалить записи из БД")
			}
			deleted += len(ids)
			log.Info().Int("count", len(ids)).Int("days", settings.Storage.RetentionDays).
				Msg("очистка: удалены устаревшие записи")
		}
	}

	// Снимки событий
	if settings.Snapshots.RetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -settings.Snapshots.RetentionDays)
		items, err := r.db.ExpiredEvents(ctx, cutoff)
		if err != nil {
			log.Warn().Err(err).Msg("очистка: не удалось получить устаревшие снимки")
		} else if len(items) > 0 {
			ids := make([]string, 0, len(items))
			for _, it := range items {
				if sz := r.removeFile(ctx, it.Path); sz > 0 {
					freed += sz
				}
				ids = append(ids, it.ID)
			}
			// Сами события детекции нужны для статистики — обнуляем только снимок
			if err := r.db.ClearEventSnapshots(ctx, ids); err != nil {
				log.Warn().Err(err).Msg("очистка: не удалось очистить ссылки на снимки")
			}
			deleted += len(ids)
			log.Info().Int("count", len(ids)).Int("days", settings.Snapshots.RetentionDays).
				Msg("очистка: удалены устаревшие снимки")
		}
	}

	return deleted, freed
}

// removeFile удаляет файл и возвращает его размер.
// Для MinIO удаляет объект из бакета, для локального пути — файл с диска.
func (r *RetentionService) removeFile(ctx context.Context, storedPath string) int64 {
	if storedPath == "" {
		return 0
	}

	switch {
	case strings.HasPrefix(storedPath, "minio:"):
		if r.storage == nil || r.storage.videoRepo == nil {
			return 0
		}
		key := strings.TrimPrefix(storedPath, "minio:")
		// Размер узнаём до удаления
		size := int64(0)
		if info, err := r.storage.videoRepo.Stat(ctx, key); err == nil {
			size = info
		}
		if err := r.storage.videoRepo.Delete(ctx, key); err != nil {
			log.Debug().Err(err).Str("key", key).Msg("очистка: не удалось удалить объект")
			return 0
		}
		return size

	case strings.HasPrefix(storedPath, "local:"):
		p := strings.TrimPrefix(storedPath, "local:")
		fi, err := os.Stat(p)
		if err != nil {
			return 0
		}
		size := fi.Size()
		if err := os.Remove(p); err != nil {
			log.Debug().Err(err).Str("path", p).Msg("очистка: не удалось удалить файл")
			return 0
		}
		// Убираем пустые каталоги камеры/даты, чтобы не копить пустые папки
		pruneEmptyDirs(filepath.Dir(p))
		return size
	}

	return 0
}

// pruneEmptyDirs удаляет пустые каталоги вверх по дереву (не глубже корня хранилища).
func pruneEmptyDirs(dir string) {
	for i := 0; i < 3; i++ {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// sizeOf возвращает размер объекта без удаления: из MinIO или с диска.
func (r *RetentionService) sizeOf(ctx context.Context, storedPath string) int64 {
	if storedPath == "" {
		return 0
	}
	switch {
	case strings.HasPrefix(storedPath, "minio:"):
		if r.storage == nil || r.storage.videoRepo == nil {
			return 0
		}
		key := strings.TrimPrefix(storedPath, "minio:")
		if size, err := r.storage.videoRepo.Stat(ctx, key); err == nil {
			return size
		}
	case strings.HasPrefix(storedPath, "local:"):
		p := strings.TrimPrefix(storedPath, "local:")
		if fi, err := os.Stat(p); err == nil {
			return fi.Size()
		}
	}
	return 0
}

// ExpiredReport описывает, что будет удалено при следующем проходе.
// Используется в интерфейсе, чтобы показать последствия настроек.
type ExpiredReport struct {
	Recordings int   `json:"recordings"`
	Snapshots  int   `json:"snapshots"`
	TotalSize  int64 `json:"total_size"`
}

// Preview считает, сколько объектов попадёт под очистку при текущих настройках,
// ничего не удаляя.
func (r *RetentionService) Preview(ctx context.Context, settings *domain.ServerSettings) (*ExpiredReport, error) {
	if settings == nil || r.db == nil {
		return &ExpiredReport{}, nil
	}
	rep := &ExpiredReport{}

	if settings.Storage.RetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -settings.Storage.RetentionDays)
		items, err := r.db.ExpiredRecordings(ctx, cutoff)
		if err != nil {
			return nil, fmt.Errorf("recordings: %w", err)
		}
		rep.Recordings = len(items)
		for _, it := range items {
			rep.TotalSize += r.sizeOf(ctx, it.Path)
		}
	}

	if settings.Snapshots.RetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -settings.Snapshots.RetentionDays)
		items, err := r.db.ExpiredEvents(ctx, cutoff)
		if err != nil {
			return nil, fmt.Errorf("snapshots: %w", err)
		}
		rep.Snapshots = len(items)
		for _, it := range items {
			rep.TotalSize += r.sizeOf(ctx, it.Path)
		}
	}

	return rep, nil
}
