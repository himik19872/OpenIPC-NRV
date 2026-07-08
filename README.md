# NRV — Network Video Recorder

Промышленный NVR-сервер для камер OpenIPC.
Поддержка RTSP, WebRTC (go2rtc) и H.264 стриминг без перекодирования.

## Возможности

- **Без перекодирования:** `ffmpeg -c copy` ремуксинг RTSP→fMP4, нативный H.264 в браузере
- **Два потока:** основной (высокое качество) + дополнительный (низкое, для сетки LiveView)
- **RTSP-прокси (go2rtc):** камера отдаёт поток один раз, потребители забирают с прокси
- **LiveView:** сетка до 5×5, переключение sub→main при развороте на fullscreen
- **Автообнаружение:** сетевой сканер камер OpenIPC (Majestic API)
- **Веб-интерфейс:** React + Ant Design, добавление/редактирование камер
- **REST API:** FastAPI + Swagger-документация
- **Фоновая запись:** Celery + ffmpeg
- **Мобильное приложение:** React Native (Expo)

## Архитектура

```
nvr (192.168.1.107)
├── nrv-backend     :8000   FastAPI + SQLAlchemy + Celery
├── nrv-frontend    :5173   Vite + React + Ant Design
├── nrv-go2rtc      :1984   go2rtc (RTSP/WebRTC/MSE прокси)
├── postgresql      :5432   База данных
├── redis           :6379   Кэш + брокер Celery
└── media/                  Видеозаписи
```

## Быстрый старт

### Требования

- Python 3.12+, Node.js 20+, PostgreSQL 16, Redis 7, FFmpeg
- go2rtc (опционально, для WebRTC)

### 1. Установка

```bash
git clone <repo-url>
cd nrv

# Виртуальное окружение Python
python -m venv .venv && source .venv/bin/activate
pip install -r backend/requirements.txt
pip install bcrypt==3.2.2  # совместимость с passlib

# Node.js зависимости
cd frontend && npm install && cd ..

# БД (если не создана)
sudo -u postgres createuser nrv_user
sudo -u postgres createdb nrv_db -O nrv_user

# Миграции
cd backend && alembic upgrade head && cd ..
```

### 2. Запуск

```bash
# Бэкенд
cd backend && uvicorn app.main:app --host 0.0.0.0 --port 8000 &

# Фронтенд
cd frontend && npm run dev &

# go2rtc (опционально)
go2rtc -c go2rtc.yaml &
```

Открой: **http://localhost:5173** (или `http://<ip-сервера>:5173` из сети)

### 3. Автозапуск (systemd)

```bash
sudo bash scripts/install-services.sh
```

Сервисы запускаются автоматически при загрузке системы.

Управление:
```bash
sudo systemctl start|stop|restart nrv-backend
sudo systemctl start|stop|restart nrv-frontend
sudo systemctl start|stop|restart nrv-go2rtc
sudo journalctl -u nrv-backend -f   # логи
```

### 4. Создание пользователя

```bash
curl -X POST http://localhost:8000/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "email": "admin@nrv.local", "password": "changeme123", "full_name": "Admin"}'
```

Повысить до admin:
```sql
psql -U nrv_user -d nrv_db -c "UPDATE users SET role = 'admin' WHERE username = 'admin';"
```

## API

Swagger: **http://localhost:8000/api/docs**

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/api/auth/register` | Регистрация |
| POST | `/api/auth/login` | Вход (JWT) |
| GET  | `/api/cameras` | Список камер |
| POST | `/api/cameras` | Добавить камеру |
| PUT  | `/api/cameras/{id}` | Обновить камеру |
| DELETE | `/api/cameras/{id}` | Удалить камеру |
| GET  | `/api/cameras/{id}/stream/mse?stream=0` | H.264 основной поток |
| GET  | `/api/cameras/{id}/stream/mse?stream=1` | H.264 доп. поток |
| GET  | `/api/cameras/{id}/proxy` | Прокси-RTSP URL |
| GET  | `/api/cameras/{id}/recordings` | Записи |
| POST | `/api/scanner/scan` | Сканирование сети |

## Камера OpenIPC

RTSP URL по умолчанию:
- Основной поток: `rtsp://root:12345@192.168.1.x/stream=0`
- Доп. поток:   `rtsp://root:12345@192.168.1.x/stream=1`

## Структура проекта

```
nrv/
├── backend/               # FastAPI
│   ├── app/
│   │   ├── api/           # Роутеры (auth, cameras, scanner, users)
│   │   ├── core/          # Конфигурация, БД, Redis, JWT
│   │   ├── models/        # SQLAlchemy модели (Camera, User, Event, Recording)
│   │   ├── schemas/       # Pydantic схемы
│   │   ├── services/      # Бизнес-логика (stream, rtsp_proxy, scanner, go2rtc)
│   │   └── tasks/         # Celery задачи (запись, очистка)
│   ├── alembic/           # Миграции БД
│   └── requirements.txt
├── frontend/              # React + Vite
│   └── src/pages/         # Страницы (Dashboard, LiveView, Cameras, ...)
├── mobile/                # React Native (Expo)
├── scripts/               # systemd-сервисы и скрипты установки
├── go2rtc.yaml            # Конфигурация go2rtc
├── nginx.conf             # Nginx (production)
├── docker-compose.yml     # Dev
└── docker-compose.prod.yml
```

## Лицензия

MIT
