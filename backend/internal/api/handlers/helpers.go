package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxRequestBody — предел размера тела запроса.
// Снимки справочников передаются в base64, поэтому лимит с запасом.
const maxRequestBody = 12 << 20 // 12 МБ

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// decodeJSONBody разбирает JSON из тела запроса с ограничением размера.
// Возвращает понятную ошибку вместо «unexpected EOF» при пустом теле.
func decodeJSONBody(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("тело запроса пусто")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxRequestBody))
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("тело запроса пусто")
		}
		return fmt.Errorf("некорректный JSON: %w", err)
	}
	return nil
}

// credentialsFromSettings достаёт логин и пароль камеры из JSONB-поля settings.
func credentialsFromSettings(settings map[string]any) (string, string) {
	if settings == nil {
		return "", ""
	}
	username, _ := settings["username"].(string)
	password, _ := settings["password"].(string)
	return username, password
}

// clientIP возвращает IP клиента.
//
// r.RemoteAddr — это адрес узла, который физически подключился, то есть при
// работе через прокси там будет адрес прокси. RealIP-middleware кладёт
// настоящий адрес в X-Real-IP, поэтому сначала смотрим туда. Порт отрезаем:
// сравнивать адреса удобнее без него.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	host := r.RemoteAddr
	// Адрес приходит в формате "ip:port"; IPv6 — в квадратных скобках.
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return strings.Trim(host[:i], "[]")
	}
	return host
}
