-- Глобальные настройки сервера (хранилище записей и снимков, глубина архива).
-- Таблица key-value, чтобы добавлять параметры без новых миграций.
CREATE TABLE IF NOT EXISTS server_settings (
    key         VARCHAR(100) PRIMARY KEY,
    value       JSONB NOT NULL DEFAULT '{}',
    updated_at  TIMESTAMPTZ DEFAULT now()
);

-- Настройки детекции для каждой камеры.
-- Вынесены в отдельную таблицу (а не в cameras.settings), т.к. это
-- самостоятельный набор параметров со своей валидацией и частотой изменений.
CREATE TABLE IF NOT EXISTS detection_settings (
    camera_id       UUID PRIMARY KEY REFERENCES cameras(id) ON DELETE CASCADE,
    enabled         BOOLEAN NOT NULL DEFAULT false,
    -- Классы объектов для детекции: person, car, truck, bus, motorcycle, bicycle...
    object_classes  TEXT[]  NOT NULL DEFAULT '{}',
    -- Минимальная уверенность модели (0..1)
    min_confidence  REAL    NOT NULL DEFAULT 0.4,
    -- Типы детекции: object (объекты), line (пересечение линии), face (лица), plate (номера)
    detect_types    TEXT[]  NOT NULL DEFAULT '{object}',
    -- Зона детекции: полигон в нормализованных координатах кадра (0..1).
    -- Пустой массив — вся площадь кадра.
    zone            JSONB   NOT NULL DEFAULT '[]',
    -- Линия для подсчёта пересечений: две точки {x,y} в нормализованных координатах
    line            JSONB   NOT NULL DEFAULT '[]',
    -- Направление учёта пересечений: both, forward, backward
    line_direction  VARCHAR(10) NOT NULL DEFAULT 'both',
    -- Что делать при детекции
    save_snapshots  BOOLEAN NOT NULL DEFAULT true,
    -- Режим записи: off, always (постоянно), event (по детекции)
    record_mode     VARCHAR(10) NOT NULL DEFAULT 'off',
    -- Сколько секунд видео сохранять ДО события (пребуфер)
    prebuffer_sec   INTEGER NOT NULL DEFAULT 10,
    -- Сколько секунд писать ПОСЛЕ события
    postbuffer_sec  INTEGER NOT NULL DEFAULT 20,
    -- Пауза между событиями одного типа (защита от спама), сек
    cooldown_sec    INTEGER NOT NULL DEFAULT 30,
    updated_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_detection_settings_enabled
    ON detection_settings(enabled) WHERE enabled = true;

-- Тестовый набор данных: настройки по умолчанию для уже существующих камер.
INSERT INTO detection_settings (camera_id, enabled, object_classes)
SELECT id, false, ARRAY['person', 'car']
FROM cameras
ON CONFLICT (camera_id) DO NOTHING;

-- Настройки сервера по умолчанию
INSERT INTO server_settings (key, value) VALUES
    ('storage', '{"backend": "minio", "local_path": "/var/lib/nvr/recordings", "retention_days": 30}'),
    ('snapshots', '{"backend": "minio", "local_path": "/var/lib/nvr/snapshots", "retention_days": 90}')
ON CONFLICT (key) DO NOTHING;
