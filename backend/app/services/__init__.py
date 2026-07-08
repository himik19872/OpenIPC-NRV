# NRV Backend — Services: инициализация
"""Инициализация services модуля."""

from app.services.go2rtc import *
from app.services.openipc import *
from app.services.scanner import *
from app.services.stream import *
from app.services.rtsp_proxy import *

# Опционально: компьютерное зрение (требует numpy, opencv-python)
try:
    from app.services.detection import ObjectDetector, detect_objects_from_file, detect_objects_from_rtsp, get_detector
except ImportError:
    ObjectDetector = None  # type: ignore
    detect_objects_from_file = None  # type: ignore
    detect_objects_from_rtsp = None  # type: ignore
    get_detector = None  # type: ignore

__all__ = [
    "ObjectDetector",
    "detect_objects_from_file",
    "detect_objects_from_rtsp",
    "get_detector",
    # go2rtc
    "get_go2rtc_config",
    "create_stream",
    "delete_stream",
    # openipc
    "get_openipc_config",
    "get_snapshot",
    "set_night_mode",
    "toggle_night_mode",
    "get_majestic_config",
    "get_mjpeg_stream_url",
    "get_hls_stream_url",
    "get_webrtc_ws_url",
    "play_audio_to_speaker",
    "enable_audio_output",
    "configure_sip",
    "set_motion_detection",
    "configure_sd_recording",
    "set_hls",
    "configure_onvif",
    "configure_outgoing_stream",
    "update_majestic_config",
    # scanner
    "scan_network",
    "discover_cameras",
    # stream
    "generate_rtsp_proxy_stream",
]
