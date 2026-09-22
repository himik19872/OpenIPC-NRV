package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nvr/backend/internal/service"
)

// FirmwareHandler обслуживает обновление прошивок контроллеров СКУД.
type FirmwareHandler struct {
	svc *service.FirmwareService
}

func NewFirmwareHandler(svc *service.FirmwareService) *FirmwareHandler {
	return &FirmwareHandler{svc: svc}
}

// ListFirmwares возвращает образы прошивок, доступные для установки.
func (h *FirmwareHandler) ListFirmwares(w http.ResponseWriter, r *http.Request) {
	images, err := h.svc.ListFirmwares()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, images)
}

// UploadFirmware принимает образ прошивки от оператора.
//
// Файл передаётся телом запроса, а имя — заголовком X-Firmware-Name:
// так образ можно отдать потоком, не собирая multipart-форму.
func (h *FirmwareHandler) UploadFirmware(w http.ResponseWriter, r *http.Request) {
	name := r.Header.Get("X-Firmware-Name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "не указано имя файла прошивки",
		})
		return
	}

	// Ограничение на чтение с запасом: прошивка около мегабайта, а
	// неограниченное чтение тела запроса — риск исчерпать память.
	const maxUpload = 8 << 20
	data, err := io.ReadAll(io.LimitReader(r.Body, maxUpload+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "не удалось прочитать файл"})
		return
	}
	if len(data) > maxUpload {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
			"error": "файл слишком большой для прошивки",
		})
		return
	}

	img, err := h.svc.SaveFirmware(name, data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, img)
}

// DeleteFirmware удаляет образ прошивки с сервера.
func (h *FirmwareHandler) DeleteFirmware(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := h.svc.DeleteFirmware(name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetVersion возвращает версию прошивки, установленной на контроллере.
func (h *FirmwareHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный идентификатор контроллера"})
		return
	}

	info, err := h.svc.GetFirmwareVersion(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// StartUpdate запускает обновление прошивки контроллера.
func (h *FirmwareHandler) StartUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный идентификатор контроллера"})
		return
	}

	var req struct {
		Firmware string `json:"firmware"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверное тело запроса"})
		return
	}
	if req.Firmware == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "не выбрана прошивка"})
		return
	}

	update, err := h.svc.StartUpdate(r.Context(), id, req.Firmware)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	// Обновление продолжается в фоне — сообщаем об этом кодом 202.
	writeJSON(w, http.StatusAccepted, update)
}

// GetUpdate возвращает состояние обновления контроллера.
func (h *FirmwareHandler) GetUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный идентификатор контроллера"})
		return
	}

	update := h.svc.GetUpdate(id)
	if update == nil {
		// Отсутствие записи означает, что обновление не запускалось, —
		// это не ошибка.
		writeJSON(w, http.StatusOK, map[string]any{"state": "idle"})
		return
	}
	writeJSON(w, http.StatusOK, update)
}
