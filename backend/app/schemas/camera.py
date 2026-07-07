"""
NRV Backend — Схемы камер и записей.
"""
import uuid
from datetime import datetime
from typing import Optional

from pydantic import BaseModel, Field


class CameraCreate(BaseModel):
    """Запрос на добавление камеры."""

    name: str = Field(..., min_length=1, max_length=128)
    description: str = ""
    rtsp_url: str = Field(..., max_length=512)
    webrtc_url: Optional[str] = Field(default=None, max_length=512)
    go2rtc_stream: Optional[str] = Field(default=None, max_length=256)
    manufacturer: str = "OpenIPC"
    model: str = ""
    firmware: str = ""
    latitude: Optional[float] = None
    longitude: Optional[float] = None
    location: str = ""
    # OpenIPC-специфичные поля
    openipc_user: str = "root"
    openipc_password: str = "12345"
    config: dict = Field(default_factory=dict)
    is_enabled: bool = True


class CameraUpdate(BaseModel):
    """Запрос на обновление камеры (все поля опциональны)."""

    name: Optional[str] = Field(default=None, min_length=1, max_length=128)
    description: Optional[str] = None
    rtsp_url: Optional[str] = Field(default=None, max_length=512)
    webrtc_url: Optional[str] = Field(default=None, max_length=512)
    go2rtc_stream: Optional[str] = Field(default=None, max_length=256)
    manufacturer: Optional[str] = None
    model: Optional[str] = None
    firmware: Optional[str] = None
    latitude: Optional[float] = None
    longitude: Optional[float] = None
    location: Optional[str] = None
    openipc_user: Optional[str] = None
    openipc_password: Optional[str] = None
    config: Optional[dict] = None
    is_enabled: Optional[bool] = None


class CameraOut(BaseModel):
    """Ответ с данными камеры."""

    id: uuid.UUID
    name: str
    description: str
    rtsp_url: str
    webrtc_url: Optional[str] = None
    go2rtc_stream: Optional[str] = None
    manufacturer: str
    model: str
    firmware: str
    latitude: Optional[float] = None
    longitude: Optional[float] = None
    location: str
    openipc_user: str = "root"
    config: dict
    is_online: bool
    is_recording: bool
    is_enabled: bool
    owner_id: Optional[uuid.UUID] = None
    created_at: datetime
    updated_at: datetime

    model_config = {"from_attributes": True}


class CameraDetailOut(CameraOut):
    """Расширенная информация о камере с OpenIPC-данными."""

    snapshot_base64: Optional[str] = None
    majestic_config: Optional[dict] = None
    mjpeg_url: Optional[str] = None
    hls_url: Optional[str] = None
    ws_video_url: Optional[str] = None


class CameraStatusOut(BaseModel):
    """Краткий статус камеры."""

    id: uuid.UUID
    name: str
    is_online: bool
    is_recording: bool


class RecordingOut(BaseModel):
    """Ответ с данными записи."""

    id: uuid.UUID
    camera_id: uuid.UUID
    file_path: str
    file_size: int
    duration: float
    format: str
    start_time: datetime
    end_time: Optional[datetime] = None
    status: str
    metadata_: dict = Field(alias="metadata_")
    created_at: datetime

    model_config = {"from_attributes": True}


class RecordingStartRequest(BaseModel):
    """Запрос на начало записи."""

    camera_id: uuid.UUID


class RecordingStopRequest(BaseModel):
    """Запрос на остановку записи."""

    recording_id: uuid.UUID


class EventOut(BaseModel):
    """Ответ с данными события."""

    id: uuid.UUID
    camera_id: uuid.UUID
    event_type: str
    severity: str
    title: str
    description: str
    snapshot_path: Optional[str] = None
    metadata_: dict = Field(alias="metadata_")
    created_at: datetime

    model_config = {"from_attributes": True}