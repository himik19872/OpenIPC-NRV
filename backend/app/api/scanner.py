"""
NRV Backend — API сканера сети: автообнаружение камер OpenIPC.
"""
from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, Query, status

from app.api.deps import get_current_user
from app.models.user import User
from app.services.scanner import (
    discover_camera_at_ip,
    get_local_subnet,
    quick_scan,
    scan_network,
)

router = APIRouter(prefix="/scanner", tags=["scanner"])


@router.get("/subnet")
async def get_subnet():
    """Возвращает локальную подсеть, определённую автоматически."""
    return {"subnet": get_local_subnet()}


@router.post("/scan")
async def scan(
    subnet: str = Query(default="", description="Подсеть для сканирования, например 192.168.1.0/24"),
    current_user: Annotated[User, Depends(get_current_user)] = None,
):
    """
    Сканирует сеть на наличие камер OpenIPC.
    Если subnet не указан, сканирует текущую подсеть.
    """
    if not subnet:
        subnet = get_local_subnet()

    cameras = await scan_network(subnet)
    return {
        "subnet": subnet,
        "found": len(cameras),
        "cameras": [c.to_dict() for c in cameras],
    }


@router.get("/check/{ip}")
async def check_ip(
    ip: str,
    current_user: Annotated[User, Depends(get_current_user)] = None,
):
    """Быстрая проверка одного IP на наличие камеры OpenIPC."""
    info = await quick_scan(ip)
    if info is None:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="No OpenIPC camera found at this IP",
        )
    return info


@router.post("/add-found/{ip}")
async def add_found_camera(
    ip: str,
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Находит камеру по IP и автоматически добавляет её в систему.
    Возвращает данные созданной камеры.
    """
    from sqlalchemy import select
    from sqlalchemy.ext.asyncio import AsyncSession

    from app.core.database import get_db
    from app.models.camera import Camera
    from app.schemas.camera import CameraOut

    cam_info = await quick_scan(ip)
    if cam_info is None:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="No OpenIPC camera found at this IP",
        )

    # Проверяем, нет ли уже такой камеры
    # get_db через Depends — используем create_async_engine напрямую
    from app.core.database import async_session_factory

    async with async_session_factory() as db:
        existing = await db.execute(
            select(Camera).where(Camera.rtsp_main_url == cam_info["rtsp_main_url"])
        )
        if existing.scalar_one_or_none():
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail="Camera with this RTSP URL already exists",
            )

        camera = Camera(
            name=f"OpenIPC {cam_info['model']} ({ip})",
            rtsp_main_url=cam_info["rtsp_main_url"],
            rtsp_sub_url=cam_info.get("rtsp_sub_url"),
            manufacturer=cam_info["model"],
            firmware=cam_info["firmware"],
            location=f"Auto-discovered at {ip}",
            owner_id=current_user.id,
            is_online=True,
        )
        db.add(camera)
        await db.commit()
        await db.refresh(camera)
        return CameraOut.model_validate(camera)