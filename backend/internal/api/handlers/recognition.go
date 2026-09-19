package handlers

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nvr/backend/internal/domain"
	"github.com/nvr/backend/internal/repository/postgres"
	"github.com/nvr/backend/internal/service"
)

// RecognitionHandler — справочники известных лиц и автомобильных номеров,
// а также настройки распознавания.
//
// Списки нужны, чтобы отличать «своих» от посторонних: обнаруженное лицо или
// номер сопоставляется со справочником, и результат сохраняется в событии.
type RecognitionHandler struct {
	repo    *postgres.RecognitionRepo
	storage *service.StorageService
}

func NewRecognitionHandler(repo *postgres.RecognitionRepo) *RecognitionHandler {
	return &RecognitionHandler{repo: repo}
}

// WithStorage подключает хранилище для эталонных снимков.
func (h *RecognitionHandler) WithStorage(s *service.StorageService) *RecognitionHandler {
	h.storage = s
	return h
}

// --- Лица ---

// ListFaces возвращает справочник известных лиц.
// GET /api/v1/faces?all=true
func (h *RecognitionHandler) ListFaces(w http.ResponseWriter, r *http.Request) {
	withDisabled := r.URL.Query().Get("all") == "true"

	faces, err := h.repo.ListFaces(r.Context(), withDisabled)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"faces": faces, "total": len(faces)})
}

// CreateFace регистрирует новое лицо в справочнике.
// POST /api/v1/faces
func (h *RecognitionHandler) CreateFace(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateKnownFaceRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "имя обязательно"})
		return
	}

	face := &domain.KnownFace{
		Name:      req.Name,
		Note:      req.Note,
		IsBlocked: req.IsBlocked,
		Enabled:   true,
	}

	if err := h.repo.CreateFace(r.Context(), face, req.Embedding); err != nil {
		// Нарушение уникальности или иная ошибка БД — отдаём понятный текст.
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Снимок сохраняем после создания записи: его путь привязан к ID лица.
	if err := h.saveReferencePhoto(r, "faces", face.ID, req.PhotoBase64); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, face)
}

// UpdateFace изменяет запись справочника лиц.
// PATCH /api/v1/faces/{id}
func (h *RecognitionHandler) UpdateFace(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	var req domain.UpdateKnownFaceRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "имя не может быть пустым"})
		return
	}

	if err := h.repo.UpdateFace(r.Context(), id, req); err != nil {
		writeRecognitionError(w, err)
		return
	}

	if req.PhotoBase64 != nil {
		if err := h.saveReferencePhoto(r, "faces", id, *req.PhotoBase64); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	face, err := h.repo.GetFace(r.Context(), id)
	if err != nil {
		writeRecognitionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, face)
}

// DeleteFace удаляет лицо из справочника.
// DELETE /api/v1/faces/{id}
func (h *RecognitionHandler) DeleteFace(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.repo.DeleteFace(r.Context(), id); err != nil {
		writeRecognitionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// FacePhoto отдаёт эталонный снимок лица.
// GET /api/v1/faces/{id}/photo
//
// Вне JWT-группы: картинку запрашивает тег <img>, который не передаёт заголовок.
func (h *RecognitionHandler) FacePhoto(w http.ResponseWriter, r *http.Request) {
	h.servePhoto(w, r, "faces")
}

// --- Номера ---

// ListPlates возвращает справочник известных номеров.
// GET /api/v1/plates?all=true
func (h *RecognitionHandler) ListPlates(w http.ResponseWriter, r *http.Request) {
	withDisabled := r.URL.Query().Get("all") == "true"

	plates, err := h.repo.ListPlates(r.Context(), withDisabled)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plates": plates, "total": len(plates)})
}

// CreatePlate добавляет номер в справочник.
// POST /api/v1/plates
func (h *RecognitionHandler) CreatePlate(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateKnownPlateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.Plate = strings.TrimSpace(req.Plate)
	if req.Plate == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "номер обязателен"})
		return
	}
	// Нормализуем и проверяем, что после очистки остались символы.
	if postgres.NormalizePlate(req.Plate) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "номер не содержит букв или цифр"})
		return
	}

	plate := &domain.KnownPlate{
		Plate:     req.Plate,
		Owner:     req.Owner,
		Note:      req.Note,
		IsBlocked: req.IsBlocked,
		Enabled:   true,
	}
	if err := h.repo.CreatePlate(r.Context(), plate); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := h.saveReferencePhoto(r, "plates", plate.ID, req.PhotoBase64); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, plate)
}

