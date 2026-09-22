package acs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Обновление прошивки контроллера по OTA.
//
// Прошивка передаётся на контроллер одним HTTP-запросом (POST /api/ota) в
// виде сырого бинарника. Контроллер пишет образ во второй OTA-раздел и
// перезагружается в него.
//
// Проверить результат сразу после запроса нельзя: устройство уходит в
// перезагрузку и какое-то время недоступно. Поэтому обновление выполняется
// в два шага — загрузка образа и последующая проверка версии после старта.

// otaUploadTimeout — сколько ждать загрузки образа.
//
// Прошивка около мегабайта и передаётся в локальной сети, но запись во
// flash идёт медленно и с проверкой целостности, поэтому таймаут с запасом.
const otaUploadTimeout = 5 * time.Minute

// FirmwareInfo описывает установленную прошивку контроллера.
type FirmwareInfo struct {
	Version string `json:"version"`
	Build   string `json:"build"`
}

// GetFirmwareInfo читает версию прошивки с контроллера.
//
// Версия обязательна: без неё нельзя понять, применилось ли обновление.
// Контроллеры со старыми прошивками её не отдают — это распознаётся как
// пустая версия, и такая прошивка считается не подлежащей проверке.
func (a *SkudAdapter) GetFirmwareInfo(ctx context.Context) (*FirmwareInfo, error) {
	data, err := a.call(ctx, http.MethodGet, "/api/status", nil)
	if err != nil {
		return nil, err
	}

	var st struct {
		Version string `json:"fw_version"`
		Build   string `json:"fw_build"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("разобрать версию прошивки: %w", err)
	}

	return &FirmwareInfo{Version: st.Version, Build: st.Build}, nil
}

// UploadFirmware отправляет образ прошивки на контроллер.
//
// Вызов возвращается, когда образ записан и контроллер начал
// перезагрузку, — либо сразу с ошибкой, если запись не началась.
func (a *SkudAdapter) UploadFirmware(ctx context.Context, image []byte) error {
	if len(image) == 0 {
		return fmt.Errorf("образ прошивки пуст")
	}

	// Таймаут отдельный: общая пятисекундная настройка клиента для
	// загрузки мегабайта не подходит.
	ctx, cancel := context.WithTimeout(ctx, otaUploadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL()+"/api/ota", bytes.NewReader(image))
	if err != nil {
		return err
	}
	req.SetBasicAuth(a.login, a.password)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(image))

	client := &http.Client{Timeout: otaUploadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		// Контроллер перезагружается только после полной записи образа,
		// поэтому обрыв соединения — это ошибка передачи, а не признак
		// успеха. Проверить это по одной лишь ошибке нельзя, поэтому
		// возвращаем её наверх: вердикт о результате даётся по версии
		// прошивки после перезагрузки.
		return fmt.Errorf("передать прошивку: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("неверный логин или пароль контроллера")
	case resp.StatusCode >= 400:
		msg := string(data)
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		return fmt.Errorf("контроллер отклонил прошивку (%d): %s", resp.StatusCode, msg)
	}

	// Успешный ответ содержит {"ok":true}; при ok=false образ записан не
	// был, и контроллер остался на прежней прошивке.
	var r struct {
		OK *bool `json:"ok"`
	}
	if json.Unmarshal(data, &r) == nil && r.OK != nil && !*r.OK {
		return fmt.Errorf("контроллер не принял прошивку")
	}

	return nil
}

// VerifyFirmwareVersion ждёт, пока контроллер поднимется после обновления,
// и возвращает установленную версию.
//
// Устройство перезагружается и отвечает не сразу, поэтому опрос идёт с
// паузами. Ожидание ограничено: если контроллер не поднялся, это означает
// неудачную прошивку, и о таком надо сообщить, а не ждать бесконечно.
func (a *SkudAdapter) VerifyFirmwareVersion(ctx context.Context, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("контроллер не ответил после обновления за %s", timeout)
		}

		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		info, err := a.GetFirmwareInfo(pingCtx)
		cancel()

		if err == nil {
			return info.Version, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}
