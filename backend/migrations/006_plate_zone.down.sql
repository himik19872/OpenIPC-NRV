-- Откат миграции 006: удаление настроек области и точности распознавания номеров.

DROP INDEX IF EXISTS idx_detection_settings_plate_zone;

ALTER TABLE detection_settings
    DROP COLUMN IF EXISTS plate_zone,
    DROP COLUMN IF EXISTS plate_min_length,
    DROP COLUMN IF EXISTS plate_max_length,
    DROP COLUMN IF EXISTS plate_pattern,
    DROP COLUMN IF EXISTS plate_min_confidence;

UPDATE server_settings
SET value = value - 'validate_format' - 'min_length' - 'max_length'
WHERE key = 'plate_recognition';
