-- MP4 для браузеров без встроенного HLS и возвраты платежей.

-- Качество, для которого есть одиночный MP4 (например, '720p'); NULL — нет.
ALTER TABLE videos ADD COLUMN mp4 TEXT CHECK (mp4 IN ('360p', '720p'));

-- ---------- возвраты ЮKassa ----------
ALTER TABLE wallet_transactions DROP CONSTRAINT wallet_transactions_kind_check;
ALTER TABLE wallet_transactions ADD CONSTRAINT wallet_transactions_kind_check
    CHECK (kind IN ('transfer', 'gift', 'donation', 'reward', 'mint', 'purchase', 'payout', 'refund', 'debt'));

CREATE TABLE payment_refunds (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider_refund_id TEXT        NOT NULL UNIQUE,
    payment_id         UUID        NOT NULL REFERENCES payments(id),
    amount_kopecks     BIGINT      NOT NULL CHECK (amount_kopecks > 0),
    coins              BIGINT      NOT NULL CHECK (coins >= 0),  -- сколько монет положено списать
    debited            BIGINT      NOT NULL CHECK (debited >= 0), -- сколько списано сразу
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE payments ADD COLUMN refunded_kopecks BIGINT NOT NULL DEFAULT 0 CHECK (refunded_kopecks >= 0);
ALTER TABLE payments DROP CONSTRAINT payments_status_check;
ALTER TABLE payments ADD CONSTRAINT payments_status_check
    CHECK (status IN ('created', 'pending', 'succeeded', 'canceled', 'error', 'refunded', 'partially_refunded'));
ALTER TABLE payments DROP CONSTRAINT payments_check;
ALTER TABLE payments ADD CONSTRAINT payments_paid_has_tx
    CHECK ((status IN ('succeeded', 'refunded', 'partially_refunded')) = (transaction_id IS NOT NULL));

-- Долг: монеты возвращённой покупки, которые уже потрачены. Пока долг не
-- погашен, тратить монеты нельзя, а новые поступления сначала гасят его.
CREATE TABLE wallet_debts (
    user_id    BIGINT      PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    amount     BIGINT      NOT NULL CHECK (amount > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
