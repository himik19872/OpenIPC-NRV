"""
Frame Publisher — читает RTSP-поток, извлекает кадры (1 FPS),
публикует в NATS для AI-детектора.

Запуск:
  python frame_publisher.py --rtsp rtsp://localhost:8554/test_camera --camera-id cam1
"""

import os
import sys
import time
import asyncio
import argparse
import logging
import subprocess
import json

import cv2
import numpy as np
from nats.aio.client import Client as NATS

logging.basicConfig(level=logging.INFO, format='%(asctime)s [%(levelname)s] %(message)s')
logger = logging.getLogger("frame-publisher")


async def publish_frames(rtsp_url: str, camera_id: str, nats_url: str, fps: float = 1.0):
    """Читает RTSP через ffmpeg, публикует кадры в NATS"""
    
    # Подключаемся к NATS
    nc = NATS()
    await nc.connect(nats_url)
    logger.info(f"Connected to NATS at {nats_url}")

    # Запускаем ffmpeg: читаем RTSP, ресайзим до 640px, выдаём кадры в stdout
    ffmpeg_cmd = [
        'ffmpeg',
        '-rtsp_transport', 'tcp',
        '-i', rtsp_url,
        '-vf', f'fps={fps},scale=640:-1',  # 1 FPS, ширина 640px
        '-f', 'image2pipe',
        '-vcodec', 'mjpeg',
        '-q:v', '5',                       # качество JPEG
        '-an',                              # без аудио
        '-loglevel', 'error',
        'pipe:1'
    ]

    proc = subprocess.Popen(ffmpeg_cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    logger.info(f"Started ffmpeg for camera={camera_id}, rtsp={rtsp_url}")

    frame_count = 0
    try:
        while True:
            # Читаем размер JPEG (первые 2 байта)
            size_bytes = proc.stdout.read(2)
            if not size_bytes:
                break
            
            # Ищем начало JPEG (FF D8)
            while size_bytes != b'\xff\xd8':
                b = proc.stdout.read(1)
                if not b:
                    break
                size_bytes = size_bytes[1:2] + b
            
            # Читаем остаток JPEG до FF D9
            jpeg_data = b'\xff\xd8'
            buf = b''
            while True:
                chunk = proc.stdout.read(4096)
                if not chunk:
                    break
                buf += chunk
                end_idx = buf.find(b'\xff\xd9')
                if end_idx >= 0:
                    jpeg_data += buf[:end_idx + 2]
                    break
            
            if len(jpeg_data) < 500:
                continue  # слишком маленький кадр

            # Публикуем в NATS: префикс camera_id + JPEG
            payload = camera_id.encode().ljust(36) + jpeg_data
            await nc.publish(f"cameras.{camera_id}.frame", payload)
            
            frame_count += 1
            if frame_count % 30 == 0:
                logger.info(f"[{camera_id}] Published {frame_count} frames")

    except asyncio.CancelledError:
        pass
    except Exception as e:
        logger.error(f"[{camera_id}] Error: {e}")
    finally:
        proc.terminate()
        await nc.close()
        logger.info(f"[{camera_id}] Stopped. Total frames: {frame_count}")


async def main():
    parser = argparse.ArgumentParser(description='RTSP Frame Publisher')
    parser.add_argument('--rtsp', required=True, help='RTSP URL')
    parser.add_argument('--camera-id', required=True, help='Camera ID')
    parser.add_argument('--nats', default='nats://localhost:4222', help='NATS URL')
    parser.add_argument('--fps', type=float, default=1.0, help='Frames per second')
    args = parser.parse_args()

    await publish_frames(args.rtsp, args.camera_id, args.nats, args.fps)


if __name__ == '__main__':
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        logger.info("Interrupted")