"""
NRV Backend — Точка входа FastAPI.
Запуск: uvicorn app.main:app --reload --host 0.0.0.0 --port 8000
"""
from contextlib import asynccontextmanager
from typing import AsyncGenerator

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.api.auth import router as auth_router
from app.api.cameras import router as cameras_router
from app.api.scanner import router as scanner_router
from app.api.users import router as users_router

# Опционально: детекция объектов (требует numpy, opencv-python)
try:
    from app.api.detections import router as detections_router
except Exception:
    detections_router = None
from app.core.config import get_settings
from app.core.database import create_tables
from app.core.redis import redis_client

settings = get_settings()


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncGenerator[None, None]:
    """Жизненный цикл приложения: запуск/остановка."""
    # Startup: создаём таблицы (для dev), проверяем Redis
    if settings.app_env == "development":
        await create_tables()

    await redis_client.ping()

    yield

    # Shutdown: закрываем соединения
    await redis_client.aclose()


app = FastAPI(
    title="NRV — Network Video Recorder API",
    description="Промышленный NVR для камер OpenIPC. RTSP + WebRTC + GPU Detection.",
    version="0.1.0",
    lifespan=lifespan,
    docs_url="/api/docs",
    redoc_url="/api/redoc",
    openapi_url="/api/openapi.json",
)

# CORS
app.add_middleware(
    CORSMiddleware,
    allow_origins=list(settings.cors_origins),
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Подключаем роутеры
app.include_router(auth_router, prefix="/api")
app.include_router(cameras_router, prefix="/api")
if detections_router is not None:
    app.include_router(detections_router, prefix="/api")
app.include_router(scanner_router, prefix="/api")
app.include_router(users_router, prefix="/api")


@app.get("/api/health")
async def health_check():
    """Проверка работоспособности."""
    return {
        "status": "ok",
        "app": settings.app_name,
        "version": "0.1.0",
        "env": settings.app_env,
    }


@app.get("/api/stats")
async def system_stats():
    """Системные метрики."""
    import os
    import shutil

    media_path = settings.media_path
    total, used, free = shutil.disk_usage(media_path)

    return {
        "disk": {
            "total_gb": round(total / (1024**3), 2),
            "used_gb": round(used / (1024**3), 2),
            "free_gb": round(free / (1024**3), 2),
            "percent_used": round(used / total * 100, 1),
        },
        "media_path": str(media_path),
    }
