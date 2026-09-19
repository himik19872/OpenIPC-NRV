"""
Frame Publisher — сервис публикации кадров для AI-детектора.

Берёт список камер из PostgreSQL, раз в N секунд вытаскивает по одному кадру
из RTSP-субпотока через ffmpeg и публикует его в NATS (тема cameras.<id>.frame).

Формат сообщения: camera_id (36 байт, дополнен пробелами) + JPEG.
Такой же формат читает ai-detector/main.py.

Переменные окружения:
  DATABASE_URL   — строка подключения к PostgreSQL (обязательно)
  NATS_URL       — адрес NATS (по умолчанию nats://nats:4222)
  FRAME_INTERVAL — период опроса камеры в секундах (по умолчанию 1.0)
  FRAME_WIDTH    — ширина кадра для детекции (по умолчанию 960, 0 = без ресайза)
  CAMERA_RELOAD  — период перечитывания списка камер, сек (по умолчанию 60)
  FFMPEG_TIMEOUT — таймаут одного захвата кадра, сек (по умолчанию 10)
"""

import asyncio
import logging
import os
import re
import signal
import subprocess
import sys
import time
from urllib.parse import quote, urlsplit, urlunsplit

import psycopg2
from nats.aio.client import Client as NATS

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger("frame-publisher")

# Периодические ошибки одной камеры не должны засорять лог: жалуемся редко
FAILURE_LOG_EVERY = 30


def build_rtsp_url(stream_url: str, username: str, password: str) -> str:
    """Подставляет логин/пароль в RTSP-URL, если их там ещё нет.

    У части камер учётные данные уже вшиты в URL — тогда ничего не меняем.
    Пароль кодируется через quote, т.к. может содержать @, : и другие символы.
    """
    if not username or not password:
        return stream_url

    parts = urlsplit(stream_url)
    if parts.username:  # креды уже есть в URL
        return stream_url

    host = parts.hostname or ""
    if parts.port:
        host = f"{host}:{parts.port}"
    netloc = f"{quote(username, safe='')}:{quote(password, safe='')}@{host}"

    return urlunsplit((parts.scheme, netloc, parts.path, parts.query, parts.fragment))


def grab_frame(url: str, width: int, timeout: float) -> bytes | None:
    """Забирает один кадр из RTSP и возвращает JPEG-байты."""
    vf = f"scale={width}:-2" if width else "null"
    cmd = [
        "ffmpeg",
        "-rtsp_transport", "tcp",
        "-i", url,
        "-frames:v", "1",
        "-vf", vf,
        "-q:v", "4",
        "-f", "image2",
        "-vcodec", "mjpeg",
        "-an",
        "-loglevel", "error",
        "pipe:1",
    ]
    try:
        proc = subprocess.run(cmd, capture_output=True, timeout=timeout)
    except subprocess.TimeoutExpired:
        return None

    data = proc.stdout
    # Проверяем, что это целый JPEG (начинается с FFD8, заканчивается FFD9)
    if not data or not data.startswith(b"\xff\xd8") or not data.endswith(b"\xff\xd9"):
        return None
    return data


def load_cameras(db_url: str) -> list[dict]:
    """Читает камеры с заполненным субпотоком (для детекции берём именно его)."""
    with psycopg2.connect(db_url) as conn:
        with conn.cursor() as cur:
            cur.execute("""
                SELECT id::text, name,
                       COALESCE(NULLIF(sub_stream, ''), NULLIF(main_stream, ''), rtsp_url) AS stream,
                       COALESCE(settings->>'username', '') AS username,
                       COALESCE(settings->>'password', '') AS password
                FROM cameras
                WHERE COALESCE(NULLIF(sub_stream, ''), NULLIF(main_stream, ''), rtsp_url) IS NOT NULL
                ORDER BY created_at
            """)
            return [
                {"id": r[0], "name": r[1], "stream": r[2], "username": r[3], "password": r[4]}
                for r in cur.fetchall()
            ]