// UpdatePlate изменяет запись справочника номеров.
// PATCH /api/v1/plates/{id}
func (h *RecognitionHandler) UpdatePlate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	var req domain.UpdateKnownPlateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Plate != nil && postgres.NormalizePlate(*req.Plate) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "номер не содержит букв или цифр"})
		return
	}

	if err := h.repo.UpdatePlate(r.Context(), id, req); err != nil {
		writeRecognitionError(w, err)
		return
	}

	plate, err := h.repo.GetPlate(r.Context(), id)
	if err != nil {
		writeRecognitionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plate)
}

// DeletePlate удаляет номер из справочника.
// DELETE /api/v1/plates/{id}
func (h *RecognitionHandler) DeletePlate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.repo.DeletePlate(r.Context(), id); err != nil {
		writeRecognitionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PlatePhoto отдаёт снимок автомобиля из справочника.
// GET /api/v1/plates/{id}/photo
func (h *RecognitionHandler) PlatePhoto(w http.ResponseWriter, r *http.Request) {
	h.servePhoto(w, r, "plates")
}

// --- Настройки распознавания ---

// GetSettings возвращает настройки распознавания лиц и номеров.
// GET /api/v1/settings/recognition
func (h *RecognitionHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.repo.GetRecognitionSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// UpdateSettings изменяет настройки распознавания.
// PATCH /api/v1/settings/recognition
func (h *RecognitionHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req domain.UpdateRecognitionSettingsRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if req.Faces != nil {
		if req.Faces.Threshold < 0 || req.Faces.Threshold > 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "порог лиц должен быть в диапазоне 0..1"})
			return
		}
	}
	if req.Plates != nil {
		if req.Plates.Threshold < 0 || req.Plates.Threshold > 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "порог номеров должен быть в диапазоне 0..1"})
			return
		}
	}

	s, err := h.repo.UpdateRecognitionSettings(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// Stats возвращает размеры справочников.
// GET /api/v1/recognition/stats
func (h *RecognitionHandler) Stats(w http.ResponseWriter, r *http.Request) {
	faces, plates, err := h.repo.Counts(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"faces": faces, "plates": plates})
}

// --- Вспомогательные ---

// saveReferencePhoto сохраняет эталонный снимок и привязывает его к записи.
func (h *RecognitionHandler) saveReferencePhoto(r *http.Request, kind string, id uuid.UUID, photoBase64 string) error {
	if photoBase64 == "" {
		return nil
	}
	if h.storage == nil {
		return errors.New("хранилище недоступно")
	}

	// Клиент может прислать data-URL целиком — отделяем префикс.
	if idx := strings.Index(photoBase64, ","); idx >= 0 && strings.HasPrefix(photoBase64, "data:") {
		photoBase64 = photoBase64[idx+1:]
	}

	data, err := base64.StdEncoding.DecodeString(photoBase64)
	if err != nil {
		return errors.New("снимок не является корректным base64")
	}
	if len(data) > 8<<20 {
		return errors.New("снимок слишком большой (максимум 8 МБ)")
	}

	path, err := h.storage.SaveReferencePhoto(r.Context(), kind, id, data)
	if err != nil {
		return err
	}

	switch kind {
	case "faces":
		return h.repo.UpdateFacePhoto(r.Context(), id, path)
	case "plates":
		return h.repo.UpdatePlatePhoto(r.Context(), id, path)
	}
	return nil
}

// servePhoto отдаёт эталонный снимок из хранилища.
func (h *RecognitionHandler) servePhoto(w http.ResponseWriter, r *http.Request, kind string) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	var storedPath string
	switch kind {
	case "faces":
		f, err := h.repo.GetFace(r.Context(), id)
		if err != nil {
			writeRecognitionError(w, err)
			return
		}
		storedPath = f.PhotoPath
	case "plates":
		p, err := h.repo.GetPlate(r.Context(), id)
		if err != nil {
			writeRecognitionError(w, err)
			return
		}
		storedPath = p.PhotoPath
	}

	if storedPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "снимок не задан"})
		return
	}
	if h.storage == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "хранилище недоступно"})
		return
	}

	data, _, err := h.storage.ReadStoredFile(r.Context(), storedPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "не удалось прочитать снимок"})
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	// Снимок справочника меняется редко, но по фиксированному URL —
	// запрещаем кеширование, иначе после замены фото виден старый кадр.
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

// writeRecognitionError переводит ошибки репозитория в HTTP-коды.
func writeRecognitionError(w http.ResponseWriter, err error) {
	if errors.Is(err, postgres.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "запись не найдена"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}
