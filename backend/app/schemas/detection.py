"""Схемы для детекции объектов."""
import uuid
from datetime import datetime
from typing import Optional

from pydantic import BaseModel, Field


class DetectionCreate(BaseModel):
    """Запрос на начало детекции."""

    recording_id: Optional[uuid.UUID] = None
    camera_id: Optional[uuid.UUID] = None
    rtsp_url: Optional[str] = Field(default=None, max_length=512)
    duration_seconds: int = Field(default=60, ge=10, le=300)


class DetectedObjectOut(BaseModel):
    """Ответ с данными обнаруженного объекта."""

    id: uuid.UUID
    recording_id: Optional[uuid.UUID] = None
    camera_id: uuid.UUID
    class_name: str
    confidence: float
    bbox_x: int
    bbox_y: int
    bbox_w: int
    bbox_h: int
    timestamp: str
    created_at: datetime

    model_config = {"from_attributes": True}


class DetectionStats(BaseModel):
    """Статистика детекций."""

    camera_id: uuid.UUID
    camera_name: str
    period_hours: int
    class_stats: list[dict]


class DetectionSummary(BaseModel):
    """Сводка по детекциям."""

    period_days: int
    summary: list[dict]


class DetectionHealth(BaseModel):
    """Состояние системы детекции."""

    status: str
    device: str
    model_loaded: bool
