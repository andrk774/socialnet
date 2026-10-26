-- Юридические документы: какую редакцию принял пользователь, журнал согласий,
-- удаление аккаунта (обезличивание) по отзыву согласия на обработку ПДн.

-- 0 — ещё ничего не принимал: существующие пользователи примут документы
-- при следующем входе.
ALTER TABLE users
    ADD COLUMN legal_rev  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN deleted_at TIMESTAMPTZ;

-- Журнал: кто, что, какую редакцию, когда и откуда принял. Нужен, чтобы
-- доказать наличие согласия (ч. 3 ст. 9 152-ФЗ).
CREATE TABLE consent_log (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    document   TEXT        NOT NULL CHECK (document IN ('terms', 'privacy', 'consent', 'age18')),
    version    TEXT        NOT NULL CHECK (char_length(version) BETWEEN 1 AND 40),
    action     TEXT        NOT NULL DEFAULT 'accept' CHECK (action IN ('accept', 'revoke')),
    ip         INET,
    user_agent TEXT        NOT NULL DEFAULT '' CHECK (char_length(user_agent) <= 300),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX consent_log_user_idx ON consent_log (user_id, id DESC);
