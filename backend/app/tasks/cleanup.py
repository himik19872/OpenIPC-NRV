"""
Celery-таски для очистки старых записей и архивации.
"""
import logging
import os
import shutil
from datetime import datetime, timedelta, timezone
from pathlib import Path

from app.core.celery_app import celery_app
from app.core.config import get_settings

logger = logging.getLogger(__name__)
settings = get_settings()


@celery_app.task
def cleanup_old_recordings(days: int = 30) -> dict:
    """
    Удаляет записи старше N дней (из ФС и из БД).
    Запускается по расписанию (cron).
    """
    media_path = settings.media_path
    cutoff = datetime.now(timezone.utc) - timedelta(days=days)
    deleted_count = 0
    freed_bytes = 0

    for camera_dir in media_path.iterdir():
        if not camera_dir.is_dir():
            continue

        for date_dir in camera_dir.iterdir():
            if not date_dir.is_dir():
                continue

            for video_file in date_dir.iterdir():
                if video_file.is_file():
                    mtime = datetime.fromtimestamp(video_file.stat().st_mtime, tz=timezone.utc)
                    if mtime < cutoff:
                        freed_bytes += video_file.stat().st_size
                        video_file.unlink()
                        deleted_count += 1

            # Удаляем пустые директории
            try:
                if not any(date_dir.iterdir()):
                    date_dir.rmdir()
            except OSError:
                pass

        try:
            if not any(camera_dir.iterdir()):
                camera_dir.rmdir()
        except OSError:
            pass

    logger.info(
        "Cleanup completed: %d files deleted, %d bytes freed",
        deleted_count,
        freed_bytes,
    )

    return {
        "deleted_count": deleted_count,
        "freed_bytes": freed_bytes,
        "cutoff_date": cutoff.isoformat(),
    }