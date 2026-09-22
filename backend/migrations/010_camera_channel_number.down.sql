DROP INDEX IF EXISTS idx_cameras_channel_number;
ALTER TABLE cameras DROP COLUMN IF EXISTS channel_number;
