"""
NRV Backend — Сервис интеграции с OpenIPC Majestic API.

Эндпоинты камеры на прошивке OpenIPC:
- Снапшоты: /image.jpg
- Ночной режим: /night/on, /night/off, /night/toggle
- IR-подсветка: /night/ircut, /night/light
- Аудио: /audio.opus, /audio.m4a, /audio.pcm, /play_audio
- Конфигурация: /api/v1/config.json
- Метрики: /metrics
- Потоки: /mjpeg, /video.mp4, /hls, ws://.../ws/video
"""
import base64
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from typing import Any, Optional
from urllib.parse import urlparse

import httpx

from app.core.config import get_settings

settings = get_settings()


class NightMode(str, Enum):
    ON = "on"
    OFF = "off"


class StreamType(str, Enum):
    MAIN = "0"   # H.264/H.265 основной поток
    SUB = "1"    # суб-поток
    JPEG = "2"   # JPEG поток


@dataclass
class OpenIPCConfig:
    """Конфигурация Majestic камеры."""
    ip_address: str
    user: str = "root"
    password: str = "12345"

    @property
    def base_url(self) -> str:
        return f"http://{self.ip_address}"

    @property
    def auth(self) -> tuple[str, str]:
        return (self.user, self.password)

    # --- URL-ы для разных эндпоинтов ---

    @property
    def snapshot_url(self) -> str:
        return f"{self.base_url}/image.jpg"

    @property
    def mjpeg_url(self) -> str:
        return f"{self.base_url}/mjpeg"

    @property
    def hls_url(self) -> str:
        return f"{self.base_url}/hls"

    @property
    def config_url(self) -> str:
        return f"{self.base_url}/api/v1/config.json"

    @property
    def config_schema_url(self) -> str:
        return f"{self.base_url}/api/v1/config.schema.json"

    @property
    def metrics_url(self) -> str:
        return f"{self.base_url}/metrics"

    @property
    def night_on_url(self) -> str:
        return f"{self.base_url}/night/on"

    @property
    def night_off_url(self) -> str:
        return f"{self.base_url}/night/off"

    @property
    def ircut_url(self) -> str:
        return f"{self.base_url}/night/ircut"

    @property
    def light_url(self) -> str:
        return f"{self.base_url}/night/light"

    @property
    def audio_opus_url(self) -> str:
        return f"{self.base_url}/audio.opus"

    @property
    def play_audio_url(self) -> str:
        return f"{self.base_url}/play_audio"

    def rtsp_url(self, stream: str = "0") -> str:
        return f"rtsp://{self.user}:{self.password}@{self.ip_address}/stream={stream}"

    def ws_video_url(self, stream: str = "0") -> str:
        return f"ws://{self.ip_address}/ws/video?stream={stream}"


def parse_ip_from_rtsp(rtsp_url: str) -> Optional[str]:
    """Извлекает IP-адрес из RTSP URL."""
    try:
        parsed = urlparse(rtsp_url)
        return parsed.hostname
    except Exception:
        return None


def parse_rtsp_auth(rtsp_url: str) -> tuple[str, str]:
    """Извлекает логин/пароль из RTSP URL."""
    try:
        parsed = urlparse(rtsp_url)
        return (parsed.username or "root", parsed.password or "12345")
    except Exception:
        return ("root", "12345")


def get_openipc_config(camera_rtsp_url: str) -> OpenIPCConfig:
    """
    Создаёт OpenIPCConfig из RTSP URL камеры.
    Пример: rtsp://root:12345@192.168.1.12/stream=0
    """
    ip = parse_ip_from_rtsp(camera_rtsp_url) or "192.168.1.1"
    user, password = parse_rtsp_auth(camera_rtsp_url)
    return OpenIPCConfig(ip_address=ip, user=user, password=password)


# ---- Основные функции API ----


async def get_snapshot(camera_rtsp_url: str) -> Optional[bytes]:
    """
    Получает снапшот с камеры OpenIPC (JPEG).
    Возвращает сырые байты изображения.
    """
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.get(cfg.snapshot_url)
            if resp.status_code == 200:
                return resp.content
            return None
    except Exception:
        return None


async def get_snapshot_base64(camera_rtsp_url: str) -> Optional[str]:
    """
    Получает снапшот и возвращает как base64 data URI.
    """
    data = await get_snapshot(camera_rtsp_url)
    if data:
        b64 = base64.b64encode(data).decode("utf-8")
        return f"data:image/jpeg;base64,{b64}"
    return None


async def set_night_mode(camera_rtsp_url: str, mode: NightMode) -> bool:
    """
    Включает/выключает ночной режим на камере OpenIPC.
    """
    cfg = get_openipc_config(camera_rtsp_url)
    url = cfg.night_on_url if mode == NightMode.ON else cfg.night_off_url
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.get(url)
            return resp.status_code == 200
    except Exception:
        return False


async def toggle_night_mode(camera_rtsp_url: str) -> bool:
    """
    Переключает ночной режим.
    """
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.get(f"{cfg.base_url}/night/toggle")
            return resp.status_code == 200
    except Exception:
        return False


