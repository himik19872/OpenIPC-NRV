-- Этап 5: распознавание лиц и автомобильных номеров.
--
-- Добавляются:
--   1. known_faces   — справочник известных лиц (эталонные снимки/эмбеддинги);
--   2. known_plates  — справочник известных номеров авто;
--   3. связанные поля и триггеры для событий и записей.

-- ---------------------------------------------------------------------------
-- Справочник известных лиц.
--
-- Храним и эмбеддинг, и эталонный снимок:
--   * эмбеддинг нужен для сравнения (быстро, не зависит от ракурса);
--   * снимок нужен человеку — чтобы понять, кто это, прямо в интерфейсе.
-- Эмбеддинг — массив float (512 чисел для ArcFace). Тип REAL[] выбран вместо
-- vector из pgvector: лишнее расширение не требуется, а поиск ближайшего
-- соседа здесь не нужен — лиц в списке десятки, сравниваем полным перебором.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS known_faces (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         VARCHAR(200) NOT NULL,
    -- Произвольная заметка: должность, отдел, комментарий оператора
    note         TEXT NOT NULL DEFAULT '',
    -- Эталонный снимок лица: minio:<key> или local:<abspath>
    photo_path   VARCHAR(512),
    -- Эмбеддинг лица; NULL означает «зарегистрировано, но без биометрии» —
    -- такое лицо в распознавании не участвует, но видно в списке.
    embedding    REAL[],
    -- Черный список: событие с таким лицом помечается как тревожное
    is_blocked   BOOLEAN NOT NULL DEFAULT false,
    enabled      BOOLEAN NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_known_faces_enabled
    ON known_faces(enabled) WHERE enabled = true;

-- ---------------------------------------------------------------------------
-- Справочник известных автомобильных номеров.
--
-- Номер храним в двух видах:
--   * plate      — как распознала система (например, «А123ВС77»);
--   * plate_norm — нормализованный (только буквы и цифры, верхний регистр).
-- Сравнение идёт по нормализованному виду, т.к. OCR может вернуть лишние
-- пробелы, дефисы или разный регистр букв.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS known_plates (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plate       VARCHAR(20) NOT NULL,
    plate_norm  VARCHAR(20) NOT NULL,
    -- Владелец, описание авто, комментарий
    owner       VARCHAR(200) NOT NULL DEFAULT '',
    note        TEXT NOT NULL DEFAULT '',
    -- Фото автомобиля (необязательно)
    photo_path  VARCHAR(512),
    is_blocked  BOOLEAN NOT NULL DEFAULT false,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Номер должен быть уникален: иначе непонятно, какая запись соответствует
-- распознанному знаку.
CREATE UNIQUE INDEX IF NOT EXISTS idx_known_plates_norm
    ON known_plates(plate_norm);

CREATE INDEX IF NOT EXISTS idx_known_plates_enabled
    ON known_plates(enabled) WHERE enabled = true;

-- ---------------------------------------------------------------------------
-- Расширение таблицы событий: результат распознавания.
--
-- match_type описывает, чем закончилось сравнение со справочником:
--   * unknown   — ни лица, ни номера в справочнике нет;
--   * known     — найден в справочнике (не заблокирован);
--   * blocked   — найден и помечен как заблокированный.
-- Отдельная колонка нужна для быстрого фильтра в интерфейсе: искать
-- по JSONB заметно медленнее.
-- ---------------------------------------------------------------------------
ALTER TABLE detection_events
    ADD COLUMN IF NOT EXISTS match_type VARCHAR(20) NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS matched_id UUID,
    ADD COLUMN IF NOT EXISTS matched_name VARCHAR(200);

CREATE INDEX IF NOT EXISTS idx_detection_events_match
    ON detection_events(match_type) WHERE match_type <> 'unknown';

-- ---------------------------------------------------------------------------
-- Расширение таблицы записей: причина, по которой запись была создана.
--
-- Без этого в архиве видно только «запись по событию», и непонятно, что
-- именно её вызвало. trigger_type хранит тип триггера, trigger_detail —
-- расшифровку (класс объекта, имя человека, номер автомобиля).
-- ---------------------------------------------------------------------------
ALTER TABLE recordings
    ADD COLUMN IF NOT EXISTS trigger_type VARCHAR(20) NOT NULL DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS trigger_detail VARCHAR(200) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_recordings_trigger
    ON recordings(trigger_type);

-- ---------------------------------------------------------------------------
-- Настройки распознавания для камеры.
--
-- Хранятся отдельными ключами в server_settings, чтобы не расширять
-- detection_settings: настройки распознавания общие для всей системы,
-- а не задаются для каждой камеры отдельно.
-- ---------------------------------------------------------------------------
INSERT INTO server_settings (key, value) VALUES
    ('face_recognition', '{
        "enabled": false,
        "threshold": 0.45,
        "snapshot_unknown": true,
        "alert_blocked": true
    }'::jsonb),
    ('plate_recognition', '{
        "enabled": false,
        "threshold": 0.75,
        "region": "ru",
        "snapshot_unknown": true,
        "alert_blocked": true
    }'::jsonb)
ON CONFLICT (key) DO NOTHING;
