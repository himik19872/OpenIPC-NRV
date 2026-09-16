"""AI Detector — YOLOv8 + NATS. Subscribes cameras.*.frame, publishes cameras.*.detection."""

import os, json, time, asyncio, signal, sys, logging
import cv2, numpy as np
from ultralytics import YOLO
from nats.aio.client import Client as NATS
from nats.aio.errors import ErrTimeout

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger("ai-detector")


class AIDetector:
    def __init__(self, model_path="yolov8n.pt", device="cpu", conf=0.4, iou=0.5):
        self.conf, self.iou = conf, iou
        logger.info(f"Loading {model_path} on {device}...")
        self.model = YOLO(model_path)
        self.model(np.zeros((640, 640, 3), dtype=np.uint8), verbose=False)
        logger.info(f"Model ready ({len(self.model.names)} classes)")

    def detect(self, image_bytes, camera_id, frame_ts):
        nparr = np.frombuffer(image_bytes, np.uint8)
        img = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
        if img is None:
            return []
        results = self.model.track(img, persist=True, conf=self.conf, iou=self.iou,
                                   device=self.model.device, verbose=False)
        events = []
        if results and results[0].boxes:
            boxes = results[0].boxes
            for i in range(len(boxes)):
                cls_id = int(boxes.cls[i].item())
                conf = float(boxes.conf[i].item())
                cls_name = self.model.names.get(cls_id, f"class_{cls_id}")
                xyxy = boxes.xyxy[i].tolist()
                bbox = {"x": xyxy[0], "y": xyxy[1], "w": xyxy[2] - xyxy[0], "h": xyxy[3] - xyxy[1]}
                track_id = int(boxes.id[i].item()) if boxes.id is not None else None
                events.append({"camera_id": camera_id, "timestamp": frame_ts,
                               "object_class": cls_name, "confidence": round(conf, 4),
                               "bbox": bbox, "track_id": track_id})
        return events


async def main():
    nats_url = os.getenv("NATS_URL", "nats://localhost:4222")
    model_path = os.getenv("MODEL_PATH", "yolov8n.pt")
    device = os.getenv("DEVICE", "cpu")
    conf = float(os.getenv("CONFIDENCE", "0.4"))

    detector = AIDetector(model_path=model_path, device=device, conf=conf)
    nc = NATS()
    await nc.connect(nats_url)
    logger.info(f"Connected to NATS at {nats_url}")

    sub = await nc.subscribe("cameras.*.frame")
    logger.info("Subscribed to cameras.*.frame")

    async def handle_frame(msg):
        payload = msg.data
        if len(payload) < 40:
            return
        camera_id = payload[:36].decode("ascii", errors="ignore").strip()
        frame_data = payload[36:]
        events = detector.detect(frame_data, camera_id, time.time())
        for ev in events:
            await nc.publish(f"cameras.{camera_id}.detection", json.dumps(ev).encode())
        if events:
            logger.info(f"[{camera_id[:8]}] Detected: {[e['object_class'] for e in events]}")

    async def worker():
        while True:
            try:
                msg = await sub.next_msg(timeout=1)
                if msg:
                    await handle_frame(msg)
            except ErrTimeout:
                continue
            except Exception as e:
                logger.error(f"Worker error: {e}")
                await asyncio.sleep(1)

    worker_task = asyncio.create_task(worker())
    logger.info("AI Detector running. Ctrl+C to stop.")
    try:
        await worker_task
    except asyncio.CancelledError:
        pass
    finally:
        await nc.close()
        logger.info("AI Detector stopped")


if __name__ == "__main__":
    signal.signal(signal.SIGINT, lambda s, f: sys.exit(0))
    signal.signal(signal.SIGTERM, lambda s, f: sys.exit(0))
    asyncio.run(main())