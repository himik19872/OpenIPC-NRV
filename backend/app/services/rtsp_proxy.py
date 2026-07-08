"""
NRV Backend — RTSP-прокси сервис через go2rtc.

Принцип работы:
1. Каждая камера регистрируется в go2rtc как cam_{id}_main и cam_{id}_sub
2. go2rtc забирает RTSP с камеры ОДИН раз
3. Все потребители (WebRTC, RTSP, MJPEG) забирают поток с go2rtc
4. Нагрузка с камеры снимается — она отдаёт только 1-2 подключения

Прокси-URL для внешних сервисов:
- RTSP основной:  rtsp://nvr:8554/cam_{camera_id}_main
- RTSP дополнительный: rtsp://nvr:8554/cam_{camera_id}_sub
- WebRTC: через go2rtc API
- MJPEG: /api/streams/{camera_id}/proxy.mjpeg
"""
import logging
from typing import Optional

import httpx

from app.core.config import get_settings

settings = get_settings()
logger = logging.getLogger(__name__)

# Базовый URL go2rtc (RTSP-сервер для внешних потребителей)
GO2RTC_RTSP_BASE = f"rtsp://{settings.go2rtc_api_url.replace('/api', '').replace('http://', '').replace(':1984', '')}:8554"


def get_stream_name(camera_id: str, stream_type: str = "main") -> str:
    """Формирует имя стрима в go2rtc: cam_{id}_main или cam_{id}_sub."""
    return f"cam_{camera_id}_{stream_type}"


def get_proxy_rtsp_url(camera_id: str, stream_type: str = "main") -> str:
    """RTSP-URL прокси-потока для внешних потребителей."""
    return f"{GO2RTC_RTSP_BASE}/{get_stream_name(camera_id, stream_type)}"


async def register_camera_in_go2rtc(
    camera_id: str,
    rtsp_main_url: str,
    rtsp_sub_url: Optional[str] = None,
) -> dict:
    """
    Регистрирует основной и дополнительный потоки камеры в go2rtc.

    Returns:
        dict с информацией о зарегистрированных прокси-потоках:
        {
            "stream_main": "cam_{id}_main",
            "stream_sub": "cam_{id}_sub" или None,
            "rtsp_proxy_main": "rtsp://nvr:8554/cam_{id}_main",
            "rtsp_proxy_sub": "rtsp://nvr:8554/cam_{id}_sub" или None,
        }
    """
    result = {
        "stream_main": None,
        "stream_sub": None,
        "rtsp_proxy_main": None,
        "rtsp_proxy_sub": None,
    }

    # Регистрируем основной поток
    stream_main = get_stream_name(camera_id, "main")
    ok = await _add_go2rtc_stream(stream_main, rtsp_main_url)
    if ok:
        result["stream_main"] = stream_main
        result["rtsp_proxy_main"] = get_proxy_rtsp_url(camera_id, "main")
        logger.info("RTSP proxy main stream registered: camera=%s stream=%s", camera_id, stream_main)

    # Регистрируем дополнительный поток (если есть)
    if rtsp_sub_url:
        stream_sub = get_stream_name(camera_id, "sub")
        ok = await _add_go2rtc_stream(stream_sub, rtsp_sub_url)
        if ok:
            result["stream_sub"] = stream_sub
            result["rtsp_proxy_sub"] = get_proxy_rtsp_url(camera_id, "sub")
            logger.info("RTSP proxy sub stream registered: camera=%s stream=%s", camera_id, stream_sub)

    return result


async def unregister_camera_from_go2rtc(camera_id: str) -> bool:
    """Удаляет потоки камеры из go2rtc."""
    success = True
    for stream_type in ("main", "sub"):
        stream_name = get_stream_name(camera_id, stream_type)
        if not await _delete_go2rtc_stream(stream_name):
            success = False
    return success


async def get_go2rtc_stream_info(stream_name: str) -> Optional[dict]:
    """Получает информацию о стриме из go2rtc (потребители, треки, битрейт)."""
    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(f"{settings.go2rtc_api_url}/streams")
            if resp.status_code == 200:
                streams = resp.json()
                return streams.get(stream_name)
            return None
    except Exception:
        return None


