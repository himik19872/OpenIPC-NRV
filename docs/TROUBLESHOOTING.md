# Диагностика проблем

Разбор типовых ситуаций, с которыми пришлось столкнуться при разработке.

---

## Камеры показываются offline

**Симптом:** в интерфейсе все камеры серые, хотя они работают.

**Причина:** поле `status` в базе обновляется фоновым сервисом
`CameraStatusMonitor`. Он опрашивает медиасервер раз в 15 секунд.

**Проверка:**

```bash
# 1. Что видит медиасервер
curl -s http://localhost:9997/v3/paths/list | jq '.items[] | {name, ready}'

# 2. Что записано в базе
docker compose exec postgres psql -U nvr -d nvr -c \
  "SELECT name, ip, status FROM cameras ORDER BY ip;"

# 3. Логи мониторинга
docker compose logs backend | grep -i "status changed" | tail
```

Если в медиасервере `ready=true`, а в базе `offline` — монитор не работает:

```bash
docker compose logs backend | grep -i "camera status monitor"
```

**Частая причина:** имя пути в медиасервере не совпадает с `id` камеры.
Проверьте:

```bash
curl -s http://localhost:9997/v3/paths/list | jq -r '.items[].name' | sort
docker compose exec -T postgres psql -U nvr -d nvr -tAc "SELECT id FROM cameras;" | sort
```

Списки должны совпадать.

---

## Видео не играет, в консоли 401 или 503

**Симптом:** плеер показывает «Нет сигнала», в логах ошибки авторизации.

**Проверка по шагам:**

```bash
CID=<id камеры>
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}' | jq -r .token)

# 1. Плейлист основного потока
curl -s "http://localhost:8080/api/v1/cameras/$CID/hls/index.m3u8?token=$TOKEN" | head

# 2. Плейлист субпотока
curl -s "http://localhost:8080/api/v1/cameras/$CID/hls/sub/index.m3u8?token=$TOKEN" | head

# 3. Скачивание сегмента (подставьте имя из плейлиста)
curl -s -o /dev/null -w "%{http_code}\n" \
  "http://localhost:8080/api/v1/cameras/$CID/hls/<имя-сегмента>?token=$TOKEN"
```

**Расшифровка кодов:**

| Код | Значение | Действие |
|---|---|---|
| 401 | Токен не передан или истёк | Получите новый токен |
| 503 | Поток неактивен | Проверьте путь в медиасервере |
| 504 | Камера не отдаёт видео | Проверьте камеру напрямую по RTSP |
| 404 | Путь или сегмент не найден | Камера удалена или путь не создан |

**Проверка камеры напрямую:**

```bash
ffprobe -v error -rtsp_transport tcp -timeout 5000000 \
  -i "rtsp://root:ПАРОЛЬ@192.168.1.10/stream=0" \
  -show_entries stream=codec_name,width,height -of csv=p=0
```

Если здесь ошибка — проблема на стороне камеры, не сервера.

---

## Камера не отдаёт поток: расшифровка ошибок ffprobe

| Сообщение | Значение | Решение |
|---|---|---|
| `401 Invalid credentials` | Неверный логин или пароль | Проверьте учётные данные камеры |
| `Connection refused` | Служба RTSP не отвечает | Проверьте питание камеры, перезагрузите |
| `Connection timed out` | Камера недоступна по сети | Проверьте кабель, адрес, VLAN |
| `No route to host` | Камера отключена физически | Проверьте питание и подключение |
| `method DESCRIBE failed` | Ошибка протокола | Проверьте путь потока (`stream=0`) |

**Важно:** у камер могут быть **разные** учётные данные, даже одной модели.
Перебор для поиска рабочих (подставьте свои значения):

```bash
IP=192.168.1.10
for CRED in "root:ПАРОЛЬ1" "admin:ПАРОЛЬ2" "admin:12345" "admin:admin"; do
  U="${CRED%%:*}"; P="${CRED##*:}"
  printf "%-22s " "$CRED"
  timeout 8 ffprobe -v error -rtsp_transport tcp \
    -i "rtsp://$U:$P@$IP/stream=0" \
    -select_streams v:0 -show_entries stream=codec_name,width,height \
    -of csv=p=0 2>&1 | head -1
done
```

---

