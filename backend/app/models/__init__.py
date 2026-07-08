"""Модели SQLAlchemy для NRV."""
from app.models.camera import Camera, Recording
from app.models.detection import DetectedObject, DetectionSummary
from app.models.event import Event
from app.models.user import User

__all__ = [
    "Camera",
    "Recording",
    "DetectedObject",
    "DetectionSummary",
    "Event",
    "User",
]
