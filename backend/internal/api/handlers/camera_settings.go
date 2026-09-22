package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nvr/backend/internal/service"
)

// CameraSettingsHandler управляет настройками камеры через API Majestic.
//
// Это замена SSH-управлению: те же действия (правка параметров видео,
// изображения, ночного режима, OSD и перезапуск) выполняются штатными
// HTTP-эндпоинтами прошивки.
type CameraSettingsHandler struct {
	svc *service.CameraSettingsService
}

func NewCameraSettingsHandler(svc *service.CameraSettingsService) *CameraSettingsHandler {
	return &CameraSettingsHandler{svc: svc}
}

// Get возвращает текущие настройки камеры и сведения об устройстве.
// GET /api/v1/cameras/{id}/settings
func (h *CameraSettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := cameraIDFromRequest(w, r)
	if !ok {
		return
	}

	view, err := h.svc.GetSettings(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// Update записывает изменённые настройки камеры.
// PATCH /api/v1/cameras/{id}/settings
func (h *CameraSettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := cameraIDFromRequest(w, r)
	if !ok {
		return
	}

	var patch service.CameraSettings
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверное тело запроса"})
		return
	}

	if err := h.svc.UpdateSettings(r.Context(), id, patch); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	// Возвращаем настройки после записи: прошивка может скорректировать
	// значения (например, привести размер кадра к поддерживаемому), и
	// интерфейсу нужно показать именно применённый результат.
	view, err := h.svc.GetSettings(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// Restart перезапускает камеру без SSH.
// POST /api/v1/cameras/{id}/restart
func (h *CameraSettingsHandler) Restart(w http.ResponseWriter, r *http.Request) {
	id, ok := cameraIDFromRequest(w, r)
	if !ok {
		return
	}

	res, err := h.svc.Restart(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// cameraIDFromRequest разбирает id камеры из адреса.
func cameraIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный id камеры"})
		return uuid.Nil, false
	}
	return id, true
}
