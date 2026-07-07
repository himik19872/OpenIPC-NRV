# NRV — Network Video Recorder

Промышленный NVR-сервер для камер OpenIPC.
Поддержка RTSP и WebRTC (через go2rtc).

## Архитектура

```
nrv/
├── backend/          # FastAPI + PostgreSQL + Redis + Celery
├── frontend/         # React + TypeScript + Vite + Ant Design
├── media/            # Хранилище видеозаписей
├── docs/             # Документация
├── docker-compose.yml       # Dev-окружение
└── docker-compose.prod.yml  # Production
```

## Быстрый старт

### 1. Клонирование и настройка

```bash
git clone <repo-url>
cd nrv

# Копируй .env и заполни своими значениями
cp backend/.env.example backend/.env
```

### 2. Запуск через Docker (рекомендуется)

```bash
docker compose up -d
```

Открой: http://localhost (фронтенд) или http://localhost:8000/api/docs (Swagger)

### 3. Локальный запуск (без Docker)

**Требования:** Python 3.12+, Node.js 20+, PostgreSQL 16, Redis 7, FFmpeg

```bash
# Бэкенд
cd backend
python -m venv venv && source venv/bin/activate
pip install -r requirements.txt
uvicorn app.main:app --reload --host 0.0.0.0 --port 8000

# Celery worker (в другом терминале)
cd backend
celery -A app.core.celery_app worker --loglevel=info

# Фронтенд
cd frontend
npm install
npm run dev
```

### 4. Создание первого пользователя

```bash
curl -X POST http://localhost:8000/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "email": "admin@nrv.local", "password": "changeme123", "full_name": "Admin"}'
```

Затем повысить до admin в БД:
```sql
UPDATE users SET role = 'admin' WHERE username = 'admin';
```

### 5. Добавление камеры OpenIPC

```bash
curl -X POST http://localhost:8000/api/cameras \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Камера 1",
    "rtsp_url": "rtsp://192.168.1.100:554/stream=0",
    "location": "Этаж 1, Вход",
    "manufacturer": "OpenIPC"
  }'
```

## API Endpoints

| Метод | Путь | Описание |
|-------|------|----------|
| POST | /api/auth/register | Регистрация |
| POST | /api/auth/login | Вход |
| POST | /api/auth/refresh | Обновление токена |
| GET | /api/auth/me | Профиль |
| GET | /api/cameras | Список камер |
| POST | /api/cameras | Добавить камеру |
| GET | /api/cameras/{id} | Детали камеры |
| PUT | /api/cameras/{id} | Обновить камеру |
| DELETE | /api/cameras/{id} | Удалить камеру |
| GET | /api/cameras/{id}/stream | RTSP поток (MJPEG) |
| GET | /api/cameras/{id}/recordings | Записи камеры |
| POST | /api/cameras/{id}/recordings/start | Начать запись |
| GET | /api/users | Список пользователей (admin) |
| GET | /api/health | Health check |
| GET | /api/stats | Системная статистика |

Полная документация: http://localhost:8000/api/docs

## go2rtc (WebRTC)

Для WebRTC-стриминга используйте go2rtc:

```bash
docker run -d --name go2rtc --network host alexxit/go2rtc
```

WebRTC URL камеры: `http://<go2rtc_ip>:1984/api/webrtc`

## Переменные окружения

См. `backend/.env.example` для полного списка.



поддержка мультиязычности (i18n)