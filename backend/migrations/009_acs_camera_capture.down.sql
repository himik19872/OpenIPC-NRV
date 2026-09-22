ALTER TABLE acs_events
    DROP CONSTRAINT IF EXISTS acs_events_media_type_check,
    DROP COLUMN IF EXISTS media_type,
    DROP COLUMN IF EXISTS recording_id;

DROP INDEX IF EXISTS idx_acs_events_recording;

ALTER TABLE acs_controllers
    DROP CONSTRAINT IF EXISTS acs_controllers_capture_mode_check,
    DROP CONSTRAINT IF EXISTS acs_controllers_clip_seconds_check,
    DROP COLUMN IF EXISTS camera_id,
    DROP COLUMN IF EXISTS capture_mode,
    DROP COLUMN IF EXISTS capture_events,
    DROP COLUMN IF EXISTS clip_seconds;
