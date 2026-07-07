"""
Celery-таски для видео-обработки: запись, транскодирование, превью.
"""
import asyncio
import logging
import os
import subprocess
import uuid
from datetime import datetime, timezone
from pathlib import Path

from app.core.celery_app import celery_app
from app.core.config import get_settings

logger = logging.getLogger(__name__)
settings = get_settings()

# Словарь для отслеживания активных ffmpeg-процессов по recording_id
_active_recordings: dict[str, subprocess.Popen] = {}


@celery_app.task(bind=True, max_retries=3)
def start_recording_task(self, recording_id: str, camera_id: str, rtsp_url: str) -> dict:
    """
    Запускает запись RTSP-потока через ffmpeg в фоне.
    Пишет видео в файл: media/recordings/{camera_id}/{date}/{recording_id}.mp4
    """
    media_path = settings.media_path
    date_str = datetime.now(timezone.utc).strftime("%Y-%m-%d")
    output_dir = media_path / camera_id / date_str
    output_dir.mkdir(parents=True, exist_ok=True)
    output_file = output_dir / f"{recording_id}.mp4"

    cmd = [
        "ffmpeg",
        "-loglevel", "error",
        "-rtsp_transport", "tcp",
        "-i", rtsp_url,
        "-c:v", "copy",
        "-c:a", "aac",
        "-b:a", "128k",
        "-f", "mp4",
        "-movflags", "+faststart+frag_keyframe+empty_moov",
        str(output_file),
    ]

    try:
        process = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        _active_recordings[recording_id] = process

        # Обновим file_path в БД (синхронно через SQLAlchemy — в Celery task)
        _update_recording_path(recording_id, str(output_file))

        logger.info(
            "Recording started",
            recording_id=recording_id,
            camera_id=camera_id,
            output=str(output_file),
        )

        return {
            "status": "started",
            "recording_id": recording_id,
            "file_path": str(output_file),
        }
    except Exception as exc:
        logger.error("Failed to start recording: %s", exc)
        raise self.retry(exc=exc, countdown=30)


@celery_app.task(bind=True)
def stop_recording_task(self, recording_id: str) -> dict:
    """
    Останавливает запись для указанного recording_id.
    """
    process = _active_recordings.pop(recording_id, None)
    if process is None:
        logger.warning("No active ffmpeg process for recording %s", recording_id)
        _update_recording_status(recording_id, "completed")
        return {"status": "not_found", "recording_id": recording_id}

    try:
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()

        _update_recording_status(recording_id, "completed")
        logger.info("Recording stopped: %s", recording_id)

        return {"status": "stopped", "recording_id": recording_id}
    except Exception as exc:
        logger.error("Failed to stop recording %s: %s", recording_id, exc)
        _update_recording_status(recording_id, "failed")
        return {"status": "error", "recording_id": recording_id, "error": str(exc)}


@celery_app.task(bind=True)
def generate_thumbnail_task(self, recording_id: str, file_path: str, timestamp: float = 10.0) -> dict:
    """
    Генерирует превью (thumbnail) для записи.
    """
    thumb_path = Path(file_path).with_suffix(".thumb.jpg")

    cmd = [
        "ffmpeg",
        "-loglevel", "error",
        "-ss", str(timestamp),
        "-i", file_path,
        "-vframes", "1",
        "-q:v", "3",
        str(thumb_path),
    ]

    try:
        subprocess.run(cmd, check=True, timeout=30)
        logger.info("Thumbnail generated: %s", thumb_path)
        return {
            "status": "ok",
            "recording_id": recording_id,
            "thumbnail_path": str(thumb_path),
        }
    except Exception as exc:
        logger.error("Thumbnail generation failed: %s", exc)
        return {"status": "error", "recording_id": recording_id, "error": str(exc)}


@celery_app.task(name="app.tasks.video.check_cameras_online")
def check_cameras_online() -> dict:
    """
    Проверяет доступность всех камер через ffprobe.
    Обновляет поле is_online в БД.
    """
    import asyncio

    from sqlalchemy import create_engine, text

    sync_url = settings.database_url.replace("+asyncpg", "")
    engine = create_engine(sync_url)

    try:
        with engine.connect() as conn:
            result = conn.execute(text("SELECT id, rtsp_url FROM cameras WHERE is_enabled = true"))
            cameras = result.fetchall()

            online_count = 0
            offline_count = 0

            for cam in cameras:
                cam_id, rtsp_url = cam

                # Проверка через ffprobe
                try:
                    proc = subprocess.run(
                        [
                            "ffprobe", "-v", "quiet",
                            "-rtsp_transport", "tcp",
                            "-timeout", "5000000",
                            "-i", rtsp_url,
                            "-show_entries", "stream=codec_type",
                            "-of", "csv=p=0",
                        ],
                        capture_output=True,
                        timeout=8,
                    )
                    is_online = proc.returncode == 0 and len(proc.stdout) > 0
                except (subprocess.TimeoutExpired, Exception):
                    is_online = False

                new_status = str(is_online).lower()
                conn.execute(
                    text("UPDATE cameras SET is_online = :online WHERE id = :cid"),
                    {"online": new_status, "cid": cam_id},
                )
                conn.commit()

                if is_online:
                    online_count += 1
                else:
                    offline_count += 1

        logger.info("Health check: %d online, %d offline", online_count, offline_count)
        return {"online": online_count, "offline": offline_count}

    finally:
        engine.dispose()


# ---- helpers (синхронные, т.к. Celery работает синхронно) ----

def _update_recording_path(recording_id: str, file_path: str) -> None:
    """Обновляет file_path записи в БД."""
    from sqlalchemy import create_engine, text

    sync_url = settings.database_url.replace("+asyncpg", "")
    engine = create_engine(sync_url)
    try:
        with engine.connect() as conn:
            conn.execute(
                text("UPDATE recordings SET file_path = :fp WHERE id = :rid"),
                {"fp": file_path, "rid": recording_id},
            )
            conn.commit()
    finally:
        engine.dispose()


def _update_recording_status(recording_id: str, status: str) -> None:
    """Обновляет статус записи и end_time."""
    from sqlalchemy import create_engine, text

    sync_url = settings.database_url.replace("+asyncpg", "")
    engine = create_engine(sync_url)
    now = datetime.now(timezone.utc)
    try:
        with engine.connect() as conn:
            conn.execute(
                text(
                    "UPDATE recordings SET status = :st, end_time = :et WHERE id = :rid"
                ),
                {"st": status, "et": now, "rid": recording_id},
            )
            conn.commit()
    finally:
        engine.dispose()