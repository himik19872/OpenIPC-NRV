# NRV Backend — Celery таски для очистки старых записей

"""
Celery-таски для очистки старых записей и управления хранилищем.
"""

import logging
import os
from datetime import datetime, timezone
from pathlib import Path

from app.core.celery_app import celery_app
from app.core.config import get_settings

logger = logging.getLogger(__name__)
settings = get_settings()


@celery_app.task(name="app.tasks.cleanup.cleanup_old_recordings")
def cleanup_old_recordings_task(max_age_days: int = 30) -> dict:
    """
    Удаляет старые записи видео и очищает дисковое пространство.
    
    Args:
        max_age_days: Максимальный возраст записи в днях
        
    Returns:
        Статистика удаления
    """
    media_path = settings.media_path
    deleted_count = 0
    freed_bytes = 0
    
    # Проходим по всем камерам
    for camera_dir in media_path.iterdir():
        if not camera_dir.is_dir():
            continue
            
        # Проходим по датам
        for date_dir in camera_dir.iterdir():
            if not date_dir.is_dir():
                continue
                
            # Удаляем файлы старше max_age_days
            cutoff_time = datetime.now(timezone.utc).timestamp() - (max_age_days * 86400)
            
            for file_path in date_dir.glob("*.mp4"):
                if file_path.stat().st_mtime < cutoff_time:
                    try:
                        file_size = file_path.stat().st_size
                        file_path.unlink()
                        freed_bytes += file_size
                        deleted_count += 1
                        
                        # Удаляем превью если есть
                        thumb_path = file_path.with_suffix(".thumb.jpg")
                        if thumb_path.exists():
                            thumb_path.unlink()
                    except Exception as e:
                        logger.error(f"Ошибка удаления {file_path}: {e}")
    
    logger.info(f"Очищено {deleted_count} записей, освобождено {freed_bytes / (1024**3):.2f} ГБ")
    
    return {
        "status": "success",
        "deleted_count": deleted_count,
        "freed_bytes": freed_bytes
    }


@celery_app.task(name="app.tasks.cleanup.optimize_storage")
def optimize_storage_task() -> dict:
    """
    Оптимизация хранилища: сжатие старых записей, удаление дубликатов.
    
    Returns:
        Статистика оптимизации
    """
    media_path = settings.media_path
    compressed_count = 0
    deleted_duplicates = 0
    
    # Проходим по всем камерам
    for camera_dir in media_path.iterdir():
        if not camera_dir.is_dir():
            continue
            
        # Проходим по датам
        for date_dir in camera_dir.iterdir():
            if not date_dir.is_dir():
                continue
                
            # Ищем дубликаты (файлы с одинаковым именем)
            files_by_size = {}
            for file_path in date_dir.glob("*.mp4"):
                file_size = file_path.stat().st_size
                if file_size in files_by_size:
                    # Дубликат найден
                    try:
                        file_path.unlink()
                        deleted_duplicates += 1
                    except Exception as e:
                        logger.error(f"Ошибка удаления дубликата {file_path}: {e}")
                else:
                    files_by_size[file_size] = file_path
            
            # Сжатие старых файлов (older than 7 days)
            cutoff_time = datetime.now(timezone.utc).timestamp() - (7 * 86400)
            
            for file_path in date_dir.glob("*.mp4"):
                if file_path.stat().st_mtime < cutoff_time and not file_path.name.endswith(".gz"):
                    try:
                        # Сжатие через gzip
                        import gzip
                        import shutil
                        
                        with file_path.open('rb') as f_in:
                            with gzip.open(f"{file_path}.gz", 'wb') as f_out:
                                shutil.copyfileobj(f_in, f_out)
                        
                        file_path.unlink()
                        compressed_count += 1
                    except Exception as e:
                        logger.error(f"Ошибка сжатия {file_path}: {e}")
    
    logger.info(f"Оптимизация: {compressed_count} сжато, {deleted_duplicates} дубликатов удалено")
    
    return {
        "status": "success",
        "compressed_count": compressed_count,
        "deleted_duplicates": deleted_duplicates
    }
