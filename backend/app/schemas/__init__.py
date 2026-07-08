"""Схемы для API."""
from app.schemas.camera import *
from app.schemas.detection import *
from app.schemas.user import *

__all__ = [
    # Camera schemas
    "CameraCreate",
    "CameraUpdate",
    "CameraOut",
    "CameraDetailOut",
    "CameraStatusOut",
    "RecordingOut",
    "RecordingStartRequest",
    "RecordingStopRequest",
    "EventOut",
    # Detection schemas
    "DetectionCreate",
    "DetectedObjectOut",
    "DetectionStats",
    "DetectionSummary",
    "DetectionHealth",
    # User schemas
    "UserCreate",
    "UserOut",
    "UserUpdate",
    "Token",
    "TokenRefresh",
    "TokenData",
]
