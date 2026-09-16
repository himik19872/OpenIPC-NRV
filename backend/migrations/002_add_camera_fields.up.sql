-- Миграция 002: Добавление полей для двухпоточной модели OpenIPC

ALTER TABLE cameras ADD COLUMN IF NOT EXISTS main_stream VARCHAR(512);
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS sub_stream VARCHAR(512);
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS ip VARCHAR(45);
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS mac VARCHAR(17);
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS firmware VARCHAR(100);

COMMENT ON COLUMN cameras.main_stream IS 'RTSP основной поток (stream=0) — запись и полный экран';
COMMENT ON COLUMN cameras.sub_stream IS 'RTSP субпоток (stream=1) — грид и детекция';
COMMENT ON COLUMN cameras.ip IS 'IP-адрес камеры в сети';
COMMENT ON COLUMN cameras.mac IS 'MAC-адрес камеры';
COMMENT ON COLUMN cameras.firmware IS 'Версия прошивки OpenIPC/Majestic';