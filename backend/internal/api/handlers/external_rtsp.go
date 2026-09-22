package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nvr/backend/internal/service"
)

// ExternalRTSPSettings — настройки внешнего доступа для интерфейса.
//
// Страница показывает оператору всё, что нужно для настройки сторонней
// системы: адрес сервера, порт, учётные данные и список каналов.
type ExternalRTSPSettings struct {
	// Port — порт для внешних систем. Показывается в адресах потоков.
	Port int `json:"port"`
	// Username и Password — учётные данные, которые внешняя система
	// указывает при подключении.
	Username string `json:"username"`
	Password string `json:"password"`
	// ServerIP — адрес, по которому сервер виден в сети. Определяется
	// по запросу браузера: у сервера может быть несколько адресов,
	// и угадать нужный нельзя.
	ServerIP string `json:"server_ip"`
	// Channels — каналы с готовыми адресами потоков.
	Channels []service.ExternalChannel `json:"channels"`
	// Unassigned — камеры без номера канала: наружу они не публикуются,
	// но оператор должен их видеть, чтобы назначить номер.
	Unassigned []service.ExternalChannel `json:"unassigned"`
	// NextChannel — свободный номер, который можно назначить следующей
	// камере. Подсказка для формы быстрого назначения.
	NextChannel int `json:"next_channel"`
}

// ExternalRTSPHandler отдаёт сведения для настройки внешнего доступа.
type ExternalRTSPHandler struct {
	svc       *service.ExternalRTSPService
	cameraSvc *service.CameraService
}

func NewExternalRTSPHandler(svc *service.ExternalRTSPService, cameraSvc *service.CameraService) *ExternalRTSPHandler {
	return &ExternalRTSPHandler{svc: svc, cameraSvc: cameraSvc}
}

// Settings возвращает настройки внешнего доступа и список каналов.
// GET /api/v1/rtsp/settings
func (h *ExternalRTSPHandler) Settings(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	channels, unassigned, err := h.cameraSvc.ExternalChannels(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Камеру определяем по адресу запроса: у сервера может быть несколько
	// сетевых интерфейсов, и только оператор знает, какой из них виден
	// внешней системе. Значение из Host — наиболее вероятный вариант.
	serverIP := r.Host
	if idx := indexByte(serverIP, ':'); idx >= 0 {
		serverIP = serverIP[:idx]
	}

	// Свободный номер показываем в форме назначения: чаще всего оператор
	// как раз хочет добавить канал в конец списка.
	nextChannel := 1
	if len(channels) > 0 {
		nextChannel = channels[len(channels)-1].Number + 1
	}

	writeJSON(w, http.StatusOK, ExternalRTSPSettings{
		Port:        service.ExternalRTSPPort,
		Username:    h.svc.PublicUsername(),
		Password:    h.svc.PublicPassword(),
		ServerIP:    serverIP,
		Channels:    channels,
		Unassigned:  unassigned,
		NextChannel: nextChannel,
	})
}

// indexByte ищет байт в строке. Вынесено отдельно, чтобы не тянуть
// в файл лишний импорт strings ради одной операции.
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// AssignChannel задаёт номер канала камере прямо со страницы внешнего доступа.
// POST /api/v1/rtsp/channels/{cameraId}
//
// Нужен, чтобы оператор не открывал карточку камеры ради одного поля:
// назначение номера — обычное действие при добавлении новой камеры.
func (h *ExternalRTSPHandler) AssignChannel(w http.ResponseWriter, r *http.Request) {
	cameraID, err := uuid.Parse(chi.URLParam(r, "cameraId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный id камеры"})
		return
	}

	var req struct {
		Channel int `json:"channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверное тело запроса"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := h.cameraSvc.AssignChannel(ctx, cameraID, req.Channel); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Возвращаем обновлённый список: интерфейсу нужно сразу показать,
	// что канал появился, а камера ушла из списка неназначенных.
	h.Settings(w, r)
}