class FramePublisher:
    def __init__(self, db_url: str, nats_url: str, interval: float,
                 width: int, reload_every: float, ffmpeg_timeout: float):
        self.db_url = db_url
        self.nats_url = nats_url
        self.interval = interval
        self.width = width
        self.reload_every = reload_every
        self.ffmpeg_timeout = ffmpeg_timeout

        self.nc: NATS | None = None
        self.cameras: list[dict] = []
        self.tasks: dict[str, asyncio.Task] = {}
        # Счётчики для сдержанного логирования: id -> (сбоев, последняя ошибка)
        self.failures: dict[str, int] = {}
        self.last_error: dict[str, str] = {}
        self.published: dict[str, int] = {}

    async def start(self):
        self.nc = NATS()
        await self.nc.connect(self.nats_url, max_reconnect_attempts=-1)
        logger.info(f"подключился к NATS: {self.nats_url}")

        await self.refresh_cameras()

        reloader = asyncio.create_task(self._reload_loop())
        monitor = asyncio.create_task(self._stats_loop())
        try:
            await asyncio.gather(reloader, monitor)
        finally:
            for t in self.tasks.values():
                t.cancel()

    async def refresh_cameras(self):
        """Перечитывает список камер и запускает/останавливает воркеры."""
        try:
            cameras = await asyncio.to_thread(load_cameras, self.db_url)
        except Exception as e:
            logger.error(f"не удалось прочитать камеры из БД: {e}")
            return

        wanted = {c["id"]: c for c in cameras}

        # Останавливаем воркеры камер, которых больше нет
        for cam_id in list(self.tasks):
            if cam_id not in wanted:
                self.tasks.pop(cam_id).cancel()
                logger.info(f"[{cam_id[:8]}] камера удалена — публикация остановлена")

        # Запускаем воркеры для новых камер
        for cam_id, cam in wanted.items():
            if cam_id not in self.tasks:
                self.tasks[cam_id] = asyncio.create_task(self._camera_loop(cam))
                logger.info(f"[{cam_id[:8]}] {cam['name']}: публикация запущена")

        self.cameras = cameras
        logger.info(f"камер в работе: {len(self.tasks)}")

    async def _reload_loop(self):
        while True:
            await asyncio.sleep(self.reload_every)
            await self.refresh_cameras()

    async def _camera_loop(self, cam: dict):
        """Цикл одной камеры: взять кадр → опубликовать → подождать интервал."""
        cam_id = cam["id"]
        url = build_rtsp_url(cam["stream"], cam["username"], cam["password"])
        # Не логируем URL целиком — в нём пароль
        safe_url = re.sub(r"//[^@/]+@", "//***@", url)

        while True:
            t0 = time.monotonic()
            jpeg = await asyncio.to_thread(grab_frame, url, self.width, self.ffmpeg_timeout)

            if jpeg:
                payload = cam_id.encode().ljust(36) + jpeg
                try:
                    await self.nc.publish(f"cameras.{cam_id}.frame", payload)
                except Exception as e:
                    logger.warning(f"[{cam_id[:8]}] публикация не удалась: {e}")

                self.published[cam_id] = self.published.get(cam_id, 0) + 1
                # После успеха сбрасываем счётчик сбоев и сообщаем о восстановлении
                if self.failures.get(cam_id):
                    logger.info(f"[{cam_id[:8]}] {cam['name']}: поток восстановлен")
                self.failures[cam_id] = 0
            else:
                n = self.failures.get(cam_id, 0) + 1
                self.failures[cam_id] = n
                if n == 1 or n % FAILURE_LOG_EVERY == 0:
                    logger.warning(
                        f"[{cam_id[:8]}] {cam['name']}: кадр не получен "
                        f"({n} раз подряд) {safe_url}"
                    )

            # Вычитаем потраченное время, чтобы держать стабильный интервал
            elapsed = time.monotonic() - t0
            await asyncio.sleep(max(0.05, self.interval - elapsed))

    async def _stats_loop(self):
        """Раз в минуту пишет, сколько кадров ушло."""
        while True:
            await asyncio.sleep(60)
            alive = sum(1 for cid in self.tasks if not self.failures.get(cid))
            total = sum(self.published.values())
            logger.info(
                f"статистика: камер активно {alive}/{len(self.tasks)}, "
                f"кадров опубликовано {total}"
            )


async def main():
    db_url = os.getenv("DATABASE_URL", "")
    if not db_url:
        logger.error("не задана переменная DATABASE_URL")
        sys.exit(1)

    publisher = FramePublisher(
        db_url=db_url,
        nats_url=os.getenv("NATS_URL", "nats://nats:4222"),
        interval=float(os.getenv("FRAME_INTERVAL", "1.0")),
        width=int(os.getenv("FRAME_WIDTH", "960")),
        reload_every=float(os.getenv("CAMERA_RELOAD", "60")),
        ffmpeg_timeout=float(os.getenv("FFMPEG_TIMEOUT", "10")),
    )

    loop = asyncio.get_running_loop()
    stop = asyncio.Event()
    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, stop.set)

    task = asyncio.create_task(publisher.start())
    logger.info("Frame Publisher запущен")
    await stop.wait()

    logger.info("останавливаюсь...")
    task.cancel()
    try:
        await task
    except asyncio.CancelledError:
        pass
    if publisher.nc:
        await publisher.nc.close()
    logger.info("Frame Publisher остановлен")


if __name__ == "__main__":
    asyncio.run(main())
