-- Откат миграции 002: удаление полей двухпоточной модели OpenIPC.

ALTER TABLE cameras DROP COLUMN IF EXISTS firmware;
ALTER TABLE cameras DROP COLUMN IF EXISTS mac;
ALTER TABLE cameras DROP COLUMN IF EXISTS ip;
ALTER TABLE cameras DROP COLUMN IF EXISTS sub_stream;
ALTER TABLE cameras DROP COLUMN IF EXISTS main_stream;
