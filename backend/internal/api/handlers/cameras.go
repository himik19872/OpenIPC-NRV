package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/nvr/backend/internal/domain"
	"github.com/nvr/backend/internal/service"
)

var validate = validator.New()

type CameraHandler struct {
	svc *service.CameraService
}

func NewCameraHandler(svc *service.CameraService) *CameraHandler {
	return &CameraHandler{svc: svc}
}

func (h *CameraHandler) List(w http.ResponseWriter, r *http.Request) {
	cameras, err := h.svc.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if cameras == nil {
		cameras = []domain.Camera{}
	}
	writeJSON(w, http.StatusOK, cameras)
}

func (h *CameraHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	cam, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "camera not found"})
		return
	}
	writeJSON(w, http.StatusOK, cam)
}

func (h *CameraHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateCameraRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := validate.Struct(req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	cam, err := h.svc.Create(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, cam)
}

func (h *CameraHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	var req domain.UpdateCameraRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	cam, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cam)
}

func (h *CameraHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// RestartStreamer перезапускает стример камеры (Majestic на OpenIPC).
// POST /api/v1/cameras/{id}/restart-streamer
func (h *CameraHandler) RestartStreamer(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	res, err := h.svc.RestartStreamer(r.Context(), id)
	if err != nil {
		// Ошибка выполнения на камере — не ошибка нашего сервера,
		// поэтому 502 и понятное сообщение.
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":   "failed to restart streamer",
			"details": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Reboot перезагружает камеру.
// POST /api/v1/cameras/{id}/reboot
func (h *CameraHandler) Reboot(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	res, err := h.svc.RebootCamera(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":   "failed to reboot camera",
			"details": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
