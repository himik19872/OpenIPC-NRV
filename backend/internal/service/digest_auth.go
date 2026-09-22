package service

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Digest-аутентификация для камер сторонних производителей.
//
// Камеры OpenIPC и большинство современных устройств принимают Basic:
// логин и пароль передаются в заголовке Authorization. Часть старых
// камер (Vivotek, ряд китайских моделей) требует Digest — они отвечают
// 401 с заголовком WWW-Authenticate: Digest, а Basic-заголовок игнорируют.
//
// Внешние библиотеки для этого не нужны: алгоритм небольшой, а лишняя
// зависимость в проекте, где уже есть свой HTTP-клиент, не оправдана.

// digestChallenge — разобранные параметры запроса Digest от камеры.
type digestChallenge struct {
	realm     string
	nonce     string
	qop       string
	opaque    string
	algorithm string
}

// doDigest выполняет запрос с Digest-аутентификацией.
//
// Запрос выполняется дважды: первый получает от камеры параметры
// (realm, nonce), второй несёт вычисленный ответ. Так работает протокол —
// параметры приходят именно в ответе 401.
func doDigest(client *http.Client, req *http.Request, username, password string) (*http.Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	// Камера приняла запрос без аутентификации — ничего считать не нужно.
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	header := resp.Header.Get("WWW-Authenticate")
	resp.Body.Close()

	challenge := parseDigestChallenge(header)
	if challenge == nil {
		// Требуется не Digest — возвращаем исходный ответ, чтобы
		// вызывающий код увидел настоящую причину отказа.
		return nil, fmt.Errorf("камера требует аутентификации, но не Digest")
	}

	// Для повторного запроса нужно тело заново: у первого оно уже прочитано
	// и закрыто. У запросов кадра тела нет, поэтому пересоздаём без него.
	retry := req.Clone(req.Context())
	retry.Header.Set("Authorization", buildDigestAuth(req.Method, req.URL.RequestURI(), username, password, challenge))

	resp2, err := client.Do(retry)
	if err != nil {
		return nil, err
	}
	return resp2, nil
}

// parseDigestChallenge разбирает заголовок WWW-Authenticate.
func parseDigestChallenge(header string) *digestChallenge {
	if !strings.HasPrefix(strings.ToLower(header), "digest ") {
		return nil
	}

	ch := &digestChallenge{algorithm: "MD5"}
	// Параметры идут в виде key="value" через запятую. Разделяем аккуратно:
	// значения могут содержать запятые внутри кавычек.
	rest := header[len("Digest "):]
	for _, part := range splitDigestParams(rest) {
		eq := strings.Index(part, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(part[:eq])
		value := strings.Trim(strings.TrimSpace(part[eq+1:]), `"`)

		switch strings.ToLower(key) {
		case "realm":
			ch.realm = value
		case "nonce":
			ch.nonce = value
		case "qop":
			ch.qop = value
		case "opaque":
			ch.opaque = value
		case "algorithm":
			ch.algorithm = value
		}
	}

	// Без realm и nonce ответ вычислить нельзя.
	if ch.realm == "" || ch.nonce == "" {
		return nil
	}
	return ch
}

// splitDigestParams делит строку параметров по запятым, не трогая те,
// что находятся внутри кавычек (в nonce они встречаются).
func splitDigestParams(s string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false

	for _, r := range s {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			current.WriteRune(r)
		case r == ',' && !inQuotes:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// buildDigestAuth вычисляет заголовок Authorization для Digest-запроса.
//
// Формула из RFC 2617: ответ считается от логина, области, пароля, метода
// и адреса запроса вместе с одноразовым числом от камеры.
func buildDigestAuth(method, uri, username, password string, ch *digestChallenge) string {
	ha1 := md5Hex(username + ":" + ch.realm + ":" + password)
	ha2 := md5Hex(method + ":" + uri)

	var response string
	// qop=auth означает расчёт через счётчик запросов; если параметра нет,
	// используется простой вариант из ранней версии протокола.
	if ch.qop == "" {
		response = md5Hex(ha1 + ":" + ch.nonce + ":" + ha2)
		auth := fmt.Sprintf(
			`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
			username, ch.realm, ch.nonce, uri, response)
		if ch.opaque != "" {
			auth += fmt.Sprintf(`, opaque="%s"`, ch.opaque)
		}
		return auth
	}

	// С qop нужен счётчик запросов и случайная строка от клиента.
	nc := "00000001"
	cnonce := md5Hex(fmt.Sprintf("%d", len(username)+len(ch.nonce)))
	response = md5Hex(ha1 + ":" + ch.nonce + ":" + nc + ":" + cnonce + ":" + ch.qop + ":" + ha2)

	auth := fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=%s, response="%s", qop=%s, nc=%s, cnonce="%s"`,
		username, ch.realm, ch.nonce, uri, ch.algorithm, response, ch.qop, nc, cnonce)
	if ch.opaque != "" {
		auth += fmt.Sprintf(`, opaque="%s"`, ch.opaque)
	}
	return auth
}

// md5Hex считает MD5-хеш строки в шестнадцатеричном виде.
func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// drainAndClose освобождает соединение, дочитывая тело ответа.
// Без этого HTTP-клиент не переиспользует соединение к камере.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
}
