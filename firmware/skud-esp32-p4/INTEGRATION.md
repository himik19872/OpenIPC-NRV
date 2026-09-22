# Интеграция с OpenIPC-NRV

Контроллер SKUD (ESP32-P4) интегрируется с сервером видеонаблюдения
**OpenIPC-NRV** двумя способами:

1. **Push событий** — контроллер сам отправляет события доступа на сервер
   по HTTP (режим «сетевой»).
2. **Опрос и управление** — сервер опрашивает контроллер (`GET /api/status`,
   `GET /api/log`) и открывает дверь (`POST /api/door/open`).

## 1. Настройка контроллера

В веб-интерфейсе (вкладка «Сеть») задайте:

| Поле | Значение |
|---|---|
| Режим работы | **Сетевой** (`op_mode = 1`) |
| Адрес сервера | IP сервера NVR, например `192.168.1.10` |
| Порт сервера | `8080` (порт бэкенда NVR) |
| Путь | `/api/v1/acs/ingest` |

Или через API:

```bash
curl -u admin:admin -X POST http://<ip-контроллера>/api/sysconfig \
  -H 'Content-Type: application/json' \
  -d '{"op_mode":1,"server_host":"192.168.1.10","server_port":8080,
       "server_path":"/api/v1/acs/ingest"}'
```

После этого каждое событие (доступ разрешён/отказан, открытие двери, кнопка
выхода, взлом и т.д.) будет отправляться на сервер в виде JSON:

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

Отправка асинхронная (очередь + фоновая задача), при сбое сети события не
теряются фатально — журнал остаётся в локальном littlefs.

## 2. Настройка сервера NVR

Добавьте контроллер как устройство СКУД с vendor `skud`:

```json
POST /api/v1/acs/controllers
{
  "name": "SKUD-01 Главный вход",
  "vendor": "skud",
  "ip": "192.168.1.50",
  "port": 80,
  "login": "admin",
  "password": "admin"
}
```

Сервер:

- открывает дверь через `POST /api/v1/acs/doors/{id}/open`;
- опрашивает `GET /api/status` для проверки доступности (ping);
- принимает push-события на `POST /api/v1/acs/ingest` и сохраняет их в
  таблицу `acs_events` (сопоставление с контроллером — по IP отправителя).

## 3. Сопоставление типов событий

| Тип контроллера (`event_type_t`) | Событие NVR (`event_type`) |
|---|---|
| 0 `EVT_CARD_GRANTED` | `access_granted` |
| 1 `EVT_CARD_DENIED` | `access_denied` |
| 2 `EVT_RTE_PRESSED` | `rte_pressed` |
| 3 `EVT_DOOR_OPEN` | `door_open` |
| 4 `EVT_DOOR_CLOSED` | `door_closed` |
| 5 `EVT_DOOR_FORCED` | `door_forced` |
| 6 `EVT_SYSTEM_START` | `system_start` |
| 7 `EVT_AUTH_FAIL` | `auth_fail` |
| 8 `EVT_CONFIG_CHANGED` | `config_changed` |

## 4. Сценарий «СКУД + видео»

Контроллер находится в одной сети с камерами (напр. камера над дверью).
Сервер NVR привязывает контроллер к камере (`site_id`/`camera_id`), и на
каждое событие прохода может брать кадр с камеры за 2 сек до/после события.

## 5. OTA-обновление по сети

Обновление прошивки — прямо через веб-интерфейс контроллера (панель
«Система» → «Обновление прошивки») или по API:

```bash
curl -u admin:admin -X POST http://192.168.1.50/api/ota \
  --data-binary @skud_esp32_p4.bin
```
