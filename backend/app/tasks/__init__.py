# NRV Backend — Tasks: инициализация

"""Инициализация tasks модуля."""

from app.tasks.cleanup import cleanup_old_recordings_task
from app.tasks.detection import (
    detect_and_save,
    detect_objects_in_recording,
    detect_objects_in_stream,
    check_detections_periodic,
)
from app.tasks.video import (
    check_cameras_online,
    generate_thumbnail_task,
    start_recording_task,
    stop_recording_task,
)

__all__ = [
    # Cleanup
    "cleanup_old_recordings_task",
    # Detection
    "detect_and_save",
    "detect_objects_in_recording",
    "detect_objects_in_stream",
    "check_detections_periodic",
    # Video
    "check_cameras_online",
    "generate_thumbnail_task",
    "start_recording_task",
    "stop_recording_task",
]
