package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/nvr/backend/internal/service"
)

// CameraHealthHandler отдаёт показатели здоровья камер OpenIPC.
type CameraHealthHandler struct {
	svc *service.CameraHealthService
}

func NewCameraHealthHandler(svc *service.CameraHealthService) *CameraHealthHandler {
	return &CameraHealthHandler{svc: svc}
}

// List возвращает здоровье всех камер, самые проблемные — сверху.
func (h *CameraHealthHandler) List(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.List())
}

// Get возвращает здоровье одной камеры.
func (h *CameraHealthHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := cameraIDFromRequest(w, r)
	if !ok {
		return
	}

	health, ok := h.svc.Get(id)
	if !ok {
		// Данных ещё нет — первый сбор идёт через полминуты после старта.
		writeJSON(w, http.StatusOK, map[string]any{
			"camera_id": id,
			"level":     "unknown",
			"message":   "данные ещё не собраны",
		})
		return
	}
	writeJSON(w, http.StatusOK, health)
}

// Collect запускает сбор показателей камеры немедленно.
func (h *CameraHealthHandler) Collect(w http.ResponseWriter, r *http.Request) {
	id, ok := cameraIDFromRequest(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	health, err := h.svc.CollectNow(ctx, id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, health)
}
