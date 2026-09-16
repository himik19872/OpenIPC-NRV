-- Seed: пользователь admin по умолчанию
-- Пароль: admin123 (bcrypt)
-- Применяется после 001_initial_schema.up.sql

INSERT INTO users (username, password_hash, role, permissions)
VALUES ('admin', '$2b$10$xlE7a.fSY0gkwuGWtRFD2eFEzyc7YE742NAMOywjHkQQW9wfCXgj2', 'admin', '{"all": true}')
ON CONFLICT (username) DO NOTHING;