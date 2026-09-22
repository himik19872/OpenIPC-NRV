-- Номер канала для внешнего RTSP-доступа.
--
-- Внешние системы запрашивают поток по адресу вида
--   rtsp://login:password@host:9784/cameras/{N}/streaming/main
-- где N — номер камеры со смещением на минус один: первая камера это 0.
-- Отдельное поле, а не порядковый номер в списке: номера должны оставаться
-- неизменными при удалении и добавлении камер, иначе внешние интеграции
-- начнут получать поток совсем другой камеры.
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS channel_number INTEGER;

-- Номер уникален: два канала с одним номером сделали бы адрес
-- неоднозначным. Частичный индекс: NULL означает «канал не назначен»,
-- и таких камер может быть сколько угодно (они недоступны по RTSP).
CREATE UNIQUE INDEX IF NOT EXISTS idx_cameras_channel_number
    ON cameras (channel_number)
    WHERE channel_number IS NOT NULL;

-- Проставляем номера уже существующим камерам: порядок по дате добавления,
-- начиная с 1. Это разовая операция для текущего парка — дальше номер
-- задаётся в карточке камеры.
WITH numbered AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY created_at, id) AS n
    FROM cameras
    WHERE channel_number IS NULL
)
UPDATE cameras c
SET channel_number = numbered.n
FROM numbered
WHERE c.id = numbered.id;
