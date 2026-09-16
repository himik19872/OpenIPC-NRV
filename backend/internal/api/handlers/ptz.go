package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nvr/backend/internal/service"
)

// PTZHandler — управление поворотными камерами (ONVIF PTZ).
type PTZHandler struct {
	svc *service.CameraService
}

func NewPTZHandler(svc *service.CameraService) *PTZHandler {
	return &PTZHandler{svc: svc}
}

// Status возвращает текущее положение камеры.
// GET /api/v1/cameras/{id}/ptz/status
func (h *PTZHandler) Status(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	st, err := h.svc.PTZStatus(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

type ptzMoveRequest struct {
	// Скорость по осям в диапазоне -1..1.
	Pan  float64 `json:"pan"`
	Tilt float64 `json:"tilt"`
	Zoom float64 `json:"zoom"`
	// Сколько миллисекунд двигаться, после чего камера остановится.
	DurationMs int `json:"duration_ms"`
}

// Move запускает движение камеры.
// POST /api/v1/cameras/{id}/ptz/move
func (h *PTZHandler) Move(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	var req ptzMoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if req.DurationMs == 0 {
		req.DurationMs = 500 // короткий шаг по умолчанию
	}

	if err := h.svc.PTZMove(r.Context(), id, req.Pan, req.Tilt, req.Zoom, req.DurationMs); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":   "ptz move failed",
			"details": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// Stop останавливает движение камеры.
// POST /api/v1/cameras/{id}/ptz/stop
func (h *PTZHandler) Stop(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	if err := h.svc.PTZStop(r.Context(), id); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":   "ptz stop failed",
			"details": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// Presets возвращает список сохранённых позиций.
// GET /api/v1/cameras/{id}/ptz/presets
func (h *PTZHandler) Presets(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	presets, err := h.svc.PTZPresets(r.Context(), id)
	if err != nil {
		// Нет поддержки пресетов — это не ошибка, просто пустой список.
		writeJSON(w, http.StatusOK, []service.PTZPreset{})
		return
	}
	if presets == nil {
		presets = []service.PTZPreset{}
	}
	writeJSON(w, http.StatusOK, presets)
}

type ptzPresetRequest struct {
	Token string `json:"token"`
}

// GotoPreset переходит к сохранённой позиции.
// POST /api/v1/cameras/{id}/ptz/presets/goto
func (h *PTZHandler) GotoPreset(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	var req ptzPresetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}

	if err := h.svc.PTZGotoPreset(r.Context(), id, req.Token); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":   "goto preset failed",
			"details": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