## HLS-муксер падает

**Симптом:** в логах медиасервера:

```
ERR [HLS] [muxer ...] muxer instance crashed:
    muxer error: unable to extract DTS: too many reordered frames (13)
WAR [path ...] [RTSP source] 16 RTP packets lost
```

**Причина:** поток высокого разрешения (4K) идёт по UDP, пакеты теряются,
муксер не может собрать кадр.

**Решение:** принудительный TCP-транспорт. Проверьте настройку пути:

```bash
curl -s http://localhost:9997/v3/config/paths/get/<uuid> | jq .rtspTransport
# Должно быть "tcp"
```

Применить ко всем путям:

```bash
for name in $(curl -s http://localhost:9997/v3/config/paths/list | jq -r '.items[].name'); do
  curl -s -X PATCH "http://localhost:9997/v3/config/paths/patch/$name" \
    -H 'Content-Type: application/json' -d '{"rtspTransport":"tcp"}' -o /dev/null
done
```

Для новых камер это применяется автоматически.

**Счётчик падений:**

```bash
docker compose logs mediamtx --since 5m | grep -c "muxer instance crashed"
```

---

## Снапшоты не отображаются

**Симптом:** вместо превью пустые области.

**Проверка:**

```bash
CID=<id камеры>
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}' | jq -r .token)

curl -s -o /tmp/snap.jpg -w "HTTP %{http_code} size=%{size_download}\n" \
  "http://localhost:8080/api/v1/cameras/$CID/snapshot?jwt=$TOKEN"
file /tmp/snap.jpg        # ожидается JPEG image data
```

**Если 503 и в логах камеры `JPEG encoder is not running`:**

Это особенность OpenIPC: при двух активных H.264-потоках все аппаратные
скейлеры SoC заняты, и камера не может отдать JPEG. Бэкенд автоматически
извлекает кадр из HLS-потока через `ffmpeg`.

Проверьте, что `ffmpeg` доступен в контейнере:

```bash
docker compose exec backend which ffmpeg
```

Если его нет — пересоберите образ:

```bash
docker compose build backend && docker compose up -d backend
```

---

## Субпоток не работает, а основной работает

**Симптом:** основной поток играет, при переключении на субпоток — ошибка.

**Проверка пути:**

```bash
# Правильно
curl -s "http://localhost:8080/api/v1/cameras/$CID/hls/sub/index.m3u8?token=$TOKEN" | head
```

Признак субпотока передаётся **в пути** (`/hls/sub/`), а не в query.
Ссылка вида `?stream=sub` поддерживается только для совместимости.

**Проверьте, что субпоток настроен и доступен:**

```bash
docker compose exec -T postgres psql -U nvr -d nvr -c \
  "SELECT name, main_stream, sub_stream FROM cameras WHERE id='$CID';"

curl -s "http://localhost:9997/v3/paths/get/${CID}_sub" | jq '.ready'
```

Если `ready=false` — камера не отдаёт дополнительный поток. Проверьте:

```bash
ffprobe -v error -rtsp_transport tcp -timeout 5000000 \
  -i "rtsp://root:ПАРОЛЬ@192.168.1.10/stream=1" \
  -show_entries stream=codec_name,width,height -of csv=p=0
```

---

## PTZ не работает

**Проверка поддержки ONVIF:**

```bash
IP=192.168.1.8
curl -s -u root:ПАРОЛЬ "http://$IP/onvif/device_service" \
  -H 'Content-Type: application/soap+xml; action="http://www.onvif.org/ver10/device/wsdl/GetCapabilities"' \
  -d '<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GetCapabilities xmlns="http://www.onvif.org/ver10/device/wsdl"><Category>All</Category></GetCapabilities></s:Body></s:Envelope>' \
  | grep -o '<tt:XAddr>[^<]*ptz[^<]*</tt:XAddr>'
```

Если адрес PTZ есть — камера поддерживается.

**Через API:**

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/cameras/$CID/ptz/status" | jq
```

`supports_ptz: false` означает, что ONVIF-сервис не ответил: проверьте
учётные данные и включён ли флаг `ptz` у камеры в интерфейсе.

---

## Команды SSH не выполняются

**Симптом:** кнопки «Перезапустить стример» и «Перезагрузить камеру»
возвращают ошибку.

**Проверка из контейнера:**

```bash
docker compose exec backend sh -c \
  'sshpass -p "ПАРОЛЬ" ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null root@192.168.1.8 "echo OK"'
