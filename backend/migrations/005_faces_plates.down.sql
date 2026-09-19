-- Откат миграции 005: удаление справочников лиц и номеров,
-- а также связанных колонок в событиях и записях.

DROP INDEX IF EXISTS idx_recordings_trigger;
ALTER TABLE recordings
    DROP COLUMN IF EXISTS trigger_type,
    DROP COLUMN IF EXISTS trigger_detail;

DROP INDEX IF EXISTS idx_detection_events_match;
ALTER TABLE detection_events
    DROP COLUMN IF EXISTS match_type,
    DROP COLUMN IF EXISTS matched_id,
    DROP COLUMN IF EXISTS matched_name;

DROP TABLE IF EXISTS known_plates;
DROP TABLE IF EXISTS known_faces;

DELETE FROM server_settings WHERE key IN ('face_recognition', 'plate_recognition');