async def get_camera_proxy_status(camera_id: str) -> dict:
    """
    Возвращает статус прокси-потоков камеры:
    - количество потребителей на каждом потоке
    - активность стримов
    """
    result = {
        "camera_id": camera_id,
        "streams": {},
    }
    for stream_type in ("main", "sub"):
        stream_name = get_stream_name(camera_id, stream_type)
        info = await get_go2rtc_stream_info(stream_name)
        if info:
            consumers = info.get("consumers", [])
            producers = info.get("producers", [])
            result["streams"][stream_type] = {
                "name": stream_name,
                "active": len(producers) > 0,
                "consumer_count": len(consumers),
                "consumers": consumers,
                "rtsp_proxy_url": get_proxy_rtsp_url(camera_id, stream_type),
            }
        else:
            result["streams"][stream_type] = {
                "name": stream_name,
                "active": False,
                "consumer_count": 0,
                "rtsp_proxy_url": get_proxy_rtsp_url(camera_id, stream_type),
            }
    return result


# ---- Внутренние функции для работы с go2rtc API ----


async def _add_go2rtc_stream(stream_name: str, rtsp_url: str) -> bool:
    """
    Добавляет RTSP-поток в go2rtc.
    POST /api/streams?name={name}&src={rtsp_url}
    Добавляет #transport=tcp для совместимости с OpenIPC.
    """
    # go2rtc использует UDP по умолчанию, но OpenIPC требует TCP
    if "#" not in rtsp_url:
        rtsp_url += "#video=all&audio=all&transport=tcp"

    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.put(
                f"{settings.go2rtc_api_url}/streams",
                params={"name": stream_name, "src": rtsp_url},
            )
            return resp.status_code == 200 or resp.status_code == 201
    except Exception as exc:
        logger.error("Failed to add go2rtc stream=%s error=%s", stream_name, str(exc))
        return False


async def _delete_go2rtc_stream(stream_name: str) -> bool:
    """Удаляет стрим из go2rtc."""
    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.delete(
                f"{settings.go2rtc_api_url}/streams",
                params={"name": stream_name},
            )
            return resp.status_code == 200
    except Exception:
        return False


async def get_all_go2rtc_streams() -> list:
    """Возвращает список всех активных стримов go2rtc."""
    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(f"{settings.go2rtc_api_url}/streams")
            if resp.status_code == 200:
                return resp.json()
            return []
    except Exception:
        return []


# ---- MSE/WebRTC (без перекодирования!) ----


def get_mse_url(camera_id: str, stream_type: str = "main") -> str:
    """
    URL для H.264-стриминга через MSE (Media Source Extensions).
    Браузер играет НАТИВНЫЙ H.264 без ffmpeg!

    go2rtc отдаёт фрагментированный MP4 (fMP4) — браузер парсит через MSE.
    """
    stream_name = get_stream_name(camera_id, stream_type)
    return f"{settings.go2rtc_api_url.replace('/api', '')}/api/stream.mse?src={stream_name}"


def get_webrtc_url(camera_id: str, stream_type: str = "main") -> str:
    """URL для WebRTC-стриминга (сверхнизкая задержка)."""
    stream_name = get_stream_name(camera_id, stream_type)
    return f"{settings.go2rtc_api_url.replace('/api', '')}/api/webrtc?src={stream_name}"


def get_mp4_url(camera_id: str, stream_type: str = "main") -> str:
    """
    URL для прямого H.264 MP4-потока.
    Можно использовать в <video src="..."> или ffplay.
    """
    stream_name = get_stream_name(camera_id, stream_type)
    return f"{settings.go2rtc_api_url.replace('/api', '')}/api/stream.mp4?src={stream_name}"


async def proxy_mse_stream(camera_id: str, stream_type: str = "main"):
    """
    Проксирует MSE-поток от go2rtc в FastAPI (сохраняя авторизацию).
    H.264 fMP4 — без перекодирования, почти ноль CPU.
    """
    mse_url = get_mse_url(camera_id, stream_type)
    async with httpx.AsyncClient(timeout=3600) as client:
        async with client.stream("GET", mse_url) as resp:
            async for chunk in resp.aiter_bytes(65536):
                yield chunk


async def restart_go2rtc() -> bool:
    """Перезапускает go2rtc (применяет конфиг)."""
    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.post(f"{settings.go2rtc_api_url}/restart")
            return resp.status_code == 200
    except Exception:
        return False
