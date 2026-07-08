"""
NRV Backend — Модель камеры.
"""
import uuid
from datetime import datetime, timezone

from sqlalchemy import Boolean, DateTime, Float, ForeignKey, Integer, String, Text
from sqlalchemy.dialects.postgresql import JSONB, UUID
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.core.database import Base


class Camera(Base):
    """Камера видеонаблюдения (OpenIPC / RTSP / WebRTC)."""

    __tablename__ = "cameras"

    id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True), primary_key=True, default=uuid.uuid4
    )
    name: Mapped[str] = mapped_column(String(128), nullable=False)
    description: Mapped[str] = mapped_column(Text, default="")

    # RTSP-потоки: основной (высокое качество) и дополнительный (низкое)
    rtsp_main_url: Mapped[str] = mapped_column(String(512), nullable=False)
    rtsp_sub_url: Mapped[str | None] = mapped_column(String(512), nullable=True, default=None)
    # URL прокси-потоков через go2rtc (для внешних потребителей)
    rtsp_proxy_main_url: Mapped[str | None] = mapped_column(String(512), nullable=True, default=None)
    rtsp_proxy_sub_url: Mapped[str | None] = mapped_column(String(512), nullable=True, default=None)

    # WebRTC через go2rtc
    webrtc_url: Mapped[str | None] = mapped_column(String(512), nullable=True, default=None)
    go2rtc_stream: Mapped[str | None] = mapped_column(
        String(256), nullable=True, default=None
    )  # имя стрима в go2rtc

    # Мета-данные
    manufacturer: Mapped[str] = mapped_column(String(64), default="OpenIPC")
    model: Mapped[str] = mapped_column(String(64), default="")
    firmware: Mapped[str] = mapped_column(String(32), default="")
    openipc_user: Mapped[str] = mapped_column(String(64), default="root")
    openipc_password: Mapped[str] = mapped_column(String(128), default="12345")

    # Геолокация
    latitude: Mapped[float | None] = mapped_column(Float, nullable=True, default=None)
    longitude: Mapped[float | None] = mapped_column(Float, nullable=True, default=None)
    location: Mapped[str] = mapped_column(String(256), default="")

    # Настройки
    config: Mapped[dict] = mapped_column(JSONB, default=dict)

    # Статус
    is_online: Mapped[bool] = mapped_column(Boolean, default=False)
    is_recording: Mapped[bool] = mapped_column(Boolean, default=False)
    is_enabled: Mapped[bool] = mapped_column(Boolean, default=True)

    # Владелец (опционально)
    owner_id: Mapped[uuid.UUID | None] = mapped_column(
        UUID(as_uuid=True), ForeignKey("users.id", ondelete="SET NULL"), nullable=True
    )

    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=lambda: datetime.now(timezone.utc)
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True),
        default=lambda: datetime.now(timezone.utc),
        onupdate=lambda: datetime.now(timezone.utc),
    )

    # Связи
    recordings: Mapped[list["Recording"]] = relationship(
        "Recording", back_populates="camera", cascade="all, delete-orphan"
    )

    def __repr__(self) -> str:
        return f"<Camera {self.name}>"


class Recording(Base):
    """Запись видео с камеры."""

    __tablename__ = "recordings"

    id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True), primary_key=True, default=uuid.uuid4
    )
    camera_id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True), ForeignKey("cameras.id", ondelete="CASCADE"), nullable=False, index=True
    )

    file_path: Mapped[str] = mapped_column(String(1024), nullable=False)
    file_size: Mapped[int] = mapped_column(Integer, default=0)  # байты
    duration: Mapped[float] = mapped_column(Float, default=0.0)  # секунды
    format: Mapped[str] = mapped_column(String(16), default="mp4")

    # Временные метки
    start_time: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, index=True
    )
    end_time: Mapped[datetime | None] = mapped_column(
        DateTime(timezone=True), nullable=True, default=None
    )

    # Статус
    status: Mapped[str] = mapped_column(
        String(32), default="recording"
    )  # recording, completed, failed, archived

    # Мета-данные
    metadata_: Mapped[dict] = mapped_column("metadata", JSONB, default=dict)

    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=lambda: datetime.now(timezone.utc)
    )

    # Связи
    camera: Mapped["Camera"] = relationship("Camera", back_populates="recordings")

    def __repr__(self) -> str:
        return f"<Recording {self.id} ({self.status})>"