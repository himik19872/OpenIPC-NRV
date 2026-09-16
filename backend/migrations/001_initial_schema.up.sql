-- Миграция 001: Начальная схема БД
-- Применяется: golang-migrate или вручную

-- Расширения
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Сайты (объекты)
CREATE TABLE IF NOT EXISTS sites (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    address     TEXT,
    wg_pubkey   VARCHAR(64),
    wg_ip       INET,
    timezone    VARCHAR(50) DEFAULT 'Europe/Moscow',
    created_at  TIMESTAMPTZ DEFAULT now()
);

-- Камеры
CREATE TABLE IF NOT EXISTS cameras (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    rtsp_url    VARCHAR(512) NOT NULL,
    site_id     UUID REFERENCES sites(id) ON DELETE SET NULL,
    wg_ip       INET,
    status      VARCHAR(20) DEFAULT 'offline',
    hw_info     JSONB DEFAULT '{}',
    settings    JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ DEFAULT now(),
    updated_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_cameras_status ON cameras(status);
CREATE INDEX IF NOT EXISTS idx_cameras_site ON cameras(site_id);

-- События AI-детекции
CREATE TABLE IF NOT EXISTS detection_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    camera_id       UUID REFERENCES cameras(id) ON DELETE CASCADE,
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT now(),
    object_class    VARCHAR(50) NOT NULL,
    confidence      REAL NOT NULL DEFAULT 0,
    bbox            JSONB,
    track_id        INTEGER,
    snapshot_path   VARCHAR(512),
    thumbnail_path  VARCHAR(512),
    metadata        JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_detection_events_camera_time
    ON detection_events(camera_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_detection_events_class
    ON detection_events(object_class);
CREATE INDEX IF NOT EXISTS idx_detection_events_timestamp
    ON detection_events(timestamp DESC);

-- Контроллеры СКУД
CREATE TABLE IF NOT EXISTS acs_controllers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    vendor      VARCHAR(50) NOT NULL,
    ip          INET NOT NULL,
    port        INTEGER DEFAULT 80,
    credentials JSONB DEFAULT '{}',
    site_id     UUID REFERENCES sites(id) ON DELETE SET NULL,
    status      VARCHAR(20) DEFAULT 'offline',
    config      JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ DEFAULT now()
);

-- События СКУД
CREATE TABLE IF NOT EXISTS acs_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    controller_id   UUID REFERENCES acs_controllers(id) ON DELETE CASCADE,
    door_id         VARCHAR(100),
    event_type      VARCHAR(50) NOT NULL,
    card_number     VARCHAR(50),
    user_id         UUID,
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT now(),
    camera_id       UUID REFERENCES cameras(id) ON DELETE SET NULL,
    snapshot_path   VARCHAR(512),
    metadata        JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_acs_events_controller_time
    ON acs_events(controller_id, timestamp DESC);

-- Записи видео
CREATE TABLE IF NOT EXISTS recordings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    camera_id       UUID REFERENCES cameras(id) ON DELETE CASCADE,
    start_time      TIMESTAMPTZ NOT NULL,
    end_time        TIMESTAMPTZ NOT NULL,
    file_path       VARCHAR(512) NOT NULL,
    file_size       BIGINT DEFAULT 0,
    resolution      VARCHAR(20),
    codec           VARCHAR(20),
    event_triggered BOOLEAN DEFAULT false,
    metadata        JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_recordings_camera_time
    ON recordings(camera_id, start_time DESC);

-- Пользователи
CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username        VARCHAR(100) UNIQUE NOT NULL,
    password_hash   VARCHAR(255) NOT NULL,
    role            VARCHAR(20) DEFAULT 'operator',
    permissions     JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);

-- Push-токены для мобильных устройств
CREATE TABLE IF NOT EXISTS push_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID REFERENCES users(id) ON DELETE CASCADE,
    token       VARCHAR(512) NOT NULL,
    platform    VARCHAR(10) NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT now()
);

-- WireGuard-пиры
CREATE TABLE IF NOT EXISTS wireguard_peers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id     UUID REFERENCES sites(id) ON DELETE CASCADE,
    pubkey      VARCHAR(64) UNIQUE NOT NULL,
    ip          INET NOT NULL,
    last_seen   TIMESTAMPTZ,
    status      VARCHAR(20) DEFAULT 'offline'
);

-- Admin по умолчанию (пароль: admin123)
-- bcrypt hash: $2a$10$... сгенерировать при первом запуске
-- INSERT INTO users (username, password_hash, role)
-- VALUES ('admin', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'admin')
-- ON CONFLICT (username) DO NOTHING;