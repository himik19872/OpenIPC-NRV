# NRV Backend — API: инициализация
"""Инициализация API модуля."""

from app.api.auth import router as auth_router
from app.api.cameras import router as cameras_router
from app.api.scanner import router as scanner_router
from app.api.users import router as users_router

# Опционально: детекция объектов (требует numpy, opencv-python)
try:
    from app.api.detections import router as detections_router
except Exception:
    detections_router = None  # type: ignore

__all__ = [
    "auth_router",
    "cameras_router",
    "detections_router",
    "scanner_router",
    "users_router",
]
