package handlers

import (
	"net/http"
	"sort"
	"strings"
)

// APIDoc — описание одного эндпоинта для встроенной документации.
type APIDoc struct {
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Summary     string   `json:"summary"`
	Auth        bool     `json:"auth"`
	Tags        []string `json:"tags"`
	QueryParams []string `json:"query_params,omitempty"`
	Body        string   `json:"body,omitempty"`
}

// APIDocHandler отдаёт машиночитаемое описание API.
//
// Отдельная страница Swagger не подключена намеренно: список эндпоинтов
// невелик, а генератор добавил бы крупную зависимость. Документация
// поддерживается в актуальном виде тестом api_doc_test.go.
type APIDocHandler struct {
	routes []APIDoc
}

func NewAPIDocHandler() *APIDocHandler {
	d := &APIDocHandler{}
	d.routes = buildRouteList()
	return d
}

// List возвращает список эндпоинтов.
// GET /api/v1/docs
func (h *APIDocHandler) List(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":       "v1",
		"base_url":      "/api/v1",
		"auth_scheme":   "JWT (HS256) в заголовке Authorization: Bearer <token>",
		"auth_endpoint": "POST /api/v1/auth/login",
		"default_login": "admin / admin123 (смените пароль после первого входа)",
		"endpoints":     h.routes,
		"total":         len(h.routes),
		"documentation": "https://github.com/himik19872/OpenIPC-NVR/blob/main/docs/API.md",
		"notes": []string{
			"Эндпоинты с auth=false не требуют заголовка Authorization.",
			"HLS и снапшот принимают токен в query (?token= / ?jwt=), так как браузерные теги video и img не могут отправлять заголовки.",
			"Коды ошибок: 400 — некорректный запрос, 401 — нет или истёк токен, 404 — объект не найден, 502/504 — камера недоступна.",
		},
	})
}

