package acs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nvr/backend/internal/domain"
	"github.com/rs/zerolog/log"
)

// SkudAdapter — адаптер контроллера SKUD на ESP32-P4.
//
// Контроллер разрабатывается в этом же проекте (см. firmware/skud-esp32-p4),
// поэтому у него простой REST API с HTTP Basic Auth вместо вендорских
// протоколов вроде ISAPI. Вся работа сводится к обычным JSON-запросам.
//
// Особенности, которые нужно учитывать:
//   - дверь одна: контроллер управляет одним замком (lock_gpio), поэтому
//     список дверей всегда содержит один элемент;
//   - журнал хранит до 1000 записей и отдаёт их пачкой, без курсоров,
//     поэтому синхронизация идёт по времени последнего прочитанного события;
//   - у контроллера нет TLS — он работает по HTTP в локальной сети.
type SkudAdapter struct {
	ctrl     *domain.ACSController
	login    string
	password string
	client   *http.Client
}

// NewSkudAdapter создаёт адаптер для контроллера SKUD.
func NewSkudAdapter(ctrl *domain.ACSController) (Adapter, error) {
	login, password := "", ""
	if ctrl.Credentials != nil {
		if v, ok := ctrl.Credentials["login"].(string); ok {
			login = v
		}
		if v, ok := ctrl.Credentials["password"].(string); ok {
			password = v
		}
	}
	if login == "" {
		login = "admin"
	}

	return &SkudAdapter{
		ctrl:     ctrl,
		login:    login,
		password: password,
		// Таймаут небольшой: контроллер в локальной сети, и если он не
		// отвечает за 5 секунд, значит недоступен — ждать дальше нет смысла.
		client: &http.Client{Timeout: 5 * time.Second},
	}, nil
}

// baseURL собирает адрес контроллера с учётом порта.
func (a *SkudAdapter) baseURL() string {
	port := a.ctrl.Port
	if port == 0 {
		port = 80
	}
	host := a.ctrl.IP
	// Порт 80 подразумевается схемой, лишний суффикс только мешает.
	if port == 80 {
		return fmt.Sprintf("http://%s", host)
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}

// call выполняет запрос к контроллеру с Basic Auth.
func (a *SkudAdapter) call(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("собрать запрос: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, a.baseURL()+path, reader)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(a.login, a.password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("контроллер недоступен: %w", err)
	}
	defer resp.Body.Close()

	// Ограничиваем чтение: журнал может быть большим, но 4 МБ хватает
	// с запасом, а неограниченное чтение — риск исчерпать память.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("прочитать ответ: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, fmt.Errorf("неверный логин или пароль контроллера")
	case resp.StatusCode >= 400:
		// Контроллер кладёт причину в {"error": "..."} — показываем её
		// оператору, иначе сообщение будет бесполезным.
		msg := strings.TrimSpace(string(data))
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("контроллер вернул %d: %s", resp.StatusCode, msg)
	}

	return data, nil
}

// skudStatus — ответ /api/status.
type skudStatus struct {
	UptimeSec  int64  `json:"uptime_sec"`
	FreeHeap   int64  `json:"free_heap"`
	Now        int64  `json:"now"`
	TimeStr    string `json:"time_str"`
	TimeSynced bool   `json:"time_synced"`
	DeviceName string `json:"device_name"`
	Location   string `json:"device_location"`
	CardCount  int    `json:"card_count"`
	EventCount int    `json:"event_count"`
}

// Ping проверяет доступность контроллера и заодно уточняет его название.
//
// Название и расположение задаются в самом контроллере: оператор видит
// в интерфейсе то же имя, что на устройстве, и не путает контроллеры.
func (a *SkudAdapter) Ping(ctx context.Context) error {
	data, err := a.call(ctx, http.MethodGet, "/api/status", nil)
	if err != nil {
		return err
	}

	var st skudStatus
	if err := json.Unmarshal(data, &st); err != nil {
		return fmt.Errorf("разобрать статус: %w", err)
	}

	// Заодно проверяем синхронизацию времени: если часы сбиты, события
	// в журнале будут с неверными отметками, и синхронизация по времени
	// начнёт пропускать или дублировать записи.
	if !st.TimeSynced {
		log.Warn().Str("controller", a.ctrl.Name).
			Msg("у контроллера СКУД не синхронизировано время")
	}

	log.Debug().Str("controller", a.ctrl.Name).
		Str("device", st.DeviceName).
		Int("cards", st.CardCount).
		Int("events", st.EventCount).
		Msg("контроллер СКУД отвечает")

	return nil
}

