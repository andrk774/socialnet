-- Журнал действий модераторов и администраторов и тревоги детектора аномалий.

-- Только дописывание: каждая запись хранит хеш предыдущей (цепочка), так что
-- удаление или правка задним числом обнаруживается проверкой цепочки. Плюс
-- триггер запрещает UPDATE/DELETE/TRUNCATE даже владельцу таблицы.
-- actor_id без внешнего ключа: удаление пользователя не должно менять историю.
CREATE TABLE audit_log (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id   BIGINT      NOT NULL,
    actor_role TEXT        NOT NULL,
    action     TEXT        NOT NULL,  -- «POST /api/v1/admin/wallet/mint»
    target     TEXT        NOT NULL,  -- фактический путь
    details    JSONB,                 -- тело запроса (если JSON)
    status     INT         NOT NULL,
    ip         INET,
    user_agent TEXT        NOT NULL DEFAULT '',
    request_id TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    prev_hash  BYTEA       NOT NULL,
    hash       BYTEA       NOT NULL UNIQUE
);
CREATE INDEX audit_log_actor_idx ON audit_log (actor_id, id DESC);
CREATE INDEX audit_log_created_idx ON audit_log (created_at);

CREATE FUNCTION audit_log_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only';
END $$;
CREATE TRIGGER audit_log_no_update BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_append_only();
CREATE TRIGGER audit_log_no_truncate BEFORE TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION audit_log_append_only();

CREATE TABLE security_alerts (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    rule            TEXT        NOT NULL,
    subject         TEXT        NOT NULL,  -- user:42, ip:1.2.3.4, actor:7:1.2.3.4
    severity        TEXT        NOT NULL CHECK (severity IN ('low', 'medium', 'high')),
    details         JSONB       NOT NULL DEFAULT '{}',
    window_start    TIMESTAMPTZ NOT NULL,  -- одно срабатывание правила на субъект за окно
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    acknowledged_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    acknowledged_at TIMESTAMPTZ,
    UNIQUE (rule, subject, window_start)
);
CREATE INDEX security_alerts_open_idx ON security_alerts (id DESC) WHERE acknowledged_at IS NULL;

-- для правил детектора
CREATE INDEX security_events_recent_idx ON security_events (created_at, type);
CREATE INDEX wallet_tx_recent_idx ON wallet_transactions (created_at, kind);

-- Запуски резервного копирования: для метрик «давно не было удачной копии».
CREATE TABLE backup_runs (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        TEXT        NOT NULL,
    size        BIGINT      NOT NULL DEFAULT 0,
    sha256      TEXT        NOT NULL DEFAULT '',
    encrypted   BOOLEAN     NOT NULL,
    restore_ok  BOOLEAN     NOT NULL,
    error       TEXT        NOT NULL DEFAULT '',
    duration_ms BIGINT      NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
