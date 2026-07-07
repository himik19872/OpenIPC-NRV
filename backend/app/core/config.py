"""
NRV Backend — Конфигурация приложения.
Загружает настройки из .env и переменных окружения.
"""
from functools import lru_cache
from pathlib import Path
from typing import List

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Настройки приложения, загружаемые из .env."""

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        case_sensitive=False,
        extra="ignore",
    )

    # Application
    app_name: str = "NRV"
    app_env: str = "development"
    app_debug: bool = True
    secret_key: str = "change-me"

    # Database
    database_url: str = "postgresql+asyncpg://nrv_user:nrv_password@localhost:5432/nrv_db"

    # Redis
    redis_url: str = "redis://localhost:6379/0"

    # JWT
    jwt_secret_key: str = "change-me"
    jwt_algorithm: str = "HS256"
    access_token_expire_minutes: int = 30
    refresh_token_expire_days: int = 7

    # Media Storage
    media_root: str = "../media/recordings"

    # RTSP / go2rtc
    go2rtc_api_url: str = "http://localhost:1984/api"

    # CORS
    cors_origins: List[str] = ["http://localhost:5173", "http://localhost:3000"]

    # Celery
    celery_broker_url: str = "redis://localhost:6379/1"
    celery_result_backend: str = "redis://localhost:6379/2"

    @property
    def media_path(self) -> Path:
        """Абсолютный путь к директории с медиафайлами."""
        path = Path(self.media_root)
        if not path.is_absolute():
            path = Path(__file__).parent.parent.parent / path
        path.mkdir(parents=True, exist_ok=True)
        return path


@lru_cache()
def get_settings() -> Settings:
    """Возвращает закэшированный объект настроек."""
    return Settings()