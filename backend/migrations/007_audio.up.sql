-- Этап 7: звук с камер и подготовка к детекции аудио.
--
-- Задача состоит из двух частей:
--   1. СЛУШАТЬ камеру — в кадре должен быть звук;
--   2. ГОВОРИТЬ в камеру — двусторонняя связь через динамик камеры.
--
-- Проблема первой части: камеры отдают звук в G.711 (PCMU/PCMA) или Opus.
-- G.711 несовместим с HLS — браузер его в HLS не воспроизведёт, нужен AAC.
-- Поэтому для камер с G.711 включается транскодирование аудио отдельным
-- процессом ffmpeg, а результат публикуется в MediaMTX как отдельный путь
-- `<cameraID>_audio`.

-- Настройки звука для камеры.
CREATE TABLE IF NOT EXISTS audio_settings (
    camera_id       UUID PRIMARY KEY REFERENCES cameras(id) ON DELETE CASCADE,
    -- Есть ли у камеры микрофон. Если выключено — транскодирование не запускается,
    -- это экономит ресурсы на камерах без звука.
    has_microphone  BOOLEAN NOT NULL DEFAULT true,
    -- Включён ли звук в интерфейсе по умолчанию.
    -- false означает, что плеер стартует без звука (оператор включит сам).
    enabled         BOOLEAN NOT NULL DEFAULT false,
    -- Уровень громкости по умолчанию (0..1) — применяется в плеере.
    volume          REAL NOT NULL DEFAULT 0.7,
    -- Исходный аудиокодек камеры: g711, opus, aac, auto (определить автоматически).
    -- При auto кодирование определяется по треку из MediaMTX.
    source_codec    VARCHAR(20) NOT NULL DEFAULT 'auto',
    -- Нужно ли транскодировать звук в AAC для воспроизведения в браузере.
    -- Для Opus и AAC транскодирование не требуется.
    transcode       BOOLEAN NOT NULL DEFAULT true,
    -- --- Подготовка к детекции звука ---
    -- Включена ли детекция звуковых событий (крик, выстрел, разбитое стекло),
    -- а также распознавание речи.
    detect_audio    BOOLEAN NOT NULL DEFAULT false,
    -- Типы звуковых событий для поиска (классы аудиомодели).
    audio_events    TEXT[] NOT NULL DEFAULT '{}',
    -- Порог уверенности аудиодетекции (0..1).
    audio_threshold REAL NOT NULL DEFAULT 0.5,
    -- --- Двусторонняя связь ---
    -- Разрешена ли передача звука НА камеру (динамик).
    speaker_enabled BOOLEAN NOT NULL DEFAULT false,
    -- Кодек для передачи на камеру: g711, aac. Камеры принимают звук
    -- по RTSP/ONVIF, обычно в G.711.
    speaker_codec   VARCHAR(20) NOT NULL DEFAULT 'g711',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audio_settings_enabled
    ON audio_settings(enabled) WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_audio_settings_mic
    ON audio_settings(has_microphone) WHERE has_microphone = true;

-- Настройки по умолчанию для уже существующих камер.
INSERT INTO audio_settings (camera_id, enabled, has_microphone)
SELECT id, false, true FROM cameras
ON CONFLICT (camera_id) DO NOTHING;

-- Глобальные настройки аудио в key-value хранилище.
INSERT INTO server_settings (key, value) VALUES
    ('audio', '{
        "enabled": true,
        "default_volume": 0.7,
        "autoplay_muted": true,
        "transcode_g711": true,
        "audio_bitrate": "64k",
        "speaker_enabled": false,
        "detect_events": false
    }'::jsonb)
ON CONFLICT (key) DO NOTHING;

-- События аудиодетекции — по аналогии с detection_events, но для звука.
-- Отдельная таблица, потому что у звука нет рамки объекта, зато есть
-- длительность и класс звука (крик, выстрел, лай собаки и т.п.).
CREATE TABLE IF NOT EXISTS audio_events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    camera_id    UUID REFERENCES cameras(id) ON DELETE CASCADE,
    timestamp    TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Класс звука: speech, shout, gunshot, glass_break, scream, dog, alarm...
    event_class  VARCHAR(50) NOT NULL,
    -- Уверенность модели (0..1).
    confidence   REAL NOT NULL DEFAULT 0,
    -- Громкость в дБ относительно шума — помогает отсеять тихие ложные срабатывания.
    loudness_db  REAL,
    -- Длительность звукового фрагмента в секундах.
    duration_sec REAL,
    -- Распознанный текст (для речевых событий; заполняет ASR-модуль).
    transcript   TEXT,
    -- Сохранённый звуковой фрагмент: minio:<key> или local:<path>.
    clip_path    VARCHAR(512),
    metadata     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audio_events_camera_time
    ON audio_events(camera_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audio_events_class
    ON audio_events(event_class);
CREATE INDEX IF NOT EXISTS idx_audio_events_timestamp
    ON audio_events(timestamp DESC);

-- Причина записи «звук» — чтобы событие аудиодетекции было видно в архиве.
UPDATE server_settings
SET value = value || '{"audio": true}'::jsonb
WHERE key = 'storage' AND NOT (value ? 'audio');
