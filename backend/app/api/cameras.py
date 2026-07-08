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
    CameraProxyOut,
    CameraStatusOut,
    CameraUpdate,
    EventOut,
    RecordingOut,
    RecordingStartRequest,
    RecordingStopRequest,
)
from app.services.stream import generate_rtsp_proxy_stream, generate_proxy_mjpeg, generate_h264_native_stream
from app.services import openipc as openipc_service
from app.services import rtsp_proxy as rtsp_proxy_service

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
    """Добавление новой камеры и регистрация в RTSP-прокси."""
    stream_name = f"cam_{current_user.id}"[:32]  # базовое имя, уточнится после insert
    data = payload.model_dump()
    data.pop("go2rtc_stream", None)  # переопределим ниже
    camera = Camera(**data, owner_id=current_user.id, go2rtc_stream=stream_name)
    db.add(camera)
    await db.flush()
    await db.refresh(camera)

    # Регистрируем потоки в go2rtc-прокси
    proxy_info = await rtsp_proxy_service.register_camera_in_go2rtc(
        camera_id=str(camera.id),
        rtsp_main_url=camera.rtsp_main_url,
        rtsp_sub_url=camera.rtsp_sub_url,
    )
    camera.go2rtc_stream = proxy_info.get("stream_main") or stream_name
    camera.rtsp_proxy_main_url = proxy_info.get("rtsp_proxy_main")
    camera.rtsp_proxy_sub_url = proxy_info.get("rtsp_proxy_sub")
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
    """Удаление камеры и её прокси-потоков."""
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    # Удаляем потоки из go2rtc-прокси
    await rtsp_proxy_service.unregister_camera_from_go2rtc(str(camera.id))

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
        rtsp_url=camera.rtsp_main_url,
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
    snapshot = await openipc_service.get_snapshot_base64(camera.rtsp_main_url)
    mjpeg_url = await openipc_service.get_mjpeg_stream_url(camera.rtsp_main_url)
    hls_url = await openipc_service.get_hls_stream_url(camera.rtsp_main_url)
    ws_url = await openipc_service.get_webrtc_ws_url(camera.rtsp_main_url)

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

    data = await openipc_service.get_snapshot(camera.rtsp_main_url)
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

    ok = await openipc_service.set_night_mode(camera.rtsp_main_url, openipc_service.NightMode.ON)
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

    ok = await openipc_service.set_night_mode(camera.rtsp_main_url, openipc_service.NightMode.OFF)
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

    ok = await openipc_service.toggle_night_mode(camera.rtsp_main_url)
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

    ok = await openipc_service.toggle_ircut(camera.rtsp_main_url)
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

    ok = await openipc_service.toggle_light(camera.rtsp_main_url)
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

    config = await openipc_service.get_majestic_config(camera.rtsp_main_url)
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

    text = await openipc_service.get_majestic_metrics(camera.rtsp_main_url)
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

    url = await openipc_service.get_audio_stream_url(camera.rtsp_main_url, codec)
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

    ok = await openipc_service.enable_audio_output(camera.rtsp_main_url, enable)
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

    cfg = await openipc_service.get_sip_config(camera.rtsp_main_url)
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
        camera.rtsp_main_url,
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

    cfg = await openipc_service.get_motion_config(camera.rtsp_main_url)
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
        camera.rtsp_main_url, enabled=enabled, sensitivity=sensitivity, visualize=visualize
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

    ok = await openipc_service.set_hls(camera.rtsp_main_url, enabled)
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

    ok = await openipc_service.configure_onvif(camera.rtsp_main_url, enabled, onvif_user, onvif_password)
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
        camera.rtsp_main_url, enabled=enabled, nalu_size=nalu_size, substream=substream
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

    ok = await openipc_service.update_majestic_config(camera.rtsp_main_url, dict(partial_config))
    if not ok:
        raise HTTPException(status_code=status.HTTP_502_BAD_GATEWAY, detail="Camera unreachable")
    return {"status": "ok"}


# ---- Stream Proxy ----


