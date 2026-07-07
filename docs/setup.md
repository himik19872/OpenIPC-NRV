# NRV — Документация

## Интеграция с камерами OpenIPC

Камеры OpenIPC отдают RTSP поток по умолчанию. Рекомендуемые настройки на камере:

### Прошивка OpenIPC
```bash
# Активировать RTSP сервер
/etc/init/S95majestic start

# RTSP URL по умолчанию:
# rtsp://<ip-камеры>:554/stream=0  — основной поток
# rtsp://<ip-камеры>:554/stream=1  — дополнительный поток (низкое качество)
```

### Настройка go2rtc (для WebRTC)
```yaml
streams:
  camera_1: rtsp://admin:password@192.168.1.100:554/stream=0
```

### Добавление камеры в NRV
```bash
curl -X POST http://localhost:8000/api/cameras \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Входная группа",
    "rtsp_url": "rtsp://192.168.1.100:554/stream=0",
    "webrtc_url": "http://localhost:1984/api/webrtc",
    "go2rtc_stream": "camera_1",
    "location": "Этаж 1, Главный вход",
    "manufacturer": "OpenIPC",
    "config": {
      "resolution": "1920x1080",
      "fps": 15,
      "bitrate": 4096
    }
  }'
```

## API Reference

Swagger UI: `http://localhost:8000/api/docs`
ReDoc: `http://localhost:8000/api/redoc`

## Системные требования

### Минимальные
- CPU: 4 ядра
- RAM: 4 GB
- Диск: 100 GB (зависит от количества камер и срока хранения)
- OS: Ubuntu 24.04 / Debian 12

### Рекомендуемые (10+ камер)
- CPU: 8+ ядер
- RAM: 16 GB
- Диск: 1 TB+ (SSD для БД, HDD для архива)
- GPU: NVIDIA (опционально, для AI-детекции)

## Мониторинг

```bash
# Celery мониторинг
celery -A app.core.celery_app flower --port=5555

# Prometheus метрики (опционально)
# Добавить prometheus_client в requirements.txt
```

## Резервное копирование

```bash
# Бэкап PostgreSQL
pg_dump -U nrv_user nrv_db > backup_$(date +%F).sql

# Бэкап видео (rsync)
rsync -avz /var/media/nrv/ backup-server:/backups/nrv/
```