# REST API контроллера SKUD (ESP32-P4)

Базовый адрес: `http://<IP контроллера>` (порт 80).

Все эндпоинты требуют **HTTP Basic Auth**. Учётные данные задаются в
системной конфигурации (по умолчанию `admin`). При неудачной авторизации
сервер возвращает `401` с заголовком `WWW-Authenticate: Basic realm="SKUD"`.

```
Authorization: Basic base64("<login>:<password>")
```

Пример заголовка для `admin:admin123`:

```bash
curl -u admin:admin123 http://192.168.1.50/api/status
# либо вручную:
curl -H "Authorization: Basic $(echo -n 'admin:admin123' | base64)" \
     http://192.168.1.50/api/status
```

## Коды ответов

| Код | Значение |
|---|---|
| 200 | Успех |
| 400 | Ошибка (некорректный JSON, не создан файл и т.п.) |
| 401 | Не авторизован |
| 404 | Неизвестный путь |

Тело ошибки, где применимо, содержит `{"error": "<причина>"}`.

---

## Пины

### `GET /api/config`

Возвращает текущую конфигурацию пинов.

```json
{
  "lock_gpio": 8, "rte_gpio": -1, "sensor_gpio": -1,
  "wiegand_d0": 4, "wiegand_d1": 5,
  "wiegand2_d0": -1, "wiegand2_d1": -1,
  "buzzer_gpio": -1, "lock_pulse_ms": 3000
}
```

Значение `-1` = GPIO не используется.

### `POST /api/config`

Сохраняет пины и применяет их «на лету». Тело — объект с любым подмножеством
полей из `GET /api/config`.

```bash
curl -u admin:admin123 -X POST http://192.168.1.50/api/config \
  -H 'Content-Type: application/json' \
  -d '{"lock_gpio":8,"wiegand_d0":4,"wiegand_d1":5,"lock_pulse_ms":3000}'
```

Ответ: `{"ok": true, "info": "applied live"}`.

---

## Cистемная конфигурация

### `GET /api/sysconfig`

```json
{
  "login": "admin", "password": "admin123",
  "syslog_enabled": false, "syslog_host": "", "syslog_port": 514,
  "net_mode": 0, "ip": "192.168.1.50", "mask": "255.255.255.0", "gw": "192.168.1.1",
  "op_mode": 0, "server_host": "", "server_port": 8080, "server_path": "/api/v1/acs/events",
  "door_type": 0, "antipassback": false,
  "weekly": [{"day":0,"enabled":0,"on":0,"off":0}, ...],
  "floating_enabled": false, "floating_start": 0, "floating_end": 0,
  "ntp_server": "pool.ntp.org", "ntp_interval_sec": 3600, "timezone_min": 180,
  "device_name": "", "device_location": ""
}
```

Поля:

| Поле | Тип | Описание |
|---|---|---|
| `net_mode` | int | `0`=DHCP, `1`=статический |
| `op_mode` | int | `0`=автономный, `1`=сетевой (пушить события на сервер) |
| `server_host`/`server_port`/`server_path` | str/int/str | Куда слать события в сетевом режиме |
| `door_type` | int | `0`=1 считыватель+кнопка, `1`=2 считывателя |
| `antipassback` | bool | строгий антипасбек |
| `timezone_min` | int | смещение от UTC в минутах (+180 = МСК) |

### `POST /api/sysconfig`

Сохраняет системную конфигурацию. Тело — объект с любым подмножеством полей.

```bash
curl -u admin:admin123 -X POST http://192.168.1.50/api/sysconfig \
  -H 'Content-Type: application/json' \
  -d '{"op_mode":1,"server_host":"192.168.1.10","server_port":8080,"server_path":"/api/v1/acs/ingest"}'
```

---

## Статус

### `GET /api/status`

```json
{
  "uptime_sec": 12345,
  "free_heap": 180000,
  "now": 1789857349,
  "time_str": "2026-09-20 12:35:49",
  "time_synced": true,
  "device_name": "Проходная",
  "device_location": "Вход №1",
  "card_count": 42,
  "event_count": 137
}
```

---

## Дверь

### `POST /api/door/open`

Открывает дверь (импульс замка). Кнопка выхода (RTE) не требуется.

