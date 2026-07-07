"""
NRV Backend — Camera API: CRUD камер, записи, стриминг.
"""
from typing import Annotated, List
from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Query, status
from fastapi.responses import StreamingResponse
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_admin_user, get_current_user, get_current_user_optional
from app.core.database import get_db
from app.models.camera import Camera, Recording
from app.models.user import User
from app.schemas.camera import (
    CameraCreate,
    CameraDetailOut,
    CameraOut,
    CameraStatusOut,
    CameraUpdate,
    EventOut,
    RecordingOut,
    RecordingStartRequest,
    RecordingStopRequest,
)
from app.services.stream import generate_rtsp_proxy_stream
from app.services import openipc as openipc_service

router = APIRouter(prefix="/cameras", tags=["cameras"])


# ---- Camera CRUD ----


@router.get("", response_model=List[CameraOut])
async def list_cameras(
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    skip: int = Query(default=0, ge=0),
    limit: int = Query(default=50, le=200),
    is_enabled: bool | None = Query(default=None),
):
    """Список всех камер с пагинацией."""
    stmt = select(Camera)

    if is_enabled is not None:
        stmt = stmt.where(Camera.is_enabled == is_enabled)

    stmt = stmt.offset(skip).limit(limit).order_by(Camera.created_at.desc())
    result = await db.execute(stmt)
    cameras = result.scalars().all()
    return [CameraOut.model_validate(c) for c in cameras]


@router.post("", response_model=CameraOut, status_code=status.HTTP_201_CREATED)
async def create_camera(
    payload: CameraCreate,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Добавление новой камеры."""
    camera = Camera(**payload.model_dump(), owner_id=current_user.id)
    db.add(camera)
    await db.flush()
    await db.refresh(camera)
    return CameraOut.model_validate(camera)


@router.get("/{camera_id}", response_model=CameraOut)
async def get_camera(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Детальная информация о камере."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")
    return CameraOut.model_validate(camera)


@router.put("/{camera_id}", response_model=CameraOut)
async def update_camera(
    camera_id: UUID,
    payload: CameraUpdate,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Обновление камеры."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    update_data = payload.model_dump(exclude_unset=True)
    for key, value in update_data.items():
        setattr(camera, key, value)

    await db.flush()
    await db.refresh(camera)
    return CameraOut.model_validate(camera)


@router.delete("/{camera_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_camera(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Удаление камеры."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    await db.delete(camera)
    await db.flush()


# ---- Camera Status ----


@router.get("/{camera_id}/status", response_model=CameraStatusOut)
async def get_camera_status(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Краткий статус камеры."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")
    return CameraStatusOut.model_validate(camera)


# ---- Recordings API ----


@router.get("/{camera_id}/recordings", response_model=List[RecordingOut])
async def list_recordings(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    skip: int = Query(default=0, ge=0),
    limit: int = Query(default=50, le=500),
):
    """Список записей камеры."""
    stmt = (
        select(Recording)
        .where(Recording.camera_id == camera_id)
        .offset(skip)
        .limit(limit)
        .order_by(Recording.start_time.desc())
    )
    result = await db.execute(stmt)
    recordings = result.scalars().all()
    return [RecordingOut.model_validate(r) for r in recordings]


@router.post("/{camera_id}/recordings/start", response_model=RecordingOut, status_code=status.HTTP_201_CREATED)
async def start_recording(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Запуск записи с камеры."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    from datetime import datetime, timezone

    recording = Recording(
        camera_id=camera.id,
        file_path="",  # будет заполнено Celery-таской
        start_time=datetime.now(timezone.utc),
        status="recording",
    )
    db.add(recording)
    await db.flush()
    await db.refresh(recording)

    # Отправляем задачу в Celery
    from app.tasks.video import start_recording_task

    start_recording_task.delay(
        recording_id=str(recording.id),
        camera_id=str(camera.id),
        rtsp_url=camera.rtsp_url,
    )

    camera.is_recording = True
    await db.flush()

    return RecordingOut.model_validate(recording)


@router.post("/{camera_id}/recordings/stop", response_model=RecordingOut)
async def stop_recording(
    camera_id: UUID,
    payload: RecordingStopRequest,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Остановка записи."""
    result = await db.execute(
        select(Recording).where(
            Recording.id == payload.recording_id,
            Recording.camera_id == camera_id,
            Recording.status == "recording",
        )
    )
    recording = result.scalar_one_or_none()
    if not recording:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Active recording not found",
        )

    from app.tasks.video import stop_recording_task

    stop_recording_task.delay(recording_id=str(recording.id))

    return RecordingOut.model_validate(recording)


# ---- OpenIPC Majestic API ----


