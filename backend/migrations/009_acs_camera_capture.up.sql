-- 009: привязка камеры к контроллеру СКУД и съёмка по событиям доступа.
--
-- Камера, смотрящая на считыватель и дверь, позволяет подтвердить, кто и
-- как прошёл. Один контроллер обслуживает один проём (одна дверь, один
-- замок), поэтому камера привязана к контроллеру напрямую, а не через
-- отдельную таблицу связей.
--
-- Настройки съёмки хранятся в JSONB: набор событий, по которым нужно
-- снимать, зависит от вендора контроллера и может расширяться, а
-- отдельные колонки под каждый триггер сделали бы схему жёсткой.

ALTER TABLE acs_controllers
    -- Камера, наблюдающая за проёмом. NULL — съёмка по событиям отключена.
    -- ON DELETE SET NULL: удаление камеры не должно ломать контроллер.
    ADD COLUMN IF NOT EXISTS camera_id uuid REFERENCES cameras(id) ON DELETE SET NULL,

    -- capture_mode: off — не снимать, snapshot — один кадр,
    -- clip — короткое видео. Снимок дешевле и достаточно для фотофиксации,
    -- видео нужно, когда важно само действие (например, проход).
    ADD COLUMN IF NOT EXISTS capture_mode varchar(16) NOT NULL DEFAULT 'off',

    -- capture_events: события доступа, по которым срабатывает съёмка.
    -- Пустой массив при capture_mode != off означает «снимать на все
    -- события»: так проще всего включить фотофиксацию сразу.
    ADD COLUMN IF NOT EXISTS capture_events jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- clip_seconds: длительность клипа. Ограничена разумными рамками,
    -- чтобы запись не превратилась в круглосуточную.
    ADD COLUMN IF NOT EXISTS clip_seconds integer NOT NULL DEFAULT 5;

ALTER TABLE acs_controllers
    ADD CONSTRAINT acs_controllers_capture_mode_check
        CHECK (capture_mode IN ('off', 'snapshot', 'clip')),
    ADD CONSTRAINT acs_controllers_clip_seconds_check
        CHECK (clip_seconds BETWEEN 1 AND 60);

-- Съёмка по событию доступа оставляет ссылку на файл в самом событии:
-- так в журнале СКУД видно, что именно было снято. Отдельного поля для
-- видео не хватало — snapshot_path хранит путь и для клипа.
ALTER TABLE acs_events
    -- Тип снятого материала: snapshot или clip. NULL — съёмки не было.
    ADD COLUMN IF NOT EXISTS media_type varchar(16),
    -- Идентификатор записи в архиве: связывает событие доступа с записью,
    -- чтобы из журнала СКУД можно было открыть видео.
    ADD COLUMN IF NOT EXISTS recording_id uuid;

ALTER TABLE acs_events
    ADD CONSTRAINT acs_events_media_type_check
        CHECK (media_type IS NULL OR media_type IN ('snapshot', 'clip'));

CREATE INDEX IF NOT EXISTS idx_acs_events_recording
    ON acs_events (recording_id) WHERE recording_id IS NOT NULL;

COMMENT ON COLUMN acs_controllers.capture_mode IS 'off | snapshot | clip';
COMMENT ON COLUMN acs_controllers.capture_events IS 'JSON-массив событий доступа; пустой — все события';
COMMENT ON COLUMN acs_events.media_type IS 'snapshot | clip, NULL — съёмки не было';
