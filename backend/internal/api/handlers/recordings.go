package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nvr/backend/internal/domain"
	miniorepo "github.com/nvr/backend/internal/repository/minio"
	"github.com/rs/zerolog/log"
)

type RecordingHandler struct {
	db *pgxpool.Pool
	// videoRepo может быть nil, если MinIO недоступен: тогда архив
	// остаётся доступен для чтения метаданных, но без ссылок на файлы.
	videoRepo *miniorepo.VideoRepo
}

func NewRecordingHandler(db *pgxpool.Pool, videoRepo *miniorepo.VideoRepo) *RecordingHandler {
	return &RecordingHandler{db: db, videoRepo: videoRepo}
}

// signedURL возвращает временную ссылку на файл записи в MinIO.
// Пустая строка означает, что ссылку получить не удалось.
func (h *RecordingHandler) signedURL(filePath string) string {
	if h.videoRepo == nil || filePath == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url, err := h.videoRepo.PresignedURL(ctx, filePath, time.Hour)
	if err != nil {
		log.Warn().Err(err).Str("key", filePath).Msg("failed to presign recording url")
		return ""
	}
	return url
}

func (h *RecordingHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	cameraIDStr := r.URL.Query().Get("camera_id")
	var cameraID *uuid.UUID
	if cameraIDStr != "" {
		id, err := uuid.Parse(cameraIDStr)
		if err == nil {
			cameraID = &id
		}
	}

	query := `SELECT id, camera_id, start_time, end_time,
		EXTRACT(EPOCH FROM (end_time - start_time))::float as duration,
		file_path, file_size, resolution, codec, event_triggered, metadata
		FROM recordings WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM recordings WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if cameraID != nil {
		query += ` AND camera_id = $` + strconv.Itoa(argIdx)
		countQuery += ` AND camera_id = $` + strconv.Itoa(argIdx)
		args = append(args, cameraID.String())
		argIdx++
	}

	// Total count
	var total int64
	if err := h.db.QueryRow(r.Context(), countQuery, args...).Scan(&total); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	offset := (page - 1) * pageSize
	query += ` ORDER BY start_time DESC LIMIT $` + strconv.Itoa(argIdx) + ` OFFSET $` + strconv.Itoa(argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	recordings := make([]domain.Recording, 0)
	// Ссылки на файлы формируем отдельно: они не входят в domain.Recording,
	// но нужны фронтенду для воспроизведения.
	urls := make([]string, 0, 20)
	for rows.Next() {
		var rec domain.Recording
		if err := rows.Scan(
			&rec.ID, &rec.CameraID, &rec.StartTime, &rec.EndTime,
			&rec.Duration, &rec.FilePath, &rec.FileSize,
			&rec.Resolution, &rec.Codec, &rec.EventTriggered, &rec.Metadata,
		); err != nil {
			continue
		}
		recordings = append(recordings, rec)
		urls = append(urls, h.signedURL(rec.FilePath))
	}

	// Подписи ссылок генерируются параллельно, поэтому собираем ответ вручную.
	items := make([]map[string]any, 0, len(recordings))
	for i, rec := range recordings {
		items = append(items, recordingResponse(rec, urls[i]))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"recordings": items,
		"total":      total,
		"page":       page,
		"page_size":  pageSize,
	})
}

func (h *RecordingHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	var rec domain.Recording
	err = h.db.QueryRow(r.Context(),
		`SELECT id, camera_id, start_time, end_time,
		EXTRACT(EPOCH FROM (end_time - start_time))::float as duration,
		file_path, file_size, resolution, codec, event_triggered, metadata
		FROM recordings WHERE id = $1`, id.String(),
	).Scan(
		&rec.ID, &rec.CameraID, &rec.StartTime, &rec.EndTime,
		&rec.Duration, &rec.FilePath, &rec.FileSize,
		&rec.Resolution, &rec.Codec, &rec.EventTriggered, &rec.Metadata,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "recording not found"})
		return
	}

	// Ссылка на файл в хранилище (действует час).
	writeJSON(w, http.StatusOK, recordingResponse(rec, h.signedURL(rec.FilePath)))
}

// recordingResponse добавляет к записи временную ссылку на файл.
func recordingResponse(rec domain.Recording, url string) map[string]any {
	return map[string]any{
		"id":              rec.ID,
		"camera_id":       rec.CameraID,
		"start_time":      rec.StartTime,
		"end_time":        rec.EndTime,
		"duration":        rec.Duration,
		"file_path":       rec.FilePath,
		"file_size":       rec.FileSize,
		"resolution":      rec.Resolution,
		"codec":           rec.Codec,
		"event_triggered": rec.EventTriggered,
		"metadata":        rec.Metadata,
		"url":             url,
	}
}

func (h *RecordingHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	// Сначала забираем ключ объекта, чтобы удалить и файл, и запись.
	var filePath string
	err = h.db.QueryRow(r.Context(),
		`SELECT COALESCE(file_path, '') FROM recordings WHERE id = $1`, id.String(),
	).Scan(&filePath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "recording not found"})
		return
	}

	if _, err := h.db.Exec(r.Context(), `DELETE FROM recordings WHERE id = $1`, id.String()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Файл удаляем после записи: если хранилище недоступно, метаданные
	// уже удалены и «осиротевший» объект можно убрать отдельной задачей.
	if h.videoRepo != nil && filePath != "" {
		if err := h.videoRepo.Delete(r.Context(), filePath); err != nil {
			log.Warn().Err(err).Str("key", filePath).Msg("failed to delete recording file from storage")
		}
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// Используемый time для ответа stats
var _ = time.Now