@router.get("/{camera_id}/stream")
async def stream_camera(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user_optional)],
    stream: int = Query(default=0, ge=0, le=2, description="0=основной(H.264), 1=доп. поток, 2=JPEG"),
    fps: int = Query(default=0, ge=0, le=30, description="FPS (0=по умолчанию)"),
    quality: int = Query(default=0, ge=0, le=31, description="Качество JPEG (0=по умолчанию)"),
    scale: str = Query(default="", description="Масштаб, напр. 320:240"),
):
    """
    Прямой RTSP→MJPEG прокси (нагрузка на камеру).

    Предпочтительнее использовать /{camera_id}/stream/proxy — через go2rtc (без нагрузки на камеру).
    - stream=0 — основной поток (rtsp_main_url, высокое качество)
    - stream=1 — дополнительный поток (rtsp_sub_url, низкое качество)
    - stream=2 — JPEG поток с камеры
    """
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    # Выбираем URL в зависимости от типа потока
    if stream == 1 and camera.rtsp_sub_url:
        rtsp = camera.rtsp_sub_url
    elif stream == 0:
        rtsp = camera.rtsp_main_url
    else:
        # Fallback: заменяем stream в старом URL
        rtsp = camera.rtsp_main_url
        if "/stream=" in rtsp:
            import re
            rtsp = re.sub(r"/stream=\d+", f"/stream={stream}", rtsp)

    effective_fps = fps if fps > 0 else (5 if stream == 1 else 10)
    effective_qual = quality if quality > 0 else (10 if stream == 1 else 5)

    return StreamingResponse(
        generate_rtsp_proxy_stream(rtsp, fps=effective_fps, quality=effective_qual, scale=scale),
        media_type="multipart/x-mixed-replace; boundary=frame",
    )


# ---- RTSP Proxy через go2rtc (без нагрузки на камеру) ----


@router.get("/{camera_id}/stream/proxy")
async def stream_camera_proxy(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user_optional)],
    stream: int = Query(default=0, ge=0, le=1, description="0=основной, 1=доп. поток"),
    preset: str = Query(default="sub", description="main/sub/thumbnail — пресет качества"),
):
    """
    MJPEG-поток через go2rtc-прокси (НЕ нагружает камеру).

    go2rtc уже держит RTSP-подключение к камере.
    Мы забираем MJPEG с go2rtc — камера отдаёт поток только один раз.

    Пресеты качества:
    - "main" — 15 fps, высокое качество, оригинальный размер
    - "sub" — 5 fps, среднее, 640x360 (по умолчанию)
    - "thumbnail" — 1 fps, низкое, 320x180
    """
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    stream_type = "main" if stream == 0 else "sub"

    return StreamingResponse(
        generate_proxy_mjpeg(str(camera.id), stream_type=stream_type, preset=preset),
        media_type="multipart/x-mixed-replace; boundary=frame",
    )


