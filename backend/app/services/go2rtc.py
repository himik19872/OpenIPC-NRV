"""
NRV Backend — Сервис интеграции с go2rtc (WebRTC).
go2rtc API: https://github.com/AlexxIT/go2rtc
"""
from typing import Optional

import httpx

from app.core.config import get_settings

settings = get_settings()


async def add_go2rtc_stream(stream_name: str, rtsp_url: str) -> bool:
    """
    Добавляет RTSP-поток в go2rtc через API.
    POST /api/streams — добавляет стрим.
    """
    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.post(
                f"{settings.go2rtc_api_url}/streams",
                json={"name": stream_name, "sources": [rtsp_url]},
            )
            return resp.status_code == 200
    except Exception:
        return False


async def get_webrtc_sdp(stream_name: str) -> Optional[str]:
    """
    Получает SDP-предложение для WebRTC-подключения.
    POST /api/webrtc — создаёт WebRTC сессию.
    """
    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.post(
                f"{settings.go2rtc_api_url}/webrtc",
                json={"name": stream_name},
            )
            if resp.status_code == 200:
                return resp.json().get("sdp")
            return None
    except Exception:
        return None


async def get_go2rtc_streams() -> list:
    """Возвращает список активных стримов go2rtc."""
    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.get(f"{settings.go2rtc_api_url}/streams")
            if resp.status_code == 200:
                return resp.json()
            return []
    except Exception:
        return []