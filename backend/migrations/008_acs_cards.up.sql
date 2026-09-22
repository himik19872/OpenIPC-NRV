-- 008: карты доступа СКУД.
--
-- Сервер выступает центральным хранилищем карт: контроллеры получают базу
-- от него, а не наоборот (op_mode=1 в прошивке контроллера). Поэтому карты
-- живут на сервере и выдаются на устройства, а не читаются с них.
--
-- Ключ карты — пара (facility, card). Она приходит из Wiegand-кода и не
-- меняется при переезде карты с одного контроллера на другой, поэтому
-- уникальность задана именно по ней, а не по суррогатному id.

CREATE TABLE IF NOT EXISTS acs_cards (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    controller_id uuid REFERENCES acs_controllers(id) ON DELETE CASCADE,
    facility      integer NOT NULL,
    card          integer NOT NULL,
    name          varchar(128) NOT NULL DEFAULT '',
    grp           varchar(64) NOT NULL DEFAULT '',
    -- access: 0 — постоянный доступ, 1 — только по расписанию.
    access        integer NOT NULL DEFAULT 0,
    active        boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),

    -- Ограничения Wiegand-26 из прошивки контроллера: facility — 8 бит,
    -- card — 16 бит. Проверка на уровне БД не даёт завести карту, которую
    -- контроллер молча обрежет.
    CONSTRAINT acs_cards_facility_range CHECK (facility BETWEEN 0 AND 255),
    CONSTRAINT acs_cards_card_range     CHECK (card BETWEEN 0 AND 65535)
);

-- Одна и та же карта не может быть заведена на одном контроллере дважды.
CREATE UNIQUE INDEX IF NOT EXISTS idx_acs_cards_unique
    ON acs_cards (controller_id, facility, card);

-- Поиск по номеру карты при разборе события доступа.
CREATE INDEX IF NOT EXISTS idx_acs_cards_lookup
    ON acs_cards (facility, card);

-- Список карт контроллера.
CREATE INDEX IF NOT EXISTS idx_acs_cards_controller
    ON acs_cards (controller_id);

-- Группа задаётся отдельной колонкой grp, а не group: group — служебное
-- слово SQL, и его использование требует кавычек в каждом запросе.
COMMENT ON COLUMN acs_cards.grp IS 'группа доступа (group — зарезервированное слово)';
COMMENT ON COLUMN acs_cards.access IS '0 — постоянный доступ, 1 — по расписанию';
