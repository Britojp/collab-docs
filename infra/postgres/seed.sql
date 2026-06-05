-- Seed inicial do CollabDocs
-- Usuário de teste: admin@collabdocs.dev / admin123
--
-- O hash abaixo é BCrypt(cost=10) de "admin123", compatível com
-- Spring Security BCryptPasswordEncoder e PostgreSQL pgcrypto.
-- Gerado com: SELECT crypt('admin123', gen_salt('bf', 10));
--
-- Idempotente: ON CONFLICT DO NOTHING permite re-executar sem erro.

INSERT INTO users (email, name, password_hash)
VALUES (
    'admin@collabdocs.dev',
    'Admin CollabDocs',
    '$2a$10$gQk3TodONa8A/TmNcw1W1uREvs06yrwIaNr4GzTh.MQzBSjUQ1pbq'
)
ON CONFLICT (email) DO NOTHING;
