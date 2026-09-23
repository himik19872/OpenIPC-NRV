-- Откат фильтров точности детекции.

ALTER TABLE detection_settings
    DROP COLUMN IF EXISTS face_requires_person,
    DROP COLUMN IF EXISTS face_min_confidence,
    DROP COLUMN IF EXISTS static_seconds,
    DROP COLUMN IF EXISTS max_aspect_ratio,
    DROP COLUMN IF EXISTS max_object_area,
    DROP COLUMN IF EXISTS min_object_area;

ALTER TABLE detection_settings
    ALTER COLUMN min_confidence SET DEFAULT 0.4;

DROP INDEX IF EXISTS idx_detection_settings_filters;

-- Пороги камер, поднятые миграцией, возвращаем к прежнему значению.
-- Восстановить точный исходный порог нельзя: часть камер могла быть
-- настроена вручную уже после применения миграции.
--
-- Приведение ::real нужно по той же причине, что и в прямой миграции:
-- сравнение real с numeric даёт false при внешне равных значениях.
UPDATE detection_settings
   SET min_confidence = 0.4::real
 WHERE min_confidence = 0.6::real;