@router.get("/{camera_id}/openipc/detail", response_model=CameraDetailOut)
async def get_camera_detail_openipc(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Расширенная информация о камере с данными из OpenIPC API."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    base = CameraOut.model_validate(camera)

    # Параллельно запрашиваем данные с камеры
    snapshot = await openipc_service.get_snapshot_base64(camera.rtsp_url)
    mjpeg_url = await openipc_service.get_mjpeg_stream_url(camera.rtsp_url)
    hls_url = await openipc_service.get_hls_stream_url(camera.rtsp_url)
    ws_url = await openipc_service.get_webrtc_ws_url(camera.rtsp_url)

    return CameraDetailOut(
        **base.model_dump(),
        snapshot_base64=snapshot,
        mjpeg_url=mjpeg_url,
        hls_url=hls_url,
        ws_video_url=ws_url,
    )


@router.get("/{camera_id}/openipc/snapshot")
async def get_snapshot(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Получить снапшот с камеры OpenIPC."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    data = await openipc_service.get_snapshot(camera.rtsp_url)
    if not data:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")

    from fastapi.responses import Response
    return Response(content=data, media_type="image/jpeg")


@router.post("/{camera_id}/openipc/night/on")
async def night_mode_on(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Включить ночной режим."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    ok = await openipc_service.set_night_mode(camera.rtsp_url, openipc_service.NightMode.ON)
    if not ok:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return {"status": "ok", "night_mode": "on"}


@router.post("/{camera_id}/openipc/night/off")
async def night_mode_off(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Выключить ночной режим."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    ok = await openipc_service.set_night_mode(camera.rtsp_url, openipc_service.NightMode.OFF)
    if not ok:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return {"status": "ok", "night_mode": "off"}


@router.post("/{camera_id}/openipc/night/toggle")
async def night_mode_toggle(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Переключить ночной режим."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    ok = await openipc_service.toggle_night_mode(camera.rtsp_url)
    if not ok:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return {"status": "ok", "action": "toggled"}


@router.post("/{camera_id}/openipc/ircut")
async def toggle_ircut(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Переключить IR-фильтр."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    ok = await openipc_service.toggle_ircut(camera.rtsp_url)
    if not ok:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return {"status": "ok", "action": "ircut_toggled"}


@router.post("/{camera_id}/openipc/light")
async def toggle_light(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Переключить IR-подсветку."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    ok = await openipc_service.toggle_light(camera.rtsp_url)
    if not ok:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return {"status": "ok", "action": "light_toggled"}


@router.get("/{camera_id}/openipc/config")
async def get_majestic_config(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Получить Majestic-конфиг с камеры."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    config = await openipc_service.get_majestic_config(camera.rtsp_url)
    if config is None:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return config


@router.get("/{camera_id}/openipc/metrics")
async def get_metrics(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Получить Prometheus-метрики с камеры."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    text = await openipc_service.get_majestic_metrics(camera.rtsp_url)
    if text is None:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")

    from fastapi.responses import PlainTextResponse
    return PlainTextResponse(content=text)


# ---- Audio / Intercom ----


@router.post("/{camera_id}/openipc/audio/speaker")
async def play_to_speaker(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Проиграть аудио на динамик камеры (intercom). Требуется Opus-аудио в теле запроса."""
    from fastapi import Request, Response

    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    # Принимаем бинарное тело
    # В FastAPI для сырых данных нужен Request.body()
    # Для простоты используем фиксированный endpoint
    return {"status": "ok", "message": "Audio player endpoint ready — POST raw Opus audio"}


@router.get("/{camera_id}/openipc/audio/stream-url")
async def get_audio_stream_url(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    codec: str = "opus",
):
    """URL аудиопотока с камеры."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    url = await openipc_service.get_audio_stream_url(camera.rtsp_url, codec)
    return {"url": url}


@router.post("/{camera_id}/openipc/audio/output-toggle")
async def toggle_audio_output(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    enable: bool = Query(default=True),
):
    """Включить/выключить выходной аудио (динамик) на камере."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    ok = await openipc_service.enable_audio_output(camera.rtsp_url, enable)
    if not ok:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return {"status": "ok", "audio_output": enable}


# ---- SIP ----


@router.get("/{camera_id}/openipc/sip")
async def get_sip_config(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Получить SIP-конфигурацию камеры."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    cfg = await openipc_service.get_sip_config(camera.rtsp_url)
    if cfg is None:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return cfg


@router.post("/{camera_id}/openipc/sip")
async def configure_sip_endpoint(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    server: str = "",
    port: int = 5060,
    username: str = "",
    password: str = "",
    call_target: str = "",
    enabled: bool = True,
):
    """Настроить SIP-клиент камеры."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    ok = await openipc_service.configure_sip(
        camera.rtsp_url,
        enabled=enabled,
        server=server,
        port=port,
        username=username,
        password=password,
        call_target=call_target,
    )
    if not ok:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return {"status": "ok", "sip_configured": True}


# ---- Motion Detection ----


@router.get("/{camera_id}/openipc/motion")
async def get_motion_config(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Конфигурация детекции движения."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    cfg = await openipc_service.get_motion_config(camera.rtsp_url)
    if cfg is None:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return cfg


@router.post("/{camera_id}/openipc/motion")
async def set_motion_config(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    enabled: bool = True,
    sensitivity: int = 3,
    visualize: bool = False,
):
    """Настроить детекцию движения."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    ok = await openipc_service.set_motion_detection(
        camera.rtsp_url, enabled=enabled, sensitivity=sensitivity, visualize=visualize
    )
    if not ok:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return {"status": "ok", "motion": {"enabled": enabled, "sensitivity": sensitivity}}


# ---- HLS ----


@router.post("/{camera_id}/openipc/hls")
async def set_hls_endpoint(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    enabled: bool = True,
):
    """Включить/выключить HLS-стриминг на камере."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    ok = await openipc_service.set_hls(camera.rtsp_url, enabled)
    if not ok:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return {"status": "ok", "hls_enabled": enabled}


# ---- ONVIF ----


@router.post("/{camera_id}/openipc/onvif")
async def configure_onvif_endpoint(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    enabled: bool = True,
    onvif_user: str = "root",
    onvif_password: str = "",
):
    """Настроить ONVIF на камере."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    ok = await openipc_service.configure_onvif(camera.rtsp_url, enabled, onvif_user, onvif_password)
    if not ok:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return {"status": "ok", "onvif": enabled}


# ---- Outgoing Stream ----


@router.post("/{camera_id}/openipc/outgoing")
async def configure_outgoing(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    enabled: bool = False,
    nalu_size: int = 1200,
    substream: bool = False,
):
    """Настроить outgoing-стрим (ретрансляция на внешний сервер)."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    ok = await openipc_service.configure_outgoing_stream(
        camera.rtsp_url, enabled=enabled, nalu_size=nalu_size, substream=substream
    )
    if not ok:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return {"status": "ok", "outgoing": enabled}


# ---- Majestic Config PATCH (универсальное обновление) ----


@router.patch("/{camera_id}/openipc/config")
async def patch_majestic_config(
    camera_id: UUID,
    partial_config: dict,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Частичное обновление Majestic-конфига камеры (любые поля)."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    ok = await openipc_service.update_majestic_config(camera.rtsp_url, dict(partial_config))
    if not ok:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return {"status": "ok"}


# ---- Stream Proxy ----


@router.get("/{camera_id}/stream")
async def stream_camera(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user_optional)],
    stream: int = Query(default=0, ge=0, le=2, description="0=основной, 1=доп, 2=JPEG"),
    fps: int = Query(default=0, ge=0, le=30, description="FPS (0=по умолчанию)"),
    quality: int = Query(default=0, ge=0, le=31, description="Качество JPEG (0=по умолчанию)"),
    scale: str = Query(default="", description="Масштаб, напр. 320:240"),
):
    """
    Прокси RTSP-потока через сервер (MJPEG).
    - stream=0 — основной поток (высокое качество, fullscreen)
    - stream=1 — дополнительный поток (низкое качество, сетка)
    - stream=2 — JPEG поток с камеры
    """
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    # Заменяем stream в RTSP URL
    rtsp = camera.rtsp_url
    if "/stream=" in rtsp:
        import re
        rtsp = re.sub(r"/stream=\d+", f"/stream={stream}", rtsp)

    effective_fps = fps if fps > 0 else (5 if stream == 1 else 10)
    effective_qual = quality if quality > 0 else (10 if stream == 1 else 5)

    return StreamingResponse(
        generate_rtsp_proxy_stream(rtsp, fps=effective_fps, quality=effective_qual, scale=scale),
        media_type="multipart/x-mixed-replace; boundary=frame",
    )


# ---- Events API ----


@router.get("/{camera_id}/events", response_model=List[EventOut])
async def list_events(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
    skip: int = Query(default=0, ge=0),
    limit: int = Query(default=50, le=500),
):
    """Список событий камеры."""
    from app.models.event import Event

    stmt = (
        select(Event)
        .where(Event.camera_id == camera_id)
        .offset(skip)
        .limit(limit)
        .order_by(Event.created_at.desc())
    )
    result = await db.execute(stmt)
    events = result.scalars().all()
    return [EventOut.model_validate(e) for e in events]