async def toggle_ircut(camera_rtsp_url: str) -> bool:
    """
    Переключает IR-фильтр камеры (механический).
    """
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.get(cfg.ircut_url)
            return resp.status_code == 200
    except Exception:
        return False


async def toggle_light(camera_rtsp_url: str) -> bool:
    """
    Переключает IR-подсветку.
    """
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.get(cfg.light_url)
            return resp.status_code == 200
    except Exception:
        return False


async def get_majestic_config(camera_rtsp_url: str) -> Optional[dict[str, Any]]:
    """
    Получает полный конфиг Majestic с камеры.
    """
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.get(cfg.config_url)
            if resp.status_code == 200:
                return resp.json()
            return None
    except Exception:
        return None


async def get_majestic_metrics(camera_rtsp_url: str) -> Optional[str]:
    """
    Получает Prometheus-метрики с камеры.
    """
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.get(cfg.metrics_url)
            if resp.status_code == 200:
                return resp.text
            return None
    except Exception:
        return None


async def get_mjpeg_stream_url(camera_rtsp_url: str) -> Optional[str]:
    """
    Возвращает URL для MJPEG-стрима камеры OpenIPC.
    Используется для прямого эмбеддинга в веб-интерфейсе.
    """
    cfg = get_openipc_config(camera_rtsp_url)
    return cfg.mjpeg_url


async def get_hls_stream_url(camera_rtsp_url: str) -> Optional[str]:
    """
    Возвращает URL для HLS-стрима камеры OpenIPC.
    """
    cfg = get_openipc_config(camera_rtsp_url)
    return cfg.hls_url


async def get_webrtc_ws_url(camera_rtsp_url: str, stream: str = "0") -> Optional[str]:
    """
    Возвращает WebSocket URL для низколатентного fMP4/MSE стрима.
    """
    cfg = get_openipc_config(camera_rtsp_url)
    return cfg.ws_video_url(stream)


async def play_audio_on_camera(camera_rtsp_url: str, audio_data: bytes) -> bool:
    """
    Отправляет аудио на динамик камеры (play_audio).
    """
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=15.0, auth=cfg.auth) as client:
            resp = await client.post(
                cfg.play_audio_url,
                content=audio_data,
                headers={"Content-Type": "audio/opus"},
            )
            return resp.status_code == 200
    except Exception:
        return False


async def ping_camera(camera_rtsp_url: str, timeout: float = 3.0) -> bool:
    """
    Быстрая проверка доступности камеры через /image.jpg.
    """
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=timeout, auth=cfg.auth) as client:
            resp = await client.get(cfg.snapshot_url)
            return resp.status_code == 200
    except Exception:
        return False


# ============================================================
# Двухстороннее аудио (Intercom)
# ============================================================


async def play_audio_to_speaker(camera_rtsp_url: str, audio_data: bytes) -> bool:
    """
    Проигрывает аудио на динамик камеры (двухсторонняя связь).
    POST /play_audio — Opus-аудио.
    """
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=15.0, auth=cfg.auth) as client:
            resp = await client.post(
                cfg.play_audio_url,
                content=audio_data,
                headers={"Content-Type": "audio/opus"},
            )
            return resp.status_code == 200
    except Exception:
        return False


async def get_audio_stream_url(camera_rtsp_url: str, codec: str = "opus") -> Optional[str]:
    """
    URL для получения аудиопотока с камеры.
    codec: opus, m4a, pcm, alaw, ulaw
    """
    cfg = get_openipc_config(camera_rtsp_url)
    return f"{cfg.base_url}/audio.{codec}"


async def stream_camera_audio(camera_rtsp_url: str, codec: str = "opus") -> Optional[bytes]:
    """
    Получает аудиоданные с камеры (сырой стрим).
    """
    cfg = get_openipc_config(camera_rtsp_url)
    url = f"{cfg.base_url}/audio.{codec}"
    try:
        async with httpx.AsyncClient(timeout=30.0, auth=cfg.auth) as client:
            async with client.stream("GET", url) as resp:
                if resp.status_code == 200:
                    chunks: list[bytes] = []
                    async for chunk in resp.aiter_bytes(chunk_size=4096):
                        chunks.append(chunk)
                        if len(chunks) > 50:  # лимит ~200KB
                            break
                    return b"".join(chunks)
                return None
    except Exception:
        return None


async def enable_audio_output(camera_rtsp_url: str, enable: bool = True) -> bool:
    """Включает/выключает выходной аудиоканал (динамик)."""
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            # Через Majestic API обновляем audio.outputEnabled
            resp = await client.patch(
                cfg.config_url,
                json={"audio": {"outputEnabled": enable}},
            )
            return resp.status_code == 200
    except Exception:
        return False


# ============================================================
# SIP-клиент
# ============================================================


async def get_sip_config(camera_rtsp_url: str) -> Optional[dict[str, Any]]:
    """Получает SIP-конфигурацию с камеры."""
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.get(cfg.config_url)
            if resp.status_code == 200:
                full = resp.json()
                return full.get("sip")
            return None
    except Exception:
        return None


