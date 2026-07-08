# NRV Backend — Celery таски для детекции объектов

"""
Celery-таски для обнаружения объектов на видео с GPU ускорением.
"""

import logging
import os
import uuid
from datetime import datetime, timezone
from pathlib import Path

from app.core.celery_app import celery_app
from app.core.config import get_settings
from app.models.camera import Recording
from app.models.detection import DetectedObject
from app.services.detection import detect_objects_from_file, detect_objects_from_rtsp, ObjectDetector

logger = logging.getLogger(__name__)
settings = get_settings()


@celery_app.task(bind=True, max_retries=3)
def detect_objects_in_recording(self, recording_id: str, file_path: str) -> dict:
    """
    Детекция объектов в записанном видеофайле.
    
    Args:
        recording_id: ID записи в БД
        file_path: Путь к видеофайлу
        
    Returns:
        Словарь с результатами детекции
    """
    try:
        # Инициализация детектора
        detector = ObjectDetector()
        detector.load_model()
        
        logger.info(f"Начало детекции в файле: {file_path}")
        
        # Детекция
        detections = detect_objects_from_file(file_path)
        
        # Сохранение в БД
        from sqlalchemy import create_engine, text
        from sqlalchemy.orm import sessionmaker
        
        sync_url = settings.database_url.replace("+asyncpg", "")
        engine = create_engine(sync_url)
        Session = sessionmaker(bind=engine)
        
        session = Session()
        try:
            for det in detections:
                obj = DetectedObject(
                    recording_id=uuid.UUID(recording_id),
                    camera_id=uuid.UUID("00000000-0000-0000-0000-000000000000"),  # Заполнить при наличии
                    class_name=det.class_name,
                    confidence=det.confidence,
                    bbox_x=det.bbox[0],
                    bbox_y=det.bbox[1],
                    bbox_w=det.bbox[2],
                    bbox_h=det.bbox[3],
                    timestamp=det.timestamp or datetime.now(timezone.utc).isoformat()
                )
                session.add(obj)
            
            session.commit()
            logger.info(f"Сохранено {len(detections)} объектов в БД")
            
        finally:
            session.close()
            engine.dispose()
        
        return {
            "status": "success",
            "recording_id": recording_id,
            "file_path": file_path,
            "total_detections": len(detections),
            "objects": [
                {"class": d.class_name, "confidence": d.confidence}
                for d in detections[:10]  # Первые 10 объектов
            ]
        }
        
    except Exception as exc:
        logger.error(f"Ошибка детекции в записи {recording_id}: {exc}")
        raise self.retry(exc=exc, countdown=30)


@celery_app.task(bind=True)
def detect_objects_in_stream(self, camera_id: str, rtsp_url: str, duration_seconds: int = 60) -> dict:
    """
    Детекция объектов в реальном времени из RTSP стрима.
    
    Args:
        camera_id: ID камеры
        rtsp_url: URL RTSP стрима
        duration_seconds: Длительность анализа в секундах
        
    Returns:
        Словарь с результатами детекции
    """
    try:
        # Оценка количества кадров (5 FPS)
        max_frames = duration_seconds * 5
        
        # Детекция
        detections = detect_objects_from_rtsp(rtsp_url, max_frames=max_frames)
        
        # Группировка по классам
        class_counts = {}
        for det in detections:
            class_counts[det.class_name] = class_counts.get(det.class_name, 0) + 1
        
        logger.info(f"Детекция из стрима {camera_id}: {class_counts}")
        
        return {
            "status": "success",
            "camera_id": camera_id,
            "duration_seconds": duration_seconds,
            "total_detections": len(detections),
            "class_counts": class_counts
        }
        
    except Exception as exc:
        logger.error(f"Ошибка детекции в стриме {camera_id}: {exc}")
        return {
            "status": "error",
            "camera_id": camera_id,
            "error": str(exc)
        }


@celery_app.task(name="app.tasks.detection.check_detections_periodic")
def check_detections_periodic() -> dict:
    """
    Периодическая задача для проверки и запуска детекции.
    Обрабатывает новую запись видео.
    """
    from sqlalchemy import create_engine, text
    
    sync_url = settings.database_url.replace("+asyncpg", "")
    engine = create_engine(sync_url)
    
    try:
        with engine.connect() as conn:
            # Найти непроанализированные записи
            result = conn.execute(text("""
                SELECT id, file_path, camera_id 
                FROM recordings 
                WHERE status = 'completed' 
                AND id NOT IN (
                    SELECT recording_id FROM detected_objects
                )
                ORDER BY start_time DESC 
                LIMIT 5
            """))
            
            recordings = result.fetchall()
            
            processed = 0
            for rec in recordings:
                rec_id, file_path, cam_id = rec
                
                # Запуск детекции
                detect_objects_in_recording.delay(str(rec_id), file_path)
                processed += 1
            
            logger.info(f"Запущено {processed} задач детекции")
            
            return {
                "status": "ok",
                "processed": processed
            }
            
    finally:
        engine.dispose()


@celery_app.task(bind=True)
def detect_and_save(self, recording_id: str, file_path: str, camera_id: str) -> dict:
    """
    Упрощенная задача детекции с автоматическим сохранением.
    
    Args:
        recording_id: ID записи
        file_path: Путь к файлу
        camera_id: ID камеры
        
    Returns:
        Результат детекции
    """
    try:
        # Инициализация детектора
        detector = ObjectDetector()
        detector.load_model()
        
        # Детекция
        detections = detect_objects_from_file(file_path)
        
        # Сохранение в БД
        from sqlalchemy import create_engine, text
        from sqlalchemy.orm import sessionmaker
        import uuid as uuid_module
        
        sync_url = settings.database_url.replace("+asyncpg", "")
        engine = create_engine(sync_url)
        Session = sessionmaker(bind=engine)
        
        session = Session()
        try:
            for det in detections:
                obj = DetectedObject(
                    recording_id=uuid_module.UUID(recording_id),
                    camera_id=uuid_module.UUID(camera_id),
                    class_name=det.class_name,
                    confidence=det.confidence,
                    bbox_x=det.bbox[0],
                    bbox_y=det.bbox[1],
                    bbox_w=det.bbox[2],
                    bbox_h=det.bbox[3],
                    timestamp=datetime.now(timezone.utc).isoformat()
                )
                session.add(obj)
            
            session.commit()
            
        finally:
            session.close()
            engine.dispose()
        
        # Подсчет по классам
        class_counts = {}
        for det in detections:
            class_counts[det.class_name] = class_counts.get(det.class_name, 0) + 1
        
        return {
            "status": "success",
            "recording_id": recording_id,
            "total_detections": len(detections),
            "class_counts": class_counts
        }
        
    except Exception as exc:
        logger.error(f"Ошибка детекции и сохранения: {exc}")
        raise self.retry(exc=exc, countdown=30)