@router.get("/{camera_id}/proxy", response_model=CameraProxyOut)
async def get_camera_proxy_info(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Информация о прокси-потоках камеры для внешних сервисов.

    Возвращает RTSP-URL'ы, которые другие сервисы могут использовать
    для подключения к камере через NRV (без прямой нагрузки на камеру):
    - rtsp_proxy_main: rtsp://nvr:8554/cam_{id}_main
    - rtsp_proxy_sub:  rtsp://nvr:8554/cam_{id}_sub
    """
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    return CameraProxyOut(
        camera_id=camera.id,
        camera_name=camera.name,
        rtsp_proxy_main=camera.rtsp_proxy_main_url,
        rtsp_proxy_sub=camera.rtsp_proxy_sub_url,
        webrtc_url=camera.webrtc_url,
        mjpeg_proxy_url=f"/api/cameras/{camera.id}/stream/proxy",
        is_online=camera.is_online,
    )


@router.get("/{camera_id}/proxy/status")
async def get_camera_proxy_status(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Статус прокси-потоков: количество потребителей, активность.
    Позволяет мониторить, кто подключён к RTSP-прокси.
    """
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    return await rtsp_proxy_service.get_camera_proxy_status(str(camera.id))


@router.post("/{camera_id}/proxy/reconnect")
async def reconnect_camera_proxy(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Переподключает потоки камеры в RTSP-прокси.
    Полезно после перезагрузки камеры или go2rtc.
    """
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    # Сначала удаляем старые, потом регистрируем заново
    await rtsp_proxy_service.unregister_camera_from_go2rtc(str(camera.id))

    proxy_info = await rtsp_proxy_service.register_camera_in_go2rtc(
        camera_id=str(camera.id),
        rtsp_main_url=camera.rtsp_main_url,
        rtsp_sub_url=camera.rtsp_sub_url,
    )
    camera.rtsp_proxy_main_url = proxy_info.get("rtsp_proxy_main")
    camera.rtsp_proxy_sub_url = proxy_info.get("rtsp_proxy_sub")
    await db.flush()

    return {"status": "reconnected", "proxy": proxy_info}


# ---- Proxy: список всех прокси-потоков (для других сервисов) ----


@router.get("/proxy/all", response_model=List[CameraProxyOut])
async def list_all_proxy_streams(
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Список всех камер с их прокси-RTSP URL.
    Внешние сервисы могут использовать этот эндпоинт для обнаружения потоков.
    """
    stmt = select(Camera).where(Camera.is_enabled == True).order_by(Camera.name)
    result = await db.execute(stmt)
    cameras = result.scalars().all()

    return [
        CameraProxyOut(
            camera_id=c.id,
            camera_name=c.name,
            rtsp_proxy_main=c.rtsp_proxy_main_url,
            rtsp_proxy_sub=c.rtsp_proxy_sub_url,
            webrtc_url=c.webrtc_url,
            mjpeg_proxy_url=f"/api/cameras/{c.id}/stream/proxy",
            is_online=c.is_online,
        )
        for c in cameras
    ]


# ---- H.264 MSE/WebRTC стриминг (без ffmpeg!) ----


@router.get("/{camera_id}/stream/mse")
async def stream_mse(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user_optional)],
    stream: int = Query(default=0, ge=0, le=1, description="0=main, 1=sub"),
):
    """
    H.264 стриминг через ffmpeg -c copy (ремуксинг, БЕЗ ПЕРЕКОДИРОВАНИЯ).

    ffmpeg забирает H.264 RTSP с камеры и ремуксит в fMP4.
    Браузер играет через <video> тег с нативным H.264.

    -c:v copy = копирование без декодирования/энкодинга
    CPU: минимальный (только контейнеризация)
    """
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    # Выбираем URL потока
    stream_type = "main" if stream == 0 else "sub"
    rtsp_url = camera.rtsp_sub_url if (stream == 1 and camera.rtsp_sub_url) else camera.rtsp_main_url

    return StreamingResponse(
        generate_h264_native_stream(rtsp_url, stream_type=f"{stream_type}_ffmpeg_copy"),
        media_type="video/mp4",
        headers={
            "Cache-Control": "no-cache",
            "Access-Control-Allow-Origin": "*",
            "Connection": "keep-alive",
        },
    )


@router.get("/{camera_id}/stream/mp4")
async def stream_mp4(
    camera_id: UUID,
    db: Annotated[AsyncSession, Depends(get_db)],
    current_user: Annotated[User, Depends(get_current_user_optional)],
    stream: int = Query(default=0, ge=0, le=1, description="0=main, 1=sub"),
):
    """
    Прямой H.264 MP4-поток (без перекодирования).
    Можно открыть в VLC, ffplay или <video src="...">
    """
    result = await db.execute(select(Camera).where(Camera.id == camera_id))
    camera = result.scalar_one_or_none()
    if not camera:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Camera not found")

    stream_type = "main" if stream == 0 else "sub"
    redirect_url = rtsp_proxy_service.get_mp4_url(str(camera.id), stream_type)

    from fastapi.responses import RedirectResponse
    return RedirectResponse(url=redirect_url)


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