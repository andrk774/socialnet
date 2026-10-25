-- Устойчивый идентификатор сессии: при ротации refresh-токена он переходит
-- к новому токену. Его же содержит access-токен (claim sid), поэтому
-- завершённая сессия перестаёт работать сразу, а не через 15 минут.
ALTER TABLE refresh_tokens
    ADD COLUMN session_id UUID NOT NULL DEFAULT gen_random_uuid();

CREATE INDEX refresh_tokens_session_idx ON refresh_tokens (session_id) WHERE revoked_at IS NULL;

-- Чем отличается «заменён при ротации» от «отозван вручную»:
-- повторное предъявление заменённого токена = признак кражи (отзываем всё),
-- а отозванный выходом или «завершить сеанс» — просто недействителен.
ALTER TABLE refresh_tokens
    ADD COLUMN replaced_at TIMESTAMPTZ;