// buildRouteList — единый источник правды об эндпоинтах.
// Порядок и группировка соответствуют router.go.
func buildRouteList() []APIDoc {
	routes := []APIDoc{
		// --- Служебное ---
		{Method: "GET", Path: "/health", Summary: "Проверка работоспособности сервиса", Tags: []string{"system"}},
		{Method: "GET", Path: "/api/v1/docs", Summary: "Это описание API", Tags: []string{"system"}},

		// --- Авторизация ---
		{Method: "POST", Path: "/api/v1/auth/login", Summary: "Вход, возвращает JWT", Tags: []string{"auth"},
			Body: `{"username":"admin","password":"admin123"}`},

		// --- Камеры ---
		{Method: "GET", Path: "/api/v1/cameras", Summary: "Список камер", Auth: true, Tags: []string{"cameras"}},
		{Method: "POST", Path: "/api/v1/cameras", Summary: "Добавить камеру", Auth: true, Tags: []string{"cameras"},
			Body: `{"name":"Камера 1","ip":"192.168.1.10","main_stream":"rtsp://192.168.1.10/stream=0","sub_stream":"rtsp://192.168.1.10/stream=1","username":"root","password":"pass","ptz":false}`},
		{Method: "GET", Path: "/api/v1/cameras/{id}", Summary: "Информация о камере", Auth: true, Tags: []string{"cameras"}},
		{Method: "PATCH", Path: "/api/v1/cameras/{id}", Summary: "Изменить камеру (частичное обновление)", Auth: true, Tags: []string{"cameras"},
			Body: `{"name":"Новое имя","ptz":true}`},
		{Method: "DELETE", Path: "/api/v1/cameras/{id}", Summary: "Удалить камеру", Auth: true, Tags: []string{"cameras"}},

		// --- Потоки ---
		{Method: "GET", Path: "/api/v1/cameras/{id}/stream", Summary: "URL-ы потоков: RTSP, HLS, WebRTC, снапшот", Auth: true, Tags: []string{"streams"}},
		{Method: "GET", Path: "/api/v1/cameras/{id}/hls/index.m3u8", Summary: "HLS-плейлист основного потока", Auth: false, Tags: []string{"streams"},
			QueryParams: []string{"token — JWT (обязателен)"}},
		{Method: "GET", Path: "/api/v1/cameras/{id}/hls/sub/index.m3u8", Summary: "HLS-плейлист субпотока", Auth: false, Tags: []string{"streams"},
			QueryParams: []string{"token — JWT (обязателен)"}},
		{Method: "GET", Path: "/api/v1/cameras/{id}/hls/{file}", Summary: "Сегменты HLS и init-сегмент", Auth: false, Tags: []string{"streams"},
			QueryParams: []string{"token — JWT (обязателен)", "session — выдаётся MediaMTX"}},
		{Method: "GET", Path: "/api/v1/cameras/{id}/snapshot", Summary: "Текущий кадр в JPEG", Auth: false, Tags: []string{"streams"},
			QueryParams: []string{"jwt или token — JWT (обязателен, так как используется в теге img)"}},

		// --- PTZ ---
		{Method: "GET", Path: "/api/v1/cameras/{id}/ptz/status", Summary: "Текущее положение PTZ-камеры", Auth: true, Tags: []string{"ptz"}},
		{Method: "POST", Path: "/api/v1/cameras/{id}/ptz/move", Summary: "Движение камеры", Auth: true, Tags: []string{"ptz"},
			Body: `{"pan":0.4,"tilt":0,"zoom":0,"duration_ms":500} — скорости от -1 до 1, длительность 100-5000 мс`},
		{Method: "POST", Path: "/api/v1/cameras/{id}/ptz/stop", Summary: "Остановить движение", Auth: true, Tags: []string{"ptz"}},
		{Method: "GET", Path: "/api/v1/cameras/{id}/ptz/presets", Summary: "Список сохранённых позиций", Auth: true, Tags: []string{"ptz"}},
		{Method: "POST", Path: "/api/v1/cameras/{id}/ptz/presets/goto", Summary: "Перейти к позиции", Auth: true, Tags: []string{"ptz"},
			Body: `{"token":"1"}`},

		// --- Управление камерой ---
		{Method: "POST", Path: "/api/v1/cameras/{id}/restart-streamer", Summary: "Перезапустить стример камеры (Majestic, SSH)", Auth: true, Tags: []string{"control"}},
		{Method: "POST", Path: "/api/v1/cameras/{id}/reboot", Summary: "Перезагрузить камеру (SSH)", Auth: true, Tags: []string{"control"}},

		// --- События детекции ---
		{Method: "GET", Path: "/api/v1/events", Summary: "События AI-детекции с пагинацией", Auth: true, Tags: []string{"events"},
			QueryParams: []string{"camera_id", "page", "page_size"}},
		{Method: "GET", Path: "/api/v1/events/{id}", Summary: "Детали события", Auth: true, Tags: []string{"events"}},

		// --- Архив записей ---
		{Method: "GET", Path: "/api/v1/recordings", Summary: "Список записей со ссылками на файлы", Auth: true, Tags: []string{"recordings"},
			QueryParams: []string{"camera_id", "page", "page_size"}},
		{Method: "GET", Path: "/api/v1/recordings/{id}", Summary: "Запись с presigned-ссылкой на файл", Auth: true, Tags: []string{"recordings"}},
		{Method: "DELETE", Path: "/api/v1/recordings/{id}", Summary: "Удалить запись и её файл из хранилища", Auth: true, Tags: []string{"recordings"}},

		// --- СКУД ---
		{Method: "GET", Path: "/api/v1/acs/controllers", Summary: "Список контроллеров СКУД", Auth: true, Tags: []string{"acs"}},
		{Method: "POST", Path: "/api/v1/acs/controllers", Summary: "Добавить контроллер", Auth: true, Tags: []string{"acs"},
			Body: `{"name":"Турникет","vendor":"hikvision","ip":"192.168.1.50","port":80,"username":"admin","password":"pass"}`},
		{Method: "GET", Path: "/api/v1/acs/controllers/{id}", Summary: "Информация о контроллере", Auth: true, Tags: []string{"acs"}},
		{Method: "DELETE", Path: "/api/v1/acs/controllers/{id}", Summary: "Удалить контроллер", Auth: true, Tags: []string{"acs"}},
		{Method: "GET", Path: "/api/v1/acs/events", Summary: "События проходной", Auth: true, Tags: []string{"acs"},
			QueryParams: []string{"page", "page_size"}},
		{Method: "POST", Path: "/api/v1/acs/doors/{controllerID}/open", Summary: "Открыть дверь", Auth: true, Tags: []string{"acs"},
			Body: `{"door_id":"1"}`},

		// --- Сканер камер ---
		{Method: "POST", Path: "/api/v1/scanner/scan", Summary: "Сканирование подсети в поиске камер", Auth: true, Tags: []string{"scanner"},
			Body: `{"subnet":"192.168.1.0/24","username":"root","password":"pass"}`},
		{Method: "POST", Path: "/api/v1/scanner/probe", Summary: "Опрос одной камеры по IP", Auth: true, Tags: []string{"scanner"},
			Body: `{"ip":"192.168.1.10","username":"root","password":"pass"}`},

		// --- Статистика ---
		{Method: "GET", Path: "/api/v1/stats", Summary: "Сводка: камеры, события, диск, СКУД", Auth: true, Tags: []string{"system"}},
	}

	// Сортировка для стабильного вывода: по первому тегу, затем по пути.
	sort.SliceStable(routes, func(i, j int) bool {
		ti, tj := strings.Join(routes[i].Tags, ""), strings.Join(routes[j].Tags, "")
		if ti != tj {
			return ti < tj
		}
		if routes[i].Path != routes[j].Path {
			return routes[i].Path < routes[j].Path
		}
		return routes[i].Method < routes[j].Method
	})
	return routes
}
