"""
NRV Backend — Сервис стриминга: RTSP-прокси.
Преобразует RTSP-поток в MJPEG (multipart/x-mixed-replace) для веб-плеера.
"""
import asyncio
import subprocess
from typing import AsyncGenerator


async def generate_rtsp_proxy_stream(rtsp_url: str, fps: int = 5, quality: int = 8, scale: str = "") -> AsyncGenerator[bytes, None]:
    """
    Запускает ffmpeg для чтения RTSP и генерирует MJPEG-фреймы.
    Каждый фрейм оборачивается в multipart/x-mixed-replace контейнер.

    Args:
        rtsp_url: RTSP URL камеры
        fps: частота кадров MJPEG (меньше = меньше трафика)
        quality: качество JPEG (2-31, меньше = лучше)
        scale: масштабирование, например "320:240" или "640:360"
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

    process = await asyncio.create_subprocess_exec(
        *cmd,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )

    try:
        while True:
            # Читаем JPEG-фреймы из stdout ffmpeg
            # ffmpeg в режиме mjpeg выдаёт JPEG-файлы один за другим
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