```

**Если `sshpass: not found`** — пересоберите образ:

```bash
docker compose build backend && docker compose up -d backend
```

**Если `Permission denied`** — неверные учётные данные. Камеры OpenIPC
используют те же логин и пароль, что и для RTSP.

**Если камера не OpenIPC** — команда `/etc/init.d/S95majestic` может
отсутствовать. Проверьте:

```bash
docker compose exec backend ssh ... root@IP "ls /etc/init.d/ | grep -i maj"
```

---

## Архив не работает

**Симптом:** ссылки на записи не открываются или пустой список.

**Проверка MinIO:**

```bash
curl -s -o /dev/null -w "health: %{http_code}\n" http://localhost:9000/minio/health/live
docker compose logs backend | grep -i minio
```

**Если в логах `minio init failed`:**

- Проверьте, что контейнер запущен: `docker compose ps minio`
- Убедитесь, что в `.env` указан `MINIO_ENDPOINT=localhost:9000`
  (бэкенд в host-сети, имя `minio` не резолвится)

**Если ссылки ведут на localhost и не открываются с другого компьютера:**

Задайте внешний адрес сервера:

```bash
# В .env
MINIO_PUBLIC_ENDPOINT=192.168.1.111:9000
```

Перезапустите бэкенд. Подпись ссылок зависит от хоста, поэтому это важно.

**Если записей нет вообще:** видеозапись не включена. Включите её в
конфигурации пути медиасервера:

```bash
curl -X PATCH "http://localhost:9997/v3/config/paths/patch/<uuid>" \
  -H 'Content-Type: application/json' \
  -d '{"record":true,"recordFormat":"fmp4","recordSegmentDuration":"1h"}'
```

---

## Ошибка сборки: requires go >= 1.25

**Симптом:**

```
go: go.mod requires go >= 1.25.0 (running go 1.22.12; GOTOOLCHAIN=local)
```

**Причина:** команды `go get` и `go mod tidy` поднимают требование версии
Go, а образ построен на `golang:1.22-alpine`.

**Решение:** зафиксировать версию и понизить транзитивные зависимости:

```bash
cd backend
go mod edit -go=1.22 -toolchain=none
go get golang.org/x/crypto@v0.28.0 golang.org/x/net@v0.28.0 \
       golang.org/x/sys@v0.26.0 golang.org/x/text@v0.19.0 \
       golang.org/x/sync@v0.8.0 github.com/klauspost/compress@v1.17.9
go mod edit -go=1.22 -toolchain=none
head -4 go.mod     # должно быть go 1.22
```

После любых `go get` проверяйте версию в `go.mod`.

---

## Сервисы не поднимаются после перезагрузки

**Проверка:**

```bash
systemctl status nvr
systemctl is-enabled nvr
docker compose ps
```

**Если служба не установлена:**

```bash
sudo ./scripts/install-service.sh
```

**Если контейнеры не перезапускаются:**

```bash
docker inspect gigacode-backend-1 --format '{{.HostConfig.RestartPolicy.Name}}'
# Ожидается: unless-stopped
```

Если политика `no` — обновите `docker-compose.yml` и примените:

```bash
docker compose up -d --force-recreate
```

---

## Общие команды диагностики

```bash
# Состояние всех сервисов
./scripts/nvr.sh status

# Логи конкретного сервиса
docker compose logs -f backend
docker compose logs --since 5m mediamtx | grep -i error

# Пути медиасервера
curl -s http://localhost:9997/v3/paths/list | jq '.items[] | {name, ready, tracks}'

# Активные HLS-муксеры
curl -s http://localhost:9997/v3/hlsmuxers/list | jq '.items[] | {path, bytesSent}'

# Сводка по API
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/stats | jq

# Проверка БД
docker compose exec postgres psql -U nvr -d nvr -c "\dt"
docker compose exec postgres psql -U nvr -d nvr -c "SELECT * FROM schema_migrations;"
```

Проверка ресурсов:

```bash
docker stats --no-stream
df -h
nvidia-smi          # если используется GPU
```