// ListDoors возвращает дверь контроллера.
//
// Контроллер управляет одним замком, поэтому дверь всегда одна. Её имя
// берём из системной конфигурации: там оператор задаёт расположение
// («Главный вход»), и это понятнее, чем техническое «door-1».
func (a *SkudAdapter) ListDoors(ctx context.Context) ([]Door, error) {
	name := "Дверь"
	if data, err := a.call(ctx, http.MethodGet, "/api/sysconfig", nil); err == nil {
		var cfg struct {
			DeviceName     string `json:"device_name"`
			DeviceLocation string `json:"device_location"`
		}
		if json.Unmarshal(data, &cfg) == nil {
			switch {
			case cfg.DeviceLocation != "" && cfg.DeviceName != "":
				name = cfg.DeviceName + " — " + cfg.DeviceLocation
			case cfg.DeviceLocation != "":
				name = cfg.DeviceLocation
			case cfg.DeviceName != "":
				name = cfg.DeviceName
			}
		}
	}

	status := "locked"
	if st, err := a.GetDoorStatus(ctx, doorID); err == nil && st.Open {
		status = "open"
	}

	return []Door{{ID: doorID, Name: name, Status: status}}, nil
}

// doorID — идентификатор единственной двери контроллера.
// Нужен, потому что интерфейс адресует двери по строке.
const doorID = "door-1"

// OpenDoor открывает дверь: контроллер подаёт импульс на замок.
//
// Длительность импульса задаётся настройкой lock_pulse_ms самого контроллера,
// поэтому здесь её передавать не нужно.
func (a *SkudAdapter) OpenDoor(ctx context.Context, reqDoorID string) error {
	// Проверяем идентификатор: у контроллера одна дверь, и молча открывать
	// замок по любому переданному значению нельзя — иначе опечатка в
	// интерфейсе приведёт к открытию двери.
	if reqDoorID != "" && reqDoorID != doorID {
		return fmt.Errorf("неизвестная дверь: %s", reqDoorID)
	}
	_, err := a.call(ctx, http.MethodPost, "/api/door/open", nil)
	return err
}

// GetDoorStatus определяет состояние двери.
//
// Отдельного эндпоинта состояния нет: контроллер сообщает об открытии и
// закрытии событиями в журнале. Поэтому смотрим последнее такое событие —
// оно и есть текущее состояние.
func (a *SkudAdapter) GetDoorStatus(ctx context.Context, _ string) (DoorStatus, error) {
	data, err := a.call(ctx, http.MethodGet, "/api/log", nil)
	if err != nil {
		return DoorStatus{}, err
	}

	var lg struct {
		Events []skudEvent `json:"events"`
	}
	if err := json.Unmarshal(data, &lg); err != nil {
		return DoorStatus{}, fmt.Errorf("разобрать журнал: %w", err)
	}

	st := DoorStatus{Locked: true}
	// Идём с конца: нужно самое свежее событие двери.
	for i := len(lg.Events) - 1; i >= 0; i-- {
		switch lg.Events[i].Type {
		case evtDoorOpen:
			st.Open = true
			st.Locked = false
			return st, nil
		case evtDoorClosed:
			return st, nil
		case evtDoorForced:
			// Взлом означает, что дверь открыта без разрешения.
			st.Open = true
			st.Locked = false
			st.Alarm = true
			return st, nil
		}
	}
	return st, nil
}

// Типы событий контроллера (event_type_t из прошивки).
const (
	evtCardGranted   = 0 // доступ разрешён
	evtCardDenied    = 1 // отказ
	evtRTEPressed    = 2 // кнопка выхода
	evtDoorOpen      = 3 // дверь открыта (геркон)
	evtDoorClosed    = 4 // дверь закрыта
	evtDoorForced    = 5 // взлом
	evtSystemStart   = 6 // старт контроллера
	evtAuthFail      = 7 // неудачный вход
	evtConfigChanged = 8 // изменена конфигурация
)

// skudEvent — запись журнала контроллера.
type skudEvent struct {
	TS       int64  `json:"ts"`
	Type     int    `json:"type"`
	Facility int    `json:"facility"`
	Card     int64  `json:"card"`
	Note     string `json:"note"`
	Flags    int    `json:"flags"`
}

// eventName переводит тип события в строку для нашей базы.
func eventName(t int) string {
	switch t {
	case evtCardGranted:
		return "access_granted"
	case evtCardDenied:
		return "access_denied"
	case evtRTEPressed:
		return "exit_button"
	case evtDoorOpen:
		return "door_open"
	case evtDoorClosed:
		return "door_closed"
	case evtDoorForced:
		return "door_forced"
	case evtSystemStart:
		return "system_start"
	case evtAuthFail:
		return "auth_failed"
	case evtConfigChanged:
		return "config_changed"
	}
	return "unknown"
}

