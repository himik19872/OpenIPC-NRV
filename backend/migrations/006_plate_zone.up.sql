-- Этап 6: настройка области и точности распознавания номеров.
--
-- Проблема: морфологический поиск номера находит любой похожий прямоугольник
-- с текстом — в кадр попадало OSD-меню камеры (логотип «COMOTO» и подобное),
-- и такие «номера» тысячами писались в события.
--
-- Решение из трёх частей:
--   1. plate_zone — зона на кадре, где искать номера (рисуется как линия);
--   2. правила проверки формата (длина, шаблон) — отсекают мусор вроде COMOTO;
--   3. отдельный порог уверенности OCR для номеров.

-- Зона поиска номеров: полигон в нормализованных координатах (0..1).
-- Пустой массив означает «искать по всему кадру» (прежнее поведение).
ALTER TABLE detection_settings
    ADD COLUMN IF NOT EXISTS plate_zone JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Минимальная длина номера в символах. Российские номера — 8-9 символов,
-- короткие строки почти всегда мусор OCR.
ALTER TABLE detection_settings
    ADD COLUMN IF NOT EXISTS plate_min_length INTEGER NOT NULL DEFAULT 8;

-- Максимальная длина: защита от склейки нескольких областей в одну строку.
ALTER TABLE detection_settings
    ADD COLUMN IF NOT EXISTS plate_max_length INTEGER NOT NULL DEFAULT 12;

-- Шаблон допустимых символов в виде регулярного выражения (POSIX).
-- Пустая строка означает «не проверять».
-- Значение по умолчанию покрывает российский формат: буква-3цифры-2буквы-регион.
ALTER TABLE detection_settings
    ADD COLUMN IF NOT EXISTS plate_pattern VARCHAR(200) NOT NULL DEFAULT '';

-- Минимальная уверенность OCR для сохранения номера.
-- В detection_settings отдельный порог, потому что tesseract и YOLO
-- измеряют уверенность по-разному, и общий min_confidence не подходит.
ALTER TABLE detection_settings
    ADD COLUMN IF NOT EXISTS plate_min_confidence REAL NOT NULL DEFAULT 0.3;

CREATE INDEX IF NOT EXISTS idx_detection_settings_plate_zone
    ON detection_settings((plate_zone <> '[]'::jsonb))
    WHERE plate_zone <> '[]'::jsonb;

-- Настройки распознавания номеров: добавляем общий переключатель проверки
-- формата. Он нужен, чтобы можно было отключить фильтрацию для тех стран,
-- чей формат не похож на российский.
UPDATE server_settings
SET value = value || '{"validate_format": true, "min_length": 8, "max_length": 12}'::jsonb
WHERE key = 'plate_recognition'
  AND NOT (value ? 'validate_format');
