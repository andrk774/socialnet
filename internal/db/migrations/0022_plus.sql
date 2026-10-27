-- Подписка «Плюс» за монеты: выше лимиты, значок в профиле, инструменты
-- для клубов. Списание — обычная проводка кошелька (kind = 'subscription').

CREATE TABLE subscriptions (
    user_id    BIGINT      PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    auto_renew BOOLEAN     NOT NULL DEFAULT true,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX subscriptions_renew_idx ON subscriptions (expires_at) WHERE auto_renew;

ALTER TABLE wallet_transactions DROP CONSTRAINT wallet_transactions_kind_check;
ALTER TABLE wallet_transactions ADD CONSTRAINT wallet_transactions_kind_check
    CHECK (kind IN ('transfer', 'gift', 'donation', 'reward', 'mint', 'purchase', 'payout', 'refund', 'debt', 'subscription'));
