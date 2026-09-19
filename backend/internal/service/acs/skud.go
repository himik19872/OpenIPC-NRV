package acs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nvr/backend/internal/domain"
)

// SkudAdapter — адаптер для контроллера СКУД на ESP32-P4 (наш OpenIPC-NRV SKUD).
//
// Контроллер предоставляет собственный REST API (HTTP Basic Auth) на порту 80:
//
//	GET  /api/status      — статус (uptime, время, счётчики, имя)
//	POST /api/door/open   — открыть дверь
//	GET  /api/log         — журнал событий (для SubscribeEvents)
//
// События контроллер при этом сам пушит на сервер (POST /api/v1/acs/ingest),
// поэтому SubscribeEvents здесь опрашивает журнал только как резервный канал.
type SkudAdapter struct {
	ctrl     *domain.ACSController
	baseURL  string
	login    string
	password string
	client   *http.Client
}

func NewSkudAdapter(ctrl *domain.ACSController) (Adapter, error) {
	login, _ := ctrl.Credentials["login"].(string)
	password, _ := ctrl.Credentials["password"].(string)

	if login == "" || password == "" {
		return nil, fmt.Errorf("skud: login and password required")
	}

	port := ctrl.Port
	if port == 0 {
		port = 80
	}

	return &SkudAdapter{
		ctrl:     ctrl,
		baseURL:  fmt.Sprintf("http://%s:%d", ctrl.IP, port),
		login:    login,
		password: password,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func (a *SkudAdapter) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(a.login, a.password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return a.client.Do(req)
}

func (a *SkudAdapter) Ping(ctx context.Context) error {
	resp, err := a.do(ctx, "GET", "/api/status", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("skud ping failed: %d", resp.StatusCode)
	}
	return nil
}

func (a *SkudAdapter) ListDoors(ctx context.Context) ([]Door, error) {
	// Контроллер управляет одной дверью (одна точка прохода).
	return []Door{
		{ID: "door_1", Name: "Дверь", Status: "locked"},
	}, nil
}

func (a *SkudAdapter) OpenDoor(ctx context.Context, doorID string) error {
	resp, err := a.do(ctx, "POST", "/api/door/open", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("skud open door failed: %d", resp.StatusCode)
	}
	return nil
}

func (a *SkudAdapter) GetDoorStatus(ctx context.Context, doorID string) (DoorStatus, error) {
	// Контроллер не отдаёт состояние геркона в /api/status напрямую;
	// статус двери берётся из журнала. Возвращаем «закрыто/заперто».
	return DoorStatus{Locked: true, Open: false, Alarm: false}, nil
}

// SubscribeEvents — резервный канал: периодически опрашивает /api/log и
// транслирует новые записи в канал ACSEvent. Основной канал — push на ingest.
func (a *SkudAdapter) SubscribeEvents(ctx context.Context) (<-chan domain.ACSEvent, error) {
	ch := make(chan domain.ACSEvent, 100)

	go func() {
		defer close(ch)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		seen := uint64(0) // последняя обработанная запись (по ts)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				events, err := a.pollLog(ctx)
				if err != nil {
					continue
				}
				for _, ev := range events {
					if ev.Timestamp.UnixNano() <= int64(seen) {
						continue
					}
					seen = uint64(ev.Timestamp.UnixNano())
					select {
					case ch <- ev:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return ch, nil
}

type skudLogRecord struct {
	Ts       int    `json:"ts"`
	Type     int    `json:"type"`
	Facility int    `json:"facility"`
	Card     int    `json:"card"`
	Note     string `json:"note"`
	Flags    int    `json:"flags"`
}

type skudLogResponse struct {
	Events []skudLogRecord `json:"events"`
	Count  int             `json:"count"`
}

// mapSkudType сопоставляет числовой тип события контроллера строковому типу NVR.
func mapSkudType(t int) string {
	switch t {
	case 0:
		return "access_granted"
	case 1:
		return "access_denied"
	case 2:
		return "rte_pressed"
	case 3:
		return "door_open"
	case 4:
		return "door_closed"
	case 5:
		return "door_forced"
	case 6:
		return "system_start"
	case 7:
		return "auth_fail"
	case 8:
		return "config_changed"
	default:
		return "unknown"
	}
}

func (a *SkudAdapter) pollLog(ctx context.Context) ([]domain.ACSEvent, error) {
	resp, err := a.do(ctx, "GET", "/api/log", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("skud log failed: %d", resp.StatusCode)
	}

	var logResp skudLogResponse
	if err := json.NewDecoder(resp.Body).Decode(&logResp); err != nil {
		return nil, err
	}

	out := make([]domain.ACSEvent, 0, len(logResp.Events))
	for _, rec := range logResp.Events {
		out = append(out, domain.ACSEvent{
			ControllerID: a.ctrl.ID,
			DoorID:       "door_1",
			EventType:    mapSkudType(rec.Type),
			CardNumber:   fmt.Sprintf("%d", rec.Card),
			Timestamp:    time.Unix(int64(rec.Ts), 0),
			Metadata: map[string]any{
				"facility": rec.Facility,
				"note":     rec.Note,
				"flags":    rec.Flags,
			},
		})
	}
	return out, nil
}
