#!/usr/bin/env python3
"""Перекодирование архива из HEVC (H.265) в H.264.

Зачем: браузеры не воспроизводят HEVC в теге <video> (лицензионные ограничения),
поэтому записи с камер, отдающих H.265, в веб-интерфейсе не играют.
Новые записи уже транскодируются на лету в backend, а этот скрипт приводит
к совместимому виду уже накопленный архив.

Как работает:
  1. берёт из БД список записей с codec='hevc';
  2. для каждой скачивает объект из MinIO во временный файл;
  3. транскодирует HEVC → H.264 (libx264, yuv420p, +faststart);
  4. заливает результат обратно в MinIO поверх исходного объекта;
  5. обновляет codec в БД;
  6. удаляет временные файлы.

Запуск:
    python3 scripts/recode_hevc.py            # обработать все HEVC-записи
    python3 scripts/recode_hevc.py --dry-run  # только показать, что будет сделано
    python3 scripts/recode_hevc.py --limit 5  # обработать первые 5 (для проверки)

Параметры подключения берутся из переменных окружения (со значениями по умолчанию,
совпадающими с docker-compose.yml):
    PGHOST/PGPORT/PGUSER/PGPASSWORD/PGDATABASE
    MINIO_ENDPOINT/MINIO_ACCESS_KEY/MINIO_SECRET_KEY/MINIO_BUCKET
"""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
import tempfile
from dataclasses import dataclass

import boto3
import psycopg2
from botocore.client import Config

# --- Параметры подключения (значения по умолчанию совпадают с compose) ---
PG = {
    "host": os.getenv("PGHOST", "127.0.0.1"),
    "port": int(os.getenv("PGPORT", "5434")),
    "user": os.getenv("PGUSER", "nvr"),
    "password": os.getenv("PGPASSWORD", "nvr"),
    "dbname": os.getenv("PGDATABASE", "nvr"),
}
MINIO_ENDPOINT = os.getenv("MINIO_ENDPOINT", "http://127.0.0.1:9000")
MINIO_KEY = os.getenv("MINIO_ACCESS_KEY", "minioadmin")
MINIO_SECRET = os.getenv("MINIO_SECRET_KEY", "minioadmin")
BUCKET = os.getenv("MINIO_BUCKET", "nvr-recordings")


@dataclass
class Item:
    """Одна запись, требующая перекодирования."""
    rec_id: str
    key: str  # ключ объекта в MinIO (без префикса minio:)
    size: int


def s3_client():
    return boto3.client(
        "s3",
        endpoint_url=MINIO_ENDPOINT,
        aws_access_key_id=MINIO_KEY,
        aws_secret_access_key=MINIO_SECRET,
        config=Config(signature_version="s3v4"),
        region_name="us-east-1",
    )


def fetch_items(conn) -> list[Item]:
    """Возвращает записи с кодеком HEVC, лежащие в MinIO."""
    with conn.cursor() as cur:
        cur.execute(
            """
            SELECT id::text, file_path, COALESCE(file_size, 0)
            FROM recordings
            WHERE codec = 'hevc' AND file_path LIKE 'minio:%'
            ORDER BY start_time
            """
        )
        rows = cur.fetchall()

    items: list[Item] = []
    for rec_id, path, size in rows:
        key = path[len("minio:"):]
        if key:
            items.append(Item(rec_id=rec_id, key=key, size=size))
    return items


def transcode(src: str, dst: str) -> None:
    """HEVC → H.264. yuv420p и +faststart обязательны для браузеров."""
    subprocess.run(
        [
            "ffmpeg", "-y", "-loglevel", "error",
            "-i", src,
            "-c:v", "libx264",
            "-preset", "veryfast",
            "-crf", "23",
            "-profile:v", "main",
            # Без yuv420p часть браузеров отказывается играть файл
            "-pix_fmt", "yuv420p",
            # faststart переносит индекс в начало файла — нужно для стриминга
            "-movflags", "+faststart",
            "-an",  # аудио в архив не пишем (см. комментарий в recorder_service.go)
            dst,
        ],
        check=True,
        capture_output=True,
    )


def human(n: int) -> str:
    if n > 1 << 30:
        return f"{n / (1 << 30):.2f} GB"
    if n > 1 << 20:
        return f"{n / (1 << 20):.1f} MB"
    return f"{n / 1000:.0f} KB"


def main() -> int:
    ap = argparse.ArgumentParser(description="Перекодировать архив HEVC → H.264")
    ap.add_argument("--dry-run", action="store_true", help="только показать план")
    ap.add_argument("--limit", type=int, default=0, help="обработать не больше N записей")
    args = ap.parse_args()

    conn = psycopg2.connect(**PG)
    conn.autocommit = False
    s3 = s3_client()

    items = fetch_items(conn)
    if args.limit:
        items = items[: args.limit]

    if not items:
        print("Нет записей в HEVC — всё уже совместимо с браузером.")
        return 0

    total_in = sum(i.size for i in items)
    print(f"К перекодированию: {len(items)} записей, {human(total_in)}")
    if args.dry_run:
        for it in items[:5]:
            print(f"  {it.rec_id[:8]}  {human(it.size):>10}  {it.key}")
        if len(items) > 5:
            print(f"  ... и ещё {len(items) - 5}")
        print("\nРежим --dry-run: изменения не вносились.")
        return 0

    done = failed = 0
    saved = 0
    for idx, it in enumerate(items, 1):
        prefix = f"[{idx}/{len(items)}] {it.rec_id[:8]}"
        try:
            with tempfile.TemporaryDirectory() as tmp:
                src = os.path.join(tmp, "in.mp4")
                dst = os.path.join(tmp, "out.mp4")

                s3.download_file(BUCKET, it.key, src)
                transcode(src, dst)

                out_size = os.path.getsize(dst)
                # Пустой результат означает сбой транскодирования — исходник не трогаем
                if out_size == 0:
                    raise RuntimeError("получен пустой файл")

                s3.upload_file(dst, BUCKET, it.key, ExtraArgs={"ContentType": "video/mp4"})

                # Кодек меняем только после успешной загрузки: если упасть раньше,
                # в БД остался бы 'hevc' при уже готовом H.264 — расхождение.
                with conn.cursor() as cur:
                    cur.execute(
                        "UPDATE recordings SET codec='h264', file_size=%s WHERE id=%s",
                        (out_size, it.rec_id),
                    )
                conn.commit()

                saved += it.size - out_size
                done += 1
                print(f"{prefix}  {human(it.size)} → {human(out_size)}")
        except Exception as e:  # noqa: BLE001 — продолжаем обработку остальных
            conn.rollback()
            failed += 1
            msg = str(e).splitlines()[0][:120] if str(e) else type(e).__name__
            print(f"{prefix}  ОШИБКА: {msg}", file=sys.stderr)

    print(f"\nГотово: перекодировано {done}, ошибок {failed}, экономия {human(max(saved, 0))}")
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