// SubscribeEvents отдаёт поток событий контроллера.
//
// Webhook-режим (op_mode=1 на контроллере) требует, чтобы контроллер сам
// вызывал наш сервер. Но если сеть между ними недоступна или адрес задан
// неверно, события теряются. Поэтому используем опрос журнала: он работает
// всегда, а контроллер хранит до 1000 записей — этого хватает, чтобы
// пережить временную недоступность сервера.
//
// Повторно отданные события отсекаются по времени последней записи:
// контроллер не поддерживает курсоры, поэтому журнал читается целиком.
func (a *SkudAdapter) SubscribeEvents(ctx context.Context) (<-chan domain.ACSEvent, error) {
	out := make(chan domain.ACSEvent, 128)

	go func() {
		defer close(out)

		// Начинаем с текущего момента: события до подключения уже
		// неактуальны, а весь журнал заливать в базу не нужно.
		lastTS := time.Now().Unix()
		if err := a.Ping(ctx); err == nil {
			// Если контроллер синхронизирован по времени, берём его часы —
			// они точнее локальных при расхождении часовых поясов.
			lastTS = a.controllerNow(ctx, lastTS)
		}

		// Опрос раз в 2 секунды: события двери не критичны к задержке,
		// а редкий опрос не нагружает слабый контроллер.
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		// При сетевом сбое не спамим: увеличиваем паузу до 30 секунд.
		failures := 0

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			events, newest, err := a.fetchEvents(ctx, lastTS)
			if err != nil {
				failures++
				if failures <= 3 || failures%10 == 0 {
					log.Warn().Err(err).Str("controller", a.ctrl.Name).
						Int("failures", failures).
						Msg("не удалось прочитать журнал СКУД")
				}
				// После нескольких сбоев переходим на редкий опрос,
				// чтобы не забивать лог, но не терять контроль совсем.
				if failures > 3 {
					time.Sleep(10 * time.Second)
				}
				continue
			}
			failures = 0

			if newest > lastTS {
				lastTS = newest
			}

			for _, ev := range events {
				select {
				case <-ctx.Done():
					return
				case out <- ev:
				}
			}
		}
	}()

	return out, nil
}

// controllerNow возвращает текущее время контроллера, а при неудаче —
// переданное значение.
func (a *SkudAdapter) controllerNow(ctx context.Context, fallback int64) int64 {
	data, err := a.call(ctx, http.MethodGet, "/api/status", nil)
	if err != nil {
		return fallback
	}
	var st skudStatus
	if json.Unmarshal(data, &st) != nil || st.Now <= 0 {
		return fallback
	}
	return st.Now
}

// fetchEvents читает журнал и возвращает события новее указанного времени.
//
// Второе значение — максимальная метка времени среди прочитанных записей:
// по ней отсекаются уже обработанные события при следующем опросе.
func (a *SkudAdapter) fetchEvents(ctx context.Context, after int64) ([]domain.ACSEvent, int64, error) {
	data, err := a.call(ctx, http.MethodGet, "/api/log", nil)
	if err != nil {
		return nil, 0, err
	}

	var lg struct {
		Events []skudEvent `json:"events"`
		Count  int         `json:"count"`
	}
	if err := json.Unmarshal(data, &lg); err != nil {
		return nil, 0, fmt.Errorf("разобрать журнал: %w", err)
	}

	newest := after
	out := make([]domain.ACSEvent, 0, 8)

	for _, e := range lg.Events {
		if e.TS <= after {
			continue
		}
		if e.TS > newest {
			newest = e.TS
		}

		// Служебные записи (старт, смена конфигурации) в интерфейсе не нужны:
		// они не относятся к проходам и только засоряют журнал.
		if e.Type == evtSystemStart || e.Type == evtConfigChanged {
			continue
		}

		ev := domain.ACSEvent{
			ControllerID: a.ctrl.ID,
			DoorID:       doorID,
			EventType:    eventName(e.Type),
			Timestamp:    time.Unix(e.TS, 0),
			Metadata: map[string]any{
				"card_raw": e.Card,
				"facility": e.Facility,
				"flags":    e.Flags,
				// bit0 означает, что проход завершён: человек не только
				// приложил карту, но и открыл дверь.
				"passage_complete": e.Flags&1 == 1,
			},
		}

		// Номер карты показываем только для событий с картой: у кнопки
		// выхода и событий двери карты нет.
		if e.Type == evtCardGranted || e.Type == evtCardDenied {
			ev.CardNumber = formatCard(e.Facility, e.Card)
		}
		if e.Note != "" {
			ev.Metadata["note"] = e.Note
		}

		out = append(out, ev)
	}

	return out, newest, nil
}

// formatCard приводит номер карты к виду, понятному оператору.
//
// Контроллер хранит карту как число, а на пластике она напечатана в
// десятичном виде — поэтому показываем «facility:card».
func formatCard(facility int, card int64) string {
	return strconv.Itoa(facility) + ":" + strconv.FormatInt(card, 10)
}

// Режим работы контроллера: сам отправляет события на сервер.
const opModeServer = 1

// SyncServerConfig прописывает в контроллере адрес нашего сервера.
//
// Нужен, чтобы контроллер отправлял события сам, без опроса. Если по
// какой-то причине это не сработает, опрос журнала остаётся резервным
// путём и продолжит работать.
func (a *SkudAdapter) SyncServerConfig(ctx context.Context, host string, port int, path string) error {
	if host == "" {
		return fmt.Errorf("не указан адрес сервера")
	}
	body := map[string]any{
		"op_mode":     opModeServer,
		"server_host": host,
		"server_port": port,
		"server_path": path,
	}
	_, err := a.call(ctx, http.MethodPost, "/api/sysconfig", body)
	if err != nil {
		return err
	}
	log.Info().Str("controller", a.ctrl.Name).
		Str("server", fmt.Sprintf("%s:%d%s", host, port, path)).
		Msg("контроллеру СКУД прописан адрес сервера")
	return nil
}
