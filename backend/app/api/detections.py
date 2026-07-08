# NRV Backend — Detection API: обнаружение объектов на видео

"""
Detection API: обнаружение объектов на видео с GPU ускорением.
"""

from typing import Annotated, List
from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Query, status
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user
from app.core.database import get_db
from app.models.camera import Camera, Recording
from app.models.detection import DetectedObject
from app.models.user import User
from app.schemas.camera import RecordingOut
from app.tasks.detection import detect_objects_in_recording, detect_objects_in_stream, detect_and_save

router = APIRouter(prefix="/detections", tags=["detections"])


# ---- Detection API Endpoints ----


@router.post("/start/recording", status_code=status.HTTP_201_CREATED)
async def start_detection_recording(
    recording_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Запуск детекции объектов в записанном видео.
    
    Args:
        recording_id: ID записи в БД
    """
    result = await db.execute(select(Recording).where(Recording.id == recording_id))
    recording = result.scalar_one_or_none()
    if not recording:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Recording not found"
        )
    
    # Запуск задачи в Celery
    task = detect_and_save.delay(
        recording_id=str(recording_id),
        file_path=recording.file_path,
        camera_id=str(recording.camera_id)
    )
    
    return {
        "status": "started",
        "task_id": task.id,
        "recording_id": str(recording_id)
    }


@router.post("/start/stream")
async def start_detection_stream(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    duration_seconds: int = Query(default=60, ge=10, le=300),
):
    """
    Запуск детекции объектов в реальном времени из RTSP стрима.
    
    Args:
        camera_id: ID камеры
        duration_seconds: Длительность анализа (по умолчанию 60 сек)
    """
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Camera not found"
        )
    
    # Запуск задачи в Celery
    task = detect_objects_in_stream.delay(
        camera_id=str(camera_id),
        rtsp_url=camera.rtsp_main_url,
        duration_seconds=duration_seconds
    )
    
    return {
        "status": "started",
        "task_id": task.id,
        "camera_id": str(camera_id),
        "duration_seconds": duration_seconds
    }


@router.get("/results/{recording_id}", response_model=List[DetectedObject])
async def get_detection_results(
    recording_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    class_name: str | None = Query(default=None, description="Фильтр по классу"),
    min_confidence: float = Query(default=0.5, ge=0.0, le=1.0),
    skip: int = Query(default=0, ge=0),
    limit: int = Query(default=100, ge=1, le=500),
):
    """
    Получение результатов детекции для записи.
    
    Args:
        recording_id: ID записи
        class_name: Фильтр по классу объекта
        min_confidence: Минимальная уверенность
        skip: Пропуск записей
        limit: Лимит результатов
    """
    from sqlalchemy import and_
    
    stmt = select(DetectedObject).where(DetectedObject.recording_id == recording_id)
    
    if class_name:
        stmt = stmt.where(DetectedObject.class_name == class_name)
    
    stmt = stmt.where(DetectedObject.confidence >= min_confidence)
    stmt = stmt.offset(skip).limit(limit).order_by(DetectedObject.confidence.desc())
    
    result = await db.execute(stmt)
    detections = result.scalars().all()
    
    return [DetectedObject.model_validate(d) for d in detections]


@router.get("/stream/{camera_id}")
async def get_stream_detections(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    skip: int = Query(default=0, ge=0),
    limit: int = Query(default=50, ge=1, le=200),
):
    """
    Получение последних детекций для стрима камеры.
    
    Args:
        camera_id: ID камеры
        skip: Пропуск записей
        limit: Лимит результатов
    """
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Camera not found"
        )
    
    stmt = select(DetectedObject).where(DetectedObject.camera_id == camera_id)
    stmt = stmt.offset(skip).limit(limit).order_by(DetectedObject.timestamp.desc())
    
    result = await db.execute(stmt)
    detections = result.scalars().all()
    
    return {
        "camera_id": str(camera_id),
        "camera_name": camera.name,
        "detections": [DetectedObject.model_validate(d) for d in detections]
    }


@router.get("/stats/{camera_id}")
async def get_detection_stats(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    hours: int = Query(default=24, ge=1, le=720),
):
    """
    Статистика детекций за период.
    
    Args:
        camera_id: ID камеры
        hours: Период в часах
    """
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Camera not found"
        )
    
    from datetime import datetime, timedelta, timezone
    from sqlalchemy import func, text
    
    cutoff_time = datetime.now(timezone.utc) - timedelta(hours=hours)
    
    # Подсчет по классам
    stmt = text("""
        SELECT class_name, COUNT(*) as count, AVG(confidence) as avg_confidence
        FROM detected_objects
        WHERE camera_id = :camera_id
        AND timestamp >= :cutoff
        GROUP BY class_name
        ORDER BY count DESC
    """)
    
    result = await db.execute(stmt, {"camera_id": str(camera_id), "cutoff": cutoff_time})
    stats = result.fetchall()
    
    return {
        "camera_id": str(camera_id),
        "camera_name": camera.name,
        "period_hours": hours,
        "class_stats": [
            {
                "class": row.class_name,
                "count": row.count,
                "avg_confidence": float(row.avg_confidence) if row.avg_confidence else 0.0
            }
            for row in stats
        ]
    }


@router.get("/summary")
async def get_detection_summary(
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    days: int = Query(default=7, ge=1, le=30),
):
    """
    Сводка по всем детекциям за период.
    
    Args:
        days: Период в днях
    """
    from datetime import datetime, timedelta, timezone
    from sqlalchemy import func, text
    
    cutoff_time = datetime.now(timezone.utc) - timedelta(days=days)
    
    stmt = text("""
        SELECT 
            c.name as camera_name,
            c.location,
            d.class_name,
            COUNT(*) as total_count,
            AVG(d.confidence) as avg_confidence,
            MAX(d.timestamp) as last_detection
        FROM detected_objects d
        JOIN cameras c ON d.camera_id = c.id
        WHERE d.timestamp >= :cutoff
        GROUP BY c.name, c.location, d.class_name
        ORDER BY total_count DESC
        LIMIT 100
    """)
    
    result = await db.execute(stmt, {"cutoff": cutoff_time})
    summaries = result.fetchall()
    
    return {
        "period_days": days,
        "summary": [
            {
                "camera_name": row.camera_name,
                "location": row.location,
                "class": row.class_name,
                "count": row.total_count,
                "avg_confidence": float(row.avg_confidence) if row.avg_confidence else 0.0,
                "last_detection": row.last_detection.isoformat() if row.last_detection else None
            }
            for row in summaries
        ]
    }


@router.get("/health")
async def detection_health():
    """
    Проверка состояния системы детекции.
    """
    try:
        detector = __import__('app.services.detection', fromlist=['ObjectDetector']).ObjectDetector()
        device = detector.device
        
        return {
            "status": "ok",
            "device": device,
            "model_loaded": detector.model is not None
        }
    except Exception as e:
        return {
            "status": "error",
            "error": str(e)
        }
