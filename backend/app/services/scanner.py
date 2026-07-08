"""
NRV Backend — Сетевой сканер камер OpenIPC.

Автоматически находит камеры в локальной сети:
1. Сканирует подсеть через ping / HTTP
2. Проверяет наличие Majestic API (/api/v1/config.json)
3. Определяет модель, прошивку, параметры
"""
import asyncio
import ipaddress
import logging
import socket
from dataclasses import dataclass
from typing import Optional

import httpx

logger = logging.getLogger(__name__)


@dataclass
class DiscoveredCamera:
    """Найденная в сети камера."""

    ip_address: str
    model: str = "OpenIPC"
    firmware: str = ""
    rtsp_main_url: str = ""     # Основной поток (H.264/H.265, высокое качество)
    rtsp_sub_url: str = ""      # Дополнительный поток (низкое качество)
    rtsp_url: str = ""          # Устаревшее, для обратной совместимости
    snapshot_url: str = ""
    majestic_config: Optional[dict] = None
    is_online: bool = False

    def to_dict(self) -> dict:
        return {
            "ip_address": self.ip_address,
            "model": self.model,
            "firmware": self.firmware,
            "rtsp_main_url": self.rtsp_main_url,
            "rtsp_sub_url": self.rtsp_sub_url,
            "rtsp_url": self.rtsp_main_url,  # обратная совместимость
            "snapshot_url": self.snapshot_url,
            "is_online": self.is_online,
        }


async def discover_camera_at_ip(ip: str, timeout: float = 2.0) -> Optional[DiscoveredCamera]:
    """
    Проверяет один IP на наличие камеры OpenIPC.
    Пробует стандартные учётные данные: root / 12345.
    """
    base = f"http://{ip}"
    auth = ("root", "12345")

    try:
        async with httpx.AsyncClient(timeout=timeout) as client:
            # Проверяем наличие Majestic API с авторизацией
            resp = await client.get(f"{base}/api/v1/config.json", auth=auth)
            if resp.status_code != 200:
                # Пробуем без авторизации тоже
                resp = await client.get(f"{base}/api/v1/config.json")
                if resp.status_code != 200:
                    return None

            config = resp.json()
            if "system" not in config and "video0" not in config:
                return None  # Не Majestic

            # Это OpenIPC камера!
            cam = DiscoveredCamera(ip_address=ip)

            # Извлекаем модель / прошивку
            image = config.get("image", {})
            system = config.get("system", {})
            cam.model = system.get("vendor", "OpenIPC")

            # Пробуем получить версию с авторизацией
            try:
                ver_resp = await client.get(f"{base}/api/v1/version.json", auth=auth)
                if ver_resp.status_code == 200:
                    ver = ver_resp.json()
                    cam.firmware = ver.get("firmware", "") or ver.get("version", "")
            except Exception:
                # пробуем без авторизации
                try:
                    ver_resp = await client.get(f"{base}/api/v1/version.json")
                    if ver_resp.status_code == 200:
                        ver = ver_resp.json()
                        cam.firmware = ver.get("firmware", "") or ver.get("version", "")
                except Exception:
                    pass

            # Формируем RTSP URL'ы для основного и дополнительного потоков
            cam.rtsp_main_url = f"rtsp://root:12345@{ip}/stream=0"
            cam.rtsp_sub_url = f"rtsp://root:12345@{ip}/stream=1"
            cam.rtsp_url = cam.rtsp_main_url  # обратная совместимость
            cam.snapshot_url = f"{base}/image.jpg"
            cam.majestic_config = config
            cam.is_online = True

            return cam

    except (httpx.TimeoutException, httpx.ConnectError, OSError):
        return None
    except Exception as exc:
        logger.debug("Error checking %s: %s", ip, exc)
        return None


async def scan_network(
    subnet: str = "192.168.1.0/24",
    max_concurrent: int = 50,
    timeout: float = 2.0,
) -> list[DiscoveredCamera]:
    """
    Сканирует заданную подсеть на наличие камер OpenIPC.
    Параллельно проверяет все IP.

    Пример: scan_network("192.168.1.0/24")
    """
    network = ipaddress.ip_network(subnet, strict=False)
    hosts = [str(h) for h in network.hosts()]
    cameras: list[DiscoveredCamera] = []

    # Исключаем broadcast и network адреса
    total = len(hosts)
    logger.info("Scanning %d hosts in %s...", total, subnet)

    semaphore = asyncio.Semaphore(max_concurrent)

    async def check_one(ip: str) -> Optional[DiscoveredCamera]:
        async with semaphore:
            return await discover_camera_at_ip(ip, timeout)

    tasks = [check_one(ip) for ip in hosts]
    results = await asyncio.gather(*tasks)

    for res in results:
        if res is not None:
            cameras.append(res)
            logger.info("Found OpenIPC camera at %s", res.ip_address)

    logger.info("Scan complete: %d cameras found", len(cameras))
    return cameras


async def scan_common_ports(ip: str, timeout: float = 1.0) -> dict[str, bool]:
    """
    Сканирует типичные порты камеры для дополнительной информации.
    """
    ports = {80: "HTTP", 443: "HTTPS", 554: "RTSP", 22: "SSH", 8554: "WebRTC", 1984: "go2rtc"}
    results: dict[str, bool] = {}

    for port, name in ports.items():
        try:
            _, writer = await asyncio.wait_for(
                asyncio.open_connection(ip, port),
                timeout=timeout,
            )
            writer.close()
            await writer.wait_closed()
            results[name] = True
        except Exception:
            results[name] = False

    return results


async def quick_scan(ip: str) -> Optional[dict]:
    """
    Быстрый скан одного IP — возвращает всю доступную инфу.
    Используется для быстрой проверки при добавлении камеры.
    """
    cam = await discover_camera_at_ip(ip, timeout=3.0)
    if not cam:
        return None

    ports = await scan_common_ports(ip)

    return {
        "ip": ip,
        "model": cam.model,
        "firmware": cam.firmware,
        "rtsp_main_url": cam.rtsp_main_url,
        "rtsp_sub_url": cam.rtsp_sub_url,
        "rtsp_url": cam.rtsp_main_url,  # обратная совместимость
        "snapshot_url": cam.snapshot_url,
        "config": cam.majestic_config,
        "ports": ports,
        "is_online": cam.is_online,
    }


def get_local_subnet() -> str:
    """
    Определяет локальную подсеть для сканирования.
    Например, если IP = 192.168.1.107, возвращает 192.168.1.0/24.
    """
    try:
        hostname = socket.gethostname()
        local_ip = socket.gethostbyname(hostname)

        # Альтернативный метод: через создание UDP сокета
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        try:
            s.connect(("8.8.8.8", 80))
            local_ip = s.getsockname()[0]
        finally:
            s.close()

        parts = local_ip.split(".")
        if len(parts) == 4:
            return f"{parts[0]}.{parts[1]}.{parts[2]}.0/24"
    except Exception:
        pass

    return "192.168.1.0/24"