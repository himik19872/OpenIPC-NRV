-- Откат миграции 001: удаление начальной схемы.
-- Порядок обратен созданию: сначала зависимые таблицы,
-- затем те, на которые они ссылаются.

DROP TABLE IF EXISTS wireguard_peers;
DROP TABLE IF EXISTS push_tokens;
DROP TABLE IF EXISTS recordings;
DROP TABLE IF EXISTS acs_events;
DROP TABLE IF EXISTS acs_controllers;
DROP TABLE IF EXISTS detection_events;
DROP TABLE IF EXISTS cameras;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS sites;
