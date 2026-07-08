# NRV Backend — Модель детекции объектов

"""
Модель хранения результатов детекции объектов в PostgreSQL.
"""

import uuid
from datetime import datetime, timezone
from typing import Tuple

from sqlalchemy import DateTime, Float, Integer, String, UUID, ForeignKey
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.core.database import Base


class DetectedObject(Base):
    """Обнаруженный объект в записи или стриме."""

    __tablename__ = "detected_objects"

    id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True), primary_key=True, default=uuid.uuid4
    )
    
    recording_id: Mapped[uuid.UUID | None] = mapped_column(
        UUID(as_uuid=True), ForeignKey("recordings.id", ondelete="SET NULL"), 
        nullable=True, index=True
    )
    
    camera_id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True), ForeignKey("cameras.id", ondelete="CASCADE"), 
        nullable=False, index=True
    )
    
    # Class and confidence
    class_name: Mapped[str] = mapped_column(String(64), nullable=False, index=True)
    confidence: Mapped[float] = mapped_column(Float, nullable=False)
    
    # Bounding box (x, y, width, height)
    bbox_x: Mapped[int] = mapped_column(Integer, nullable=False)
    bbox_y: Mapped[int] = mapped_column(Integer, nullable=False)
    bbox_w: Mapped[int] = mapped_column(Integer, nullable=False)
    bbox_h: Mapped[int] = mapped_column(Integer, nullable=False)
    
    # Timestamp
    timestamp: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, index=True
    )
    
    # Metadata (optional)
    metadata_: Mapped[dict] = mapped_column("metadata", JSONB, default=dict)
    
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=lambda: datetime.now(timezone.utc)
    )
    
    # Relationships
    recording: Mapped["Recording"] = relationship("Recording", backref="detections")
    camera: Mapped["Camera"] = relationship("Camera", backref="detections")
    
    def __repr__(self) -> str:
        return f"<DetectedObject {self.class_name} ({self.confidence:.2f})>"
    
    @property
    def bbox(self) -> Tuple[int, int, int, int]:
        """Возвращает bbox как кортеж."""
        return (self.bbox_x, self.bbox_y, self.bbox_w, self.bbox_h)


class DetectionSummary(Base):
    """Сводка по детекциям за период."""

    __tablename__ = "detection_summaries"

    id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True), primary_key=True, default=uuid.uuid4
    )
    
    camera_id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True), ForeignKey("cameras.id", ondelete="CASCADE"), 
        nullable=False, index=True
    )
    
    recording_id: Mapped[uuid.UUID | None] = mapped_column(
        UUID(as_uuid=True), ForeignKey("recordings.id", ondelete="CASCADE"), 
        nullable=True, index=True
    )
    
    # Summary data
    class_name: Mapped[str] = mapped_column(String(64), nullable=False, index=True)
    total_count: Mapped[int] = mapped_column(Integer, default=0)
    avg_confidence: Mapped[float] = mapped_column(Float, default=0.0)
    
    # Time range
    start_time: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, index=True
    )
    end_time: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, index=True
    )
    
    # Location (from camera)
    location: Mapped[str] = mapped_column(String(256), default="")
    
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=lambda: datetime.now(timezone.utc)
    )
    
    def __repr__(self) -> str:
        return f"<DetectionSummary {self.class_name}: {self.total_count} objects>"
