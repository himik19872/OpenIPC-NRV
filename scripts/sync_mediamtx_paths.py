#!/usr/bin/env python3
"""
Перерегистрирует пути MediaMTX по данным из базы.

Зачем: путь в MediaMTX хранит RTSP-адрес, полученный при создании. Если
адрес или учётные данные камеры изменили в базе (или восстановили после
сбоя), сам путь не обновится: MediaMTX считает его существующим и отвечает
400 «path already exists» на повторное добавление. Тогда камера остаётся
нерабочей, хотя в интерфейсе всё заполнено верно.

Скрипт читает актуальные адреса из БД и для каждого пути делает ADD,
а при конфликте — PATCH с новым источником.

Запуск (на сервере, где поднят бэкенд):
    python3 scripts/sync_mediamtx_paths.py

Переменные окружения:
    DATABASE_URL  — строка подключения к PostgreSQL
                    (по умолчанию postgres://nvr:nvr_secret@127.0.0.1:5434/nvr)
    MEDIAMTX_API  — адрес API медиасервера (по умолчанию http://127.0.0.1:9997)
"""

import json
import os
import sys
import urllib.error
import urllib.request

DATABASE_URL = os.getenv(
    "DATABASE_URL", "postgres://nvr:nvr_secret@127.0.0.1:5434/nvr"
)
MEDIAMTX_API = os.getenv("MEDIAMTX_API", "http://127.0.0.1:9997")


def load_cameras() -> list[dict]:
    """
    Читает камеры из БД вместе с учётными данными.

    Креды хранятся отдельно в settings (JSONB) и подставляются в RTSP-адрес
    только здесь: в базе URL хранится без пароля, чтобы смена пароля
    требовала правки в одном месте.
    """
    try:
        import psycopg2
        from psycopg2.extras import RealDictCursor
    except ImportError:
        print("Нужен psycopg2: pip install psycopg2-binary", file=sys.stderr)
        sys.exit(1)

    # psycopg2 понимает префикс postgres://, но не параметр sslmode=disable
    # в формате URL — убираем его, чтобы подключение не падало.
    dsn = DATABASE_URL.replace("?sslmode=disable", "").replace("&sslmode=disable", "")

    with psycopg2.connect(dsn) as conn:
        with conn.cursor(cursor_factory=RealDictCursor) as cur:
            cur.execute("""
                SELECT id::text,
                       COALESCE(main_stream, '') AS main_stream,
                       COALESCE(sub_stream, '')  AS sub_stream,
                       COALESCE(rtsp_url, '')    AS rtsp_url,
                       COALESCE(settings, '{}'::jsonb) AS settings
                FROM cameras
            """)
            return [dict(row) for row in cur.fetchall()]


def embed_credentials(rtsp_url: str, username: str, password: str) -> str:
    """Вставляет логин и пароль в RTSP-адрес, если их там ещё нет."""
    if not rtsp_url or (not username and not password):
        return rtsp_url
    if "@" in rtsp_url:
        return rtsp_url  # креды уже в адресе — не трогаем

    prefix = "rtsp://"
    if not rtsp_url.startswith(prefix):
        return rtsp_url

    creds = username
    if password:
        creds += ":" + password
    return prefix + creds + "@" + rtsp_url[len(prefix):]


def request(method: str, path: str, payload: dict | None = None) -> tuple[int, str]:
    """Выполняет запрос к API MediaMTX."""
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(
        MEDIAMTX_API + path,
        data=data,
        method=method,
        headers={"Content-Type": "application/json"} if data else {},
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            return r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


def configure(name: str, source: str) -> str:
    """Создаёт путь или обновляет у него источник, если путь уже есть."""
    payload = {
        "name": name,
        "source": source,
        "sourceOnDemand": False,
        # TCP исключает потери RTP-пакетов: для 4K-потоков UDP не справляется,
        # и HLS-муксер падает на переупорядоченных кадрах.
        "rtspTransport": "tcp",
    }

    status, body = request("POST", f"/v3/config/paths/add/{name}", payload)
    if 200 <= status < 300:
        return "добавлен"

    # Путь уже есть — обновляем источник. Именно здесь раньше терялись новые
    # учётные данные: при повторном добавлении MediaMTX ничего не менял.
    if "already exists" in body:
        status, body = request(
            "PATCH",
            f"/v3/config/paths/patch/{name}",
            {"source": source, "rtspTransport": "tcp"},
        )
        if 200 <= status < 300:
            return "обновлён"

    return f"ОШИБКА {status}: {body[:120]}"


def main() -> None:
    cameras = load_cameras()
    if not cameras:
        print("В базе нет камер")
        return

    ok, failed = 0, 0
    for cam in cameras:
        cam_id = cam["id"]
        settings = cam["settings"] or {}
        username = settings.get("username", "") or ""
        password = settings.get("password", "") or ""

        main_url = cam["main_stream"] or cam["rtsp_url"]
        sub_url = cam["sub_stream"]

        for name, url in ((cam_id, main_url), (cam_id + "_sub", sub_url)):
            if not url:
                continue
            source = embed_credentials(url, username, password)
            result = configure(name, source)
            if result.startswith("ОШИБКА"):
                failed += 1
            else:
                ok += 1
            print(f"{name[:42]:44s} {result}")

    print(f"\nГотово: настроено {ok}, ошибок {failed}")


if __name__ == "__main__":
    main()