```bash
curl -u admin:admin123 -X POST http://192.168.1.50/api/door/open
```

Ответ: `{"ok": true}`.

---

## Журнал событий

### `GET /api/log`

Возвращает последние 200 записей (журнал хранит до 1000).

```json
{
  "events": [
    {"ts":1789857349,"type":0,"facility":0,"card":1234,"note":"","flags":1},
    ...
  ],
  "count": 137
}
```

`type` (см. `event_type_t` в `components/event_log/include/event_log.h`):

| Значение | Событие |
|---|---|
| 0 | `EVT_CARD_GRANTED` — доступ разрешён |
| 1 | `EVT_CARD_DENIED` — отказ |
| 2 | `EVT_RTE_PRESSED` — кнопка выхода |
| 3 | `EVT_DOOR_OPEN` — дверь открыта (геркон) |
| 4 | `EVT_DOOR_CLOSED` — дверь закрыта |
| 5 | `EVT_DOOR_FORCED` — взлом |
| 6 | `EVT_SYSTEM_START` — старт контроллера |
| 7 | `EVT_AUTH_FAIL` — неудачный вход |
| 8 | `EVT_CONFIG_CHANGED` — изменена конфигурация |

`flags` bit0 = проход завершён (дверь открылась после разрешения).

### `POST /api/log/clear`

Очищает журнал. Ответ: `{"ok": true}`.

---

## Карты

Карта описывается объектом:

```json
{"facility": 0, "card": 12345, "name": "Иванов Иван",
 "access": 0, "group": "", "active": true}
```

| Поле | Тип | Описание |
|---|---|---|
| `facility` | int (0-255) | код фасилити |
| `card` | int (0-65535) | номер карты |
| `name` | str | ФИО владельца |
| `access` | int | `0`=постоянный, `1`=по расписанию |
| `group` | str | группа (опционально) |
| `active` | bool | разрешена/заблокирована |

### `GET /api/cards`

```json
{"cards": [ ... ], "count": 42}
```

### `POST /api/cards/add`

Добавляет карту. Ошибки: `full` (лимит) или `exists` (уже есть).

```bash
curl -u admin:admin123 -X POST http://192.168.1.50/api/cards/add \
  -H 'Content-Type: application/json' \
  -d '{"facility":0,"card":12345,"name":"Иванов","access":0,"active":true}'
```

### `POST /api/cards/update`

Обновляет карту по `facility`+`card`. Ошибка `not found`, если нет такой карты.

### `POST /api/cards/remove`

```bash
curl -u admin:admin123 -X POST http://192.168.1.50/api/cards/remove \
  -H 'Content-Type: application/json' -d '{"facility":0,"card":12345}'
```

### `POST /api/cards/clear`

Удаляет все карты.

### `GET /api/cards/export`

Возвращает массив JSON всех карт (пригоден для сохранения в файл,
заголовок `Content-Disposition: attachment; filename="cards.json"`).

### `POST /api/cards/import`

Импортирует массив карт (тело — JSON-массив). Существующие карты обновляются,
новые добавляются.

Ответ: `{"ok": true, "imported": 3, "total": 42}`.

### Обучение карты с считывателя

```bash
# Включить режим обучения (следующая поднесённая карта будет добавлена):
curl -u admin:admin123 -X POST http://192.168.1.50/api/cards/learn \
  -H 'Content-Type: application/json' -d '{"name":"Новый сотрудник"}'

# Статус:
curl -u admin:admin123 http://192.168.1.50/api/cards/learn
# -> {"learn_active": true}

# Отменить:
curl -u admin:admin123 -X POST http://192.168.1.50/api/cards/learn/cancel
```

---

## Время

### `POST /api/time/sync`

Немедленная синхронизация времени по SNTP. Ответ: `{"ok": true}`.

---

## OTA-обновление

### `POST /api/ota`

Загружает прошивку (тело — сырой `.bin`). После успешной записи контроллер
автоматически перезагружается.

```bash
curl -u admin:admin123 -X POST http://192.168.1.50/api/ota \
  --data-binary @build/skud_esp32_p4.bin
```

Ответ на успешную запись: `{"ok": true}` (дальше — перезагрузка).
