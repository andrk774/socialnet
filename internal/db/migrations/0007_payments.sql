-- Покупка монет за рубли через ЮKassa.
-- Монеты начисляются ровно один раз: строка платежа блокируется, а
-- транзакция кошелька идёт с ключом идемпотентности pay:<id>.
CREATE TABLE payments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),  -- наш номер заказа и Idempotence-Key
    user_id             BIGINT      NOT NULL REFERENCES users(id),
    provider            TEXT        NOT NULL DEFAULT 'yookassa',
    provider_payment_id TEXT        UNIQUE,
    package_id          TEXT        NOT NULL,
    coins               BIGINT      NOT NULL CHECK (coins > 0),
    amount_kopecks      BIGINT      NOT NULL CHECK (amount_kopecks > 0),
    currency            TEXT        NOT NULL DEFAULT 'RUB',
    status              TEXT        NOT NULL DEFAULT 'created'
                        CHECK (status IN ('created', 'pending', 'succeeded', 'canceled', 'error')),
    confirmation_url    TEXT,
    receipt_email       TEXT        NOT NULL,
    transaction_id      BIGINT      REFERENCES wallet_transactions(id),
    error               TEXT        NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((status = 'succeeded') = (transaction_id IS NOT NULL))
);
CREATE INDEX payments_user_idx ON payments (user_id, created_at DESC);
