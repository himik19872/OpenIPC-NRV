"""
Проверка цепочки детекции: публикует тестовый кадр в NATS от имени камеры
и смотрит, какие события создаст детектор.

Использование:
  python test_pipeline.py <camera_id> [путь_к_изображению]

Без пути к изображению рисует синтетический кадр с фигурой, похожей на человека,
чтобы detector гарантированно что-то нашёл (для проверки фильтрации).
"""
import asyncio
import os
import subprocess
import sys
import time

import cv2
import numpy as np
from nats.aio.client import Client as NATS


def fetch_frame(rtsp_url: str) -> bytes | None:
    """Берёт один кадр с камеры через ffmpeg."""
    r = subprocess.run(
        ["ffmpeg", "-rtsp_transport", "tcp", "-i", rtsp_url,
         "-frames:v", "1", "-q:v", "3", "-f", "image2", "-y", "/tmp/test_frame.jpg",
         "-loglevel", "error"],
        capture_output=True, timeout=30,
    )
    if r.returncode != 0:
        return None
    with open("/tmp/test_frame.jpg", "rb") as f:
        return f.read()


def synthetic_frame() -> bytes:
    """Синтетический кадр: тёмный фон и светлая фигура в центре.

    Гарантирует, что детектор получит валидный JPEG, даже если камера недоступна.
    """
    img = np.full((480, 640, 3), 40, dtype=np.uint8)
    cv2.rectangle(img, (280, 120), (360, 360), (220, 220, 220), -1)  # "тело"
    cv2.circle(img, (320, 90), 35, (230, 230, 230), -1)              # "голова"
    ok, buf = cv2.imencode(".jpg", img, [int(cv2.IMWRITE_JPEG_QUALITY), 85])
    return buf.tobytes() if ok else b""


async def main():
    if len(sys.argv) < 2:
        print("использование: python test_pipeline.py <camera_id> [изображение]")
        sys.exit(1)

    camera_id = sys.argv[1]
    nats_url = os.getenv("NATS_URL", "nats://localhost:4222")
    rtsp_url = os.getenv("TEST_RTSP", "")

    if len(sys.argv) >= 3:
        with open(sys.argv[2], "rb") as f:
            jpeg = f.read()
        source = sys.argv[2]
    elif rtsp_url:
        jpeg = fetch_frame(rtsp_url)
        source = f"rtsp {rtsp_url}"
        if jpeg is None:
            print("кадр не получен, использую синтетический")
            jpeg, source = synthetic_frame(), "синтетика"
    else:
        jpeg = synthetic_frame()
        source = "синтетика"

    print(f"камера: {camera_id}")
    print(f"кадр:   {source}, {len(jpeg)} байт")

    nc = NATS()
    await nc.connect(nats_url)

    # Слушаем детекции по этой камере
    got = []

    async def on_detection(msg):
        got.append(msg.data.decode())

    sub = await nc.subscribe(f"cameras.{camera_id}.detection", cb=on_detection)

    # Формат сообщения кадра: camera_id (36 байт, добит пробелами) + JPEG
    payload = camera_id.encode().ljust(36) + jpeg
    for i in range(3):  # несколько кадров: трекер должен увидеть движение
        await nc.publish(f"cameras.{camera_id}.frame", payload)
        await asyncio.sleep(1.5)

    await asyncio.sleep(6)
    await sub.unsubscribe()

    print(f"\nполучено событий: {len(got)}")
    for g in got[:10]:
        print("  ", g[:180])

    await nc.close()
    if not got:
        sys.exit(2)


if __name__ == "__main__":
    asyncio.run(main())
