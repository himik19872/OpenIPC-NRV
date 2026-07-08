"""
NRV Backend — Сервис стриминга: RTSP-прокси и MJPEG.
Преобразует RTSP-поток в MJPEG (multipart/x-mixed-replace) для веб-плеера.
Поддерживает выбор основного (высокое качество) и дополнительного (низкое) потоков.
"""
import asyncio
import logging
from typing import AsyncGenerator

from app.core.config import get_settings

settings = get_settings()
logger = logging.getLogger(__name__)

# Качество по умолчанию для разных режимов
STREAM_PRESETS = {
    "main": {"fps": 15, "quality": 5, "scale": ""},           # основной: полное качество
    "sub": {"fps": 5, "quality": 10, "scale": "640:360"},     # доп. поток: низкое
    "thumbnail": {"fps": 1, "quality": 15, "scale": "320:180"},  # превью
}


async def generate_rtsp_proxy_stream(
    rtsp_url: str,
    fps: int = 5,
    quality: int = 8,
    scale: str = "",
    stream_type: str = "main",
) -> AsyncGenerator[bytes, None]:
    """
    Запускает ffmpeg для чтения RTSP и генерирует MJPEG-фреймы.
    Каждый фрейм оборачивается в multipart/x-mixed-replace контейнер.

    Args:
        rtsp_url: RTSP URL (камеры напрямую или go2rtc-прокси)
        fps: частота кадров MJPEG
        quality: качество JPEG (2-31, меньше = лучше)
        scale: масштабирование, например "320:240" или "640:360"
        stream_type: тип потока ("main", "sub", "thumbnail") — влияет на логирование
    """
    cmd = [
        "ffmpeg",
        "-loglevel", "error",
        "-rtsp_transport", "tcp",
        "-i", rtsp_url,
        "-f", "mjpeg",
        "-q:v", str(quality),
        "-r", str(fps),
        "-an",
    ]
    if scale:
        cmd += ["-vf", f"scale={scale}"]
    cmd.append("pipe:1")

    logger.info("Starting MJPEG proxy stream", stream_type=stream_type, fps=fps)

    process = await asyncio.create_subprocess_exec(
        *cmd,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )

    try:
        while True:
            chunk = await process.stdout.read(65536)
            if not chunk:
                break

            yield (
                b"--frame\r\n"
                b"Content-Type: image/jpeg\r\n\r\n"
                + chunk
                + b"\r\n"
            )
    finally:
        process.kill()
        await process.wait()
        logger.info("MJPEG proxy stream stopped", stream_type=stream_type)


async def generate_proxy_mjpeg(
    camera_id: str,
    stream_type: str = "main",
    preset: str | None = None,
) -> AsyncGenerator[bytes, None]:
    """
    Генерирует MJPEG-поток через go2rtc-прокси (снимает нагрузку с камеры).

    Использует RTSP-прокси go2rtc вместо прямого подключения к камере.
    go2rtc уже забрал поток с камеры, мы забираем с go2rtc.

    Args:
        camera_id: ID камеры
        stream_type: "main" или "sub"
        preset: пресет качества ("main", "sub", "thumbnail") или None для ручных настроек
    """
    from app.services.rtsp_proxy import get_proxy_rtsp_url

    # Получаем прокси-RTSP URL (go2rtc уже держит подключение к камере)
    proxy_rtsp = get_proxy_rtsp_url(camera_id, stream_type)

    # Применяем пресет или значения по умолчанию
    cfg = STREAM_PRESETS.get(preset, STREAM_PRESETS.get(stream_type, STREAM_PRESETS["sub"]))

    async for frame in generate_rtsp_proxy_stream(
        rtsp_url=proxy_rtsp,
        fps=cfg["fps"],
        quality=cfg["quality"],
        scale=cfg["scale"],
        stream_type=f"{stream_type}_proxy",
    ):
        yield frame


async def generate_h264_native_stream(
    rtsp_url: str,
    stream_type: str = "main",
) -> AsyncGenerator[bytes, None]:
    """
    Запускает ffmpeg с -c copy для ремуксинга RTSP→fMP4.
    НЕТ перекодирования! H.264 остаётся H.264, только меняется контейнер.

    CPU: почти ноль (только копирование пакетов, нет декодирования/энкодинга).
    Для 25 камер потребление CPU — минимальное.

    Args:
        rtsp_url: RTSP URL камеры
        stream_type: для логирования
    """
    cmd = [
        "ffmpeg",
        "-loglevel", "error",
        "-rtsp_transport", "tcp",
        "-i", rtsp_url,
        "-c:v", "copy",       # БЕЗ ПЕРЕКОДИРОВАНИЯ!
        "-c:a", "aac",        # Аудио — перекодируем (CPU минимально)
        "-b:a", "64k",
        "-f", "mp4",
        "-movflags", "frag_keyframe+empty_moov+default_base_moof",
        "-an",                 # пока без аудио для минимальной нагрузки
        "pipe:1",
    ]

    logger.info("Starting H.264 native stream: %s %s", stream_type, rtsp_url[:50])

    process = await asyncio.create_subprocess_exec(
        *cmd,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )

    try:
        while True:
            chunk = await process.stdout.read(65536)
            if not chunk:
                break
            yield chunk
    finally:
        process.kill()
        await process.wait()
        logger.info("H.264 native stream stopped: %s", stream_type)


async def check_rtsp_availability(rtsp_url: str, timeout: float = 5.0) -> bool:
    """
    Проверяет доступность RTSP-потока через ffprobe.
    Возвращает True, если поток доступен.
    """
    try:
        cmd = [
            "ffprobe",
            "-v", "quiet",
            "-rtsp_transport", "tcp",
            "-timeout", str(int(timeout * 1_000_000)),  # микросекунды
            "-i", rtsp_url,
            "-show_entries", "stream=codec_type",
            "-of", "csv=p=0",
        ]
        process = await asyncio.create_subprocess_exec(
            *cmd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        stdout, _ = await asyncio.wait_for(process.communicate(), timeout=timeout + 2)
        return process.returncode == 0 and len(stdout) > 0
    except (asyncio.TimeoutError, Exception):
        return False