async def configure_sip(
    camera_rtsp_url: str,
    enabled: bool = True,
    server: str = "",
    port: int = 5060,
    username: str = "",
    password: str = "",
    call_target: str = "",
    local_ip: str = "",
    do_register: bool = True,
) -> bool:
    """
    Настраивает SIP-клиент на камере.
    После настройки камера может звонить/принимать звонки через SIP-сервер.
    """
    cfg = get_openipc_config(camera_rtsp_url)
    sip_cfg: dict[str, Any] = {
        "enabled": enabled,
        "port": port,
        "username": username,
        "password": password,
        "localUri": f"sip:{username}@{server}" if username and server else "",
        "callTarget": call_target,
        "localIp": local_ip,
        "localPort": port,
        "doRegister": do_register,
        "registerExpires": 3600,
    }
    if server:
        sip_cfg["server"] = server

    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.patch(cfg.config_url, json={"sip": sip_cfg})
            return resp.status_code == 200
    except Exception:
        return False


# ============================================================
# Motion Detection / Детекция движения
# ============================================================


async def get_motion_config(camera_rtsp_url: str) -> Optional[dict[str, Any]]:
    """Получает конфигурацию детекции движения."""
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.get(cfg.config_url)
            if resp.status_code == 200:
                return resp.json().get("motionDetect")
            return None
    except Exception:
        return None


async def set_motion_detection(
    camera_rtsp_url: str,
    enabled: bool = True,
    sensitivity: int = 3,
    visualize: bool = False,
    roi: list | None = None,
) -> bool:
    """
    Включает/выключает детекцию движения.
    sensitivity: 1-5 (5 = макс. чувствительность)
    roi: list of [x1, y1, x2, y2] — зоны интереса
    """
    cfg = get_openipc_config(camera_rtsp_url)
    motion_cfg: dict[str, Any] = {
        "enabled": enabled,
        "visualize": visualize,
        "debug": False,
        "sensitivity": sensitivity,
        "roi": roi or [],
    }
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.patch(cfg.config_url, json={"motionDetect": motion_cfg})
            return resp.status_code == 200
    except Exception:
        return False


# ============================================================
# Запись на SD-карту (Majestic records)
# ============================================================


async def configure_sd_recording(
    camera_rtsp_url: str,
    enabled: bool = True,
    path: str = "/mnt/mmcblk0p1/%F",
    split_minutes: int = 20,
    max_usage_percent: int = 95,
) -> bool:
    """Настраивает запись видео на SD-карту камеры."""
    cfg = get_openipc_config(camera_rtsp_url)
    rec_cfg: dict[str, Any] = {
        "enabled": enabled,
        "path": path,
        "split": split_minutes,
        "maxUsage": max_usage_percent,
        "substream": False,
        "audioCodec": "alaw",
    }
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.patch(cfg.config_url, json={"records": rec_cfg})
            return resp.status_code == 200
    except Exception:
        return False


# ============================================================
# HLS стриминг
# ============================================================


async def set_hls(camera_rtsp_url: str, enabled: bool = True) -> bool:
    """Включает/выключает HLS-стриминг на камере."""
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.patch(cfg.config_url, json={"hls": {"enabled": enabled}})
            return resp.status_code == 200
    except Exception:
        return False


# ============================================================
# ONVIF
# ============================================================


async def configure_onvif(
    camera_rtsp_url: str,
    enabled: bool = True,
    username: str = "root",
    password: str = "",
) -> bool:
    """Настраивает ONVIF на камере."""
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.patch(
                cfg.config_url,
                json={"onvif": {"enabled": enabled, "username": username, "password": password}},
            )
            return resp.status_code == 200
    except Exception:
        return False


# ============================================================
# Обновление Majestic-конфига (PATCH)
# ============================================================


async def update_majestic_config(camera_rtsp_url: str, partial_config: dict[str, Any]) -> bool:
    """
    Частичное обновление Majestic-конфига (PATCH).
    Позволяет менять любые параметры Majestic через API.
    """
    cfg = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg.auth) as client:
            resp = await client.patch(cfg.config_url, json=partial_config)
            return resp.status_code == 200
    except Exception:
        return False


# ============================================================
# Outgoing stream (ретрансляция на другой сервер)
# ============================================================


async def configure_outgoing_stream(
    camera_rtsp_url: str,
    enabled: bool = True,
    url: str = "",
    nalu_size: int = 1200,
    substream: bool = False,
) -> bool:
    """Настраивает outgoing стрим (отправка видео на внешний RTMP/RTSP сервер)."""
    cfg_obj = get_openipc_config(camera_rtsp_url)
    try:
        async with httpx.AsyncClient(timeout=10.0, auth=cfg_obj.auth) as client:
            resp = await client.patch(
                cfg_obj.config_url,
                json={"outgoing": {"enabled": enabled, "naluSize": nalu_size, "substream": substream}},
            )
            return resp.status_code == 200
    except Exception:
        return False