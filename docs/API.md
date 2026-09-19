# REST API

Базовый адрес: `http://<сервер>:8080/api/v1`

Актуальный список эндпоинтов на работающем сервере: `GET /api/v1/docs`

---

## Аутентификация

Схема: **JWT (HS256)** в заголовке `Authorization: Bearer <token>`.

### Вход

```http
POST /api/v1/auth/login
Content-Type: application/json

{"username": "admin", "password": "admin123"}
```

Ответ:

```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_at": 1789661742,
  "user": {
    "id": "91ffdea6-5ac0-4e8a-bd78-089228573f5e",
    "username": "admin",
    "role": "admin",
    "permissions": {"all": true}
  }
}
```

Токен действует **24 часа**. Передавайте его во всех защищённых запросах:

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}' | jq -r .token)

curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/cameras
```

### Исключения

Два эндпоинта принимают токен в query-параметре, потому что обращаются к ним
браузерные теги `<video>` и `<img>`, не умеющие отправлять заголовки:

| Эндпоинт | Параметр |
|---|---|
| `GET /cameras/{id}/hls/*` | `?token=<jwt>` |
| `GET /cameras/{id}/snapshot` | `?jwt=<jwt>` или `?token=<jwt>` |

Также без авторизации: `POST /auth/login`, `GET /docs`, `GET /health`.

---

## Коды ответов

| Код | Значение |
|---|---|
| 200 | Успех |
| 201 | Объект создан |
| 204 | Успех, тело ответа пустое (удаление) |
| 400 | Некорректный запрос или тело |
| 401 | Токен отсутствует, истёк или неверен |
| 404 | Объект не найден |
| 502 | Камера или внешний сервис недоступен |
| 503 | Поток временно недоступен |
| 504 | Камера не отдаёт видеоданные |

Ошибки возвращаются в виде:

```json
{"error": "camera not found"}
```

---

## Камеры

### Список

```http
GET /api/v1/cameras
```

```json
[
  {
    "id": "00b31acf-7f5b-4567-ae11-0015856b19b5",
    "name": "Камера 192.168.1.5",
    "rtsp_url": "",
    "main_stream": "rtsp://192.168.1.5/stream=0",
    "sub_stream": "rtsp://192.168.1.5/stream=1",
    "ip": "192.168.1.5",
    "mac": "e4:e6:6c:58:fd:42",
    "firmware": "os02g10_i2c_1080p 1920x1080 sub:704x576",
    "status": "online",
    "ptz": false,
    "settings": {"username": "root", "password": "..."},
    "created_at": "2026-08-26T05:36:16Z",
    "updated_at": "2026-09-16T05:07:20Z"
  }
]
```

Поле `status` вычисляется автоматически (см. раздел «Мониторинг»).

### Создание

```http
POST /api/v1/cameras
```

```json
{
  "name": "Камера вход",
  "ip": "192.168.1.10",
  "main_stream": "rtsp://192.168.1.10/stream=0",
  "sub_stream": "rtsp://192.168.1.10/stream=1",
  "username": "root",
  "password": "secret",
  "ptz": false,
  "wg_ip": "10.99.0.2"
}
```

Обязательное поле — только `name`. При создании бэкенд регистрирует пути
в медиасервере и подставляет учётные данные в RTSP-адреса.

### Изменение

```http
PATCH /api/v1/cameras/{id}
```

Передаются только изменяемые поля:

```json
{"name": "Новое имя", "ptz": true}
```

Если меняются потоки или учётные данные, пути в медиасервере
перерегистрируются автоматически.

### Удаление

```http
DELETE /api/v1/cameras/{id}
```

Ответ `204`. Пути удаляются из медиасервера.

---

## Потоки

### Информация о потоках

```http
GET /api/v1/cameras/{id}/stream
```

```json
{
  "rtsp_url": "rtsp://localhost:8554/00b31acf-...",
  "hls_url": "/api/v1/cameras/00b31acf-.../hls/index.m3u8",
  "webrtc_url": "http://localhost:8889/00b31acf-...",
  "status": "online",
  "main_hls_url": "/api/v1/cameras/00b31acf-.../hls/index.m3u8",
  "sub_hls_url": "/api/v1/cameras/00b31acf-.../hls/sub/index.m3u8",
  "main_rtsp_url": "rtsp://192.168.1.5/stream=0",
  "sub_rtsp_url": "rtsp://192.168.1.5/stream=1",
  "snapshot_url": "http://192.168.1.5/image.jpg"
}
```

### Просмотр в браузере

```javascript
import Hls from 'hls.js'

const token = localStorage.getItem('token')
const hls = new Hls()
hls.loadSource(`/api/v1/cameras/${id}/hls/sub/index.m3u8?token=${token}`)
hls.attachMedia(document.querySelector('video'))
```

| Поток | Путь |
|---|---|
| Основной | `/api/v1/cameras/{id}/hls/index.m3u8?token=<jwt>` |
| Дополнительный | `/api/v1/cameras/{id}/hls/sub/index.m3u8?token=<jwt>` |

Признак дополнительного потока передаётся **в пути** (`/hls/sub/`), а не в
query-параметре: hls.js разрешает относительные ссылки из плейлиста сам
и не переносит query в запросы сегментов.

### Снапшот

```http
GET /api/v1/cameras/{id}/snapshot?jwt=<token>
```

Возвращает `image/jpeg`. Бэкенд сам определяет подходящий путь для
производителя камеры, а при неудаче извлекает кадр из HLS-потока через ffmpeg.

---

## PTZ (поворотные камеры)

Управление по протоколу **ONVIF**. Работает для камер, у которых в
настройках включён флаг `ptz`.

### Текущее положение

```http
GET /api/v1/cameras/{id}/ptz/status
```

```json
{"pan": -0.3706, "tilt": -0.3756, "zoom": 0, "supports_ptz": true}
```

Если камера не поддерживает PTZ, вернётся `{"supports_ptz": false}`.

### Движение

```http
POST /api/v1/cameras/{id}/ptz/move
```

```json
{"pan": 0.4, "tilt": 0, "zoom": 0, "duration_ms": 500}
```

| Поле | Диапазон | Описание |
|---|---|---|
| `pan` | -1 … 1 | Влево/вправо |
| `tilt` | -1 … 1 | Вниз/вверх |
| `zoom` | -1 … 1 | Отдалить/приблизить |
| `duration_ms` | 100 … 5000 | Время движения, затем автостоп |

Значения вне диапазона ограничиваются. Камера останавливается автоматически.

Пример поворота влево:

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"pan":-0.4,"tilt":0,"zoom":0,"duration_ms":400}' \
  http://localhost:8080/api/v1/cameras/$ID/ptz/move
```

### Остановка

```http
POST /api/v1/cameras/{id}/ptz/stop
```

### Пресеты

```http
GET  /api/v1/cameras/{id}/ptz/presets
POST /api/v1/cameras/{id}/ptz/presets/goto   {"token": "1"}
```

Если камера не поддерживает пресеты, возвращается пустой массив.

---

## Управление камерой

Команды выполняются по SSH (OpenIPC/Majestic).

```http
POST /api/v1/cameras/{id}/restart-streamer
POST /api/v1/cameras/{id}/reboot
```

```json
{"command": "restart majestic", "success": true}
```

После перезапуска стримера поток поднимается примерно за 20 секунд,
после перезагрузки камера недоступна около минуты.

---

## События детекции

```http
GET /api/v1/events?camera_id=<uuid>&page=1&page_size=20
```

```json
{
  "events": [
    {
      "id": "…",
      "camera_id": "…",
      "timestamp": "2026-09-16T19:52:09Z",
      "object_class": "person",
      "confidence": 0.87,
      "bbox": {"x": 412, "y": 180, "w": 96, "h": 240},
      "track_id": 14
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

```http
GET /api/v1/events/{id}
```

---

## Архив записей

```http
GET /api/v1/recordings?camera_id=<uuid>&page=1&page_size=20
```

Каждая запись содержит временную ссылку на файл в хранилище:

```json
{
  "recordings": [
    {
      "id": "…",
      "camera_id": "…",
      "start_time": "2026-09-16T19:00:00Z",
      "end_time": "2026-09-16T19:05:00Z",
      "duration": 300.0,
      "file_path": "<uuid>/2026-09-16/190000.mp4",
      "file_size": 104857600,
      "url": "http://host:9000/nvr-recordings/…?X-Amz-Signature=…"
    }
  ],
  "total": 1, "page": 1, "page_size": 20
}
```

Ссылка `url` действует **час**. Её можно открыть напрямую в браузере или
в плеере.

```http
GET    /api/v1/recordings/{id}    # одна запись со свежей ссылкой
DELETE /api/v1/recordings/{id}    # удаляет и запись, и файл из хранилища
```

---

## СКУД

```http
GET    /api/v1/acs/controllers
POST   /api/v1/acs/controllers
GET    /api/v1/acs/controllers/{id}
DELETE /api/v1/acs/controllers/{id}
GET    /api/v1/acs/events?page=1&page_size=20
POST   /api/v1/acs/doors/{controllerID}/open     {"door_id": "1"}
POST   /api/v1/acs/ingest                        (публично, приём событий от контроллера)
```

Создание контроллера:

```json
{
  "name": "Турникет главного входа",
  "vendor": "skud",
  "ip": "192.168.1.50",
  "port": 80,
  "login": "admin",
  "password": "secret"
}
```

Поддерживаемые значения `vendor`: `hikvision`, `dahua`, `promwad`, `skud`.

### Приём событий от контроллера (ingest)

Публичный эндпоинт для push-событий от контроллера SKUD (ESP32-P4).
Контроллер присылает JSON; сопоставление с контроллером — по IP отправителя.

```json
POST /api/v1/acs/ingest
{
  "device_id": "SKUD-01",
  "event_type": "access_granted",
  "facility": 0,
  "card_number": "12345",
  "name": "Иванов Иван",
  "timestamp": 1789857349,
  "flags": 1
}
```

Ответ: `201 {"status":"ok"}`.

Прошивка и полное описание API самого контроллера — в
[`firmware/skud-esp32-p4/`](../../firmware/skud-esp32-p4/README.md).

---

## Сканер камер

```http
POST /api/v1/scanner/scan
```

```json
{"subnet": "192.168.1.0/24", "username": "root", "password": "secret"}
```

Ответ:

```json
{
  "subnet": "192.168.1.0/24",
  "total": 254,
  "found": 3,
  "cameras": [
    {
      "ip": "192.168.1.5",
      "mac": "e4:e6:6c:58:fd:42",
      "vendor": "openipc",
      "model": "gk7205v200",
      "firmware": "os02g10_i2c_1080p",
      "main_stream": "rtsp://192.168.1.5/stream=0",
      "sub_stream": "rtsp://192.168.1.5/stream=1",
      "online": true
    }
  ]
}
```

Опрос одной камеры:

```http
POST /api/v1/scanner/probe    {"ip": "192.168.1.10"}
```

---

## Статистика

```http
GET /api/v1/stats
```

```json
{
  "total_cameras": 13,
  "online_cameras": 13,
  "total_events_24h": 0,
  "disk_used_gb": 45.58,
  "disk_total_gb": 105.35,
  "acs_total": 0,
  "acs_online": 0
}
```

---

## Служебные

```http
GET /health          # {"status":"ok"} — без авторизации
GET /api/v1/docs     # описание API в формате JSON
```

---

## Мониторинг статуса камер

Поле `status` обновляется автоматически. Раз в 15 секунд фоновый сервис
опрашивает медиасервер и выставляет:

| Значение | Условие |
|---|---|
| `online` | Источник подключён и отдаёт хотя бы один трек |
| `offline` | Поток не поднимается |
| `recording` | Установлен вручную, автопроверкой не перезаписывается |

Тот же сервис удаляет «висячие» пути медиасервера, оставшиеся от
удалённых камер.

---

## Пример: полный сценарий

```bash
BASE=http://localhost:8080/api/v1

# 1. Авторизация
TOKEN=$(curl -s -X POST $BASE/auth/login -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}' | jq -r .token)

# 2. Поиск камер в сети
curl -s -X POST $BASE/scanner/scan -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"subnet":"192.168.1.0/24"}' | jq '.cameras[] | {ip, vendor}'

# 3. Добавление камеры
CAM=$(curl -s -X POST $BASE/cameras -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Вход","ip":"192.168.1.10",
       "main_stream":"rtsp://192.168.1.10/stream=0",
       "sub_stream":"rtsp://192.168.1.10/stream=1",
       "username":"root","password":"secret"}' | jq -r .id)

# 4. Проверка статуса
curl -s -H "Authorization: Bearer $TOKEN" $BASE/cameras/$CAM | jq '{name, status}'

# 5. Поворот камеры (если PTZ)
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"pan":0.4,"tilt":0,"zoom":0,"duration_ms":500}' \
  $BASE/cameras/$CAM/ptz/move
```
