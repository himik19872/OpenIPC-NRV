-- Откат миграции 007: удаление настроек и событий аудио.

DROP TABLE IF EXISTS audio_events;
DROP TABLE IF EXISTS audio_settings;

DELETE FROM server_settings WHERE key = 'audio';
