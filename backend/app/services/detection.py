# NRV Backend — Сервис детекции объектов с GPU
# Использует OpenCV + DNN с CUDA ускорением

"""
NRV Backend — Сервис детекции объектов с GPU.
Использует OpenCV + DNN с CUDA ускорением.
"""

import logging
from pathlib import Path
from typing import List, Optional, Tuple
from dataclasses import dataclass
import numpy as np
import cv2

from app.core.config import get_settings

logger = logging.getLogger(__name__)
settings = get_settings()


@dataclass
class DetectedObject:
    """Обнаруженный объект с координатами и классом."""
    class_name: str
    confidence: float
    bbox: Tuple[int, int, int, int]  # x, y, w, h
    timestamp: str


class ObjectDetector:
    """Детектор объектов с поддержкой CUDA."""
    
    # Предобученные классы COCO
    COCO_CLASSES = [
        'person', 'bicycle', 'car', 'motorcycle', 'airplane', 'bus', 'train', 'truck', 'boat',
        'traffic light', 'fire hydrant', 'stop sign', 'parking meter', 'bench', 'bird', 'cat',
        'dog', 'horse', 'sheep', 'cow', 'elephant', 'bear', 'zebra', 'giraffe', 'backpack',
        'umbrella', 'handbag', 'tie', 'suitcase', 'frisbee', 'skis', 'snowboard', 'sports ball',
        'kite', 'baseball bat', 'baseball glove', 'skateboard', 'surfboard', 'tennis racket',
        'bottle', 'wine glass', 'cup', 'fork', 'knife', 'spoon', 'bowl', 'banana', 'apple',
        'sandwich', 'orange', 'broccoli', 'carrot', 'hot dog', 'pizza', 'donut', 'cake', 'chair',
        'couch', 'potted plant', 'bed', 'dining table', 'toilet', 'tv', 'laptop', 'mouse',
        'remote', 'keyboard', 'cell phone', 'microwave', 'oven', 'toaster', 'sink',
        'refrigerator', 'book', 'clock', 'vase', 'scissors', 'teddy bear', 'hair drier', 'toothbrush'
    ]
    
    # Классы, которые нас интересуют
    TARGET_CLASSES = {'person', 'car', 'truck', 'bus', 'motorcycle', 'bicycle', 'dog', 'cat'}
    
    def __init__(self, confidence_threshold: float = 0.5):
        """
        Инициализация детектора с CUDA.
        
        Args:
            confidence_threshold: Минимальная уверенность для обнаружения
        """
        self.confidence_threshold = confidence_threshold
        self.model = None
        self.device = self._init_cuda()
        
    def _init_cuda(self) -> str:
        """Инициализация CUDA и выбор устройства."""
        # Проверка наличия CUDA в OpenCV
        try:
            gpu_count = cv2.cuda.getCudaEnabledDeviceCount()
            logger.info(f"OpenCV CUDA devices: {gpu_count}")
            
            if gpu_count > 0:
                cv2.cuda.setDevice(0)
                logger.info("✓ OpenCV с CUDA ускорением готов")
                return "cuda"
        except Exception as e:
            logger.warning(f"OpenCV CUDA не доступен: {e}")
        
        # Проверка PyTorch CUDA
        try:
            import torch
            if torch.cuda.is_available():
                logger.info(f"✓ PyTorch CUDA доступен: {torch.cuda.get_device_name(0)}")
                return "pytorch"
        except ImportError:
            logger.warning("PyTorch не установлен")
        except Exception as e:
            logger.warning(f"PyTorch CUDA ошибка: {e}")
        
        logger.warning("⚠ Используется CPU (без ускорения)")
        return "cpu"
    
    def load_model(self, model_path: Optional[str] = None) -> None:
        """
        Загрузка YOLO модели.
        
        Args:
            model_path: Путь к модели (.weights + .cfg) или None для авто-загрузки
        """
        if model_path:
            # Загрузка модели из файлов
            config_path = str(Path(model_path).with_suffix('.cfg'))
            weights_path = model_path
            
            self.model = cv2.dnn_DetectionModel(weights_path, config_path)
            self.model.setInputSize(416, 416)
            self.model.setInputScale(1.0 / 255.0)
            self.model.setInputSwapRB(True)
            
            if self.device == "cuda":
                self.model.setPreferableBackend(cv2.dnn.DNN_BACKEND_CUDA)
                self.model.setPreferableTarget(cv2.dnn.DNN_TARGET_CUDA)
            
            logger.info(f"YOLO модель загружена: {model_path}")
        else:
            # Используем Ultralytics YOLO (авто-загрузка)
            self._load_yolo_ultralytics()
    
    def _load_yolo_ultralytics(self) -> None:
        """Загрузка YOLO модели через Ultralytics."""
        try:
            from ultralytics import YOLO
            
            # Загрузка предобученной модели YOLOv8n (быстрая)
            self.model = YOLO('yolov8n.pt')
            
            if self.device == "pytorch" and hasattr(self.model, 'to'):
                self.model.to('cuda')
            
            logger.info("YOLOv8 модель загружена (Ultralytics)")
            
        except ImportError:
            logger.error("Ultralytics не установлен. Установите: pip install ultralytics")
            raise
        except Exception as e:
            logger.error(f"Ошибка загрузки YOLO: {e}")
            raise
    
    def detect(self, frame: np.ndarray) -> List[DetectedObject]:
        """
        Обнаружение объектов на кадре.
        
        Args:
            frame: BGR изображение (numpy array)
            
        Returns:
            Список обнаруженных объектов
        """
        if self.model is None:
            self.load_model()
        
        if self.device == "cuda":
            return self._detect_opencv_cuda(frame)
        else:
            return self._detect_default(frame)
    
    def _detect_opencv_cuda(self, frame: np.ndarray) -> List[DetectedObject]:
        """Детекция с использованием OpenCV CUDA."""
        try:
            # Препарация изображения
            blob = cv2.dnn.blobFromImage(frame, 1/255.0, (416, 416), swapRB=True, crop=False)
            
            # Перенос на GPU
            gpu_blob = cv2.cuda_GpuMat()
            gpu_blob.upload(blob)
            
            # Инференс
            self.model.set_input_blob(gpu_blob)
            outputs = self.model.forward()
            
            # Обработка результатов
            return self._process_detections(outputs, frame.shape)
            
        except Exception as e:
            logger.error(f"OpenCV CUDA детекция ошибка: {e}")
            return self._detect_default(frame)
    
    def _detect_default(self, frame: np.ndarray) -> List[DetectedObject]:
        """Детекция по умолчанию (CPU или PyTorch)."""
        try:
            # Ultralytics YOLO
            results = self.model(frame, conf=self.confidence_threshold, classes=[0, 1, 2, 3, 5, 15, 16, 17])
            
            detections = []
            for result in results:
                for box in result.boxes:
                    cls = int(box.cls[0])
                    conf = float(box.conf[0])
                    xyxy = box.xyxy[0].cpu().numpy()
                    
                    x1, y1, x2, y2 = map(int, xyxy)
                    w, h = x2 - x1, y2 - y1
                    
                    class_name = self.COCO_CLASSES[cls]
                    
                    detections.append(DetectedObject(
                        class_name=class_name,
                        confidence=conf,
                        bbox=(x1, y1, w, h),
                        timestamp=""
                    ))
            
            return detections
            
        except Exception as e:
            logger.error(f"Детекция ошибка: {e}")
            return []
    
    def _process_detections(self, outputs: np.ndarray, frame_shape: tuple) -> List[DetectedObject]:
        """Обработка выходов YOLO."""
        detections = []
        height, width = frame_shape[:2]
        
        for output in outputs:
            for detection in output:
                scores = detection[5:]
                class_id = np.argmax(scores)
                confidence = scores[class_id]
                
                if confidence > self.confidence_threshold:
                    class_name = self.COCO_CLASSES[class_id]
                    
                    # Координаты
                    center_x = int(detection[0] * width)
                    center_y = int(detection[1] * height)
                    w = int(detection[2] * width)
                    h = int(detection[3] * height)
                    x = center_x - w // 2
                    y = center_y - h // 2
                    
                    detections.append(DetectedObject(
                        class_name=class_name,
                        confidence=float(confidence),
                        bbox=(x, y, w, h),
                        timestamp=""
                    ))
        
        return detections
    
    def draw_detections(self, frame: np.ndarray, detections: List[DetectedObject]) -> np.ndarray:
        """
        Рисование рамок на кадре.
        
        Args:
            frame: BGR изображение
            detections: Список обнаруженных объектов
            
        Returns:
            Кадр с нарисованными рамками
        """
        result = frame.copy()
        
        for det in detections:
            x, y, w, h = det.bbox
            
            # Цвет по классу
            color = (0, 255, 0)  # Зеленый по умолчанию
            if det.class_name == 'person':
                color = (255, 0, 0)  # Синий для людей
            elif det.class_name in ('car', 'truck', 'bus'):
                color = (0, 255, 255)  # Желтый для машин
            
            # Рамка
            cv2.rectangle(result, (x, y), (x + w, y + h), color, 2)
            
            # Текст
            label = f"{det.class_name}: {det.confidence:.2f}"
            cv2.putText(result, label, (x, y - 10), 
                       cv2.FONT_HERSHEY_SIMPLEX, 0.5, color, 2)
        
        return result


# Глобальный экземпляр детектора
_detector: Optional[ObjectDetector] = None


def get_detector() -> ObjectDetector:
    """Получение глобального экземпляра детектора."""
    global _detector
    if _detector is None:
        _detector = ObjectDetector()
    return _detector


def detect_objects_from_file(file_path: str) -> List[DetectedObject]:
    """
    Детекция объектов из видеофайла.
    
    Args:
        file_path: Путь к видеофайлу
        
    Returns:
        Список всех обнаруженных объектов
    """
    detector = get_detector()
    detections = []
    
    cap = cv2.VideoCapture(file_path)
    try:
        while cap.isOpened():
            ret, frame = cap.read()
            if not ret:
                break
            
            frame_detections = detector.detect(frame)
            detections.extend(frame_detections)
    
    finally:
        cap.release()
    
    return detections


def detect_objects_from_rtsp(rtsp_url: str, max_frames: int = 100) -> List[DetectedObject]:
    """
    Детекция объектов из RTSP стрима.
    
    Args:
        rtsp_url: URL RTSP стрима
        max_frames: Максимальное количество кадров для анализа
        
    Returns:
        Список обнаруженных объектов
    """
    detector = get_detector()
    detections = []
    
    cap = cv2.VideoCapture(rtsp_url)
    try:
        frame_count = 0
        while cap.isOpened() and frame_count < max_frames:
            ret, frame = cap.read()
            if not ret:
                break
            
            frame_detections = detector.detect(frame)
            detections.extend(frame_detections)
            frame_count += 1
    
    finally:
        cap.release()
    
    return detections
