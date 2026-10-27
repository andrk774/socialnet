-- Сборы сообществ: цель в монетах, срок, прогресс. Пожертвование — обычная
-- проводка кошелька на счёт сообщества, привязанная к сбору.

CREATE TABLE fundraisers (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    community_id BIGINT      NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    created_by   BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    title        TEXT        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
    description  TEXT        NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
    goal         BIGINT      NOT NULL CHECK (goal BETWEEN 1 AND 10000000),
    deadline     TIMESTAMPTZ NOT NULL,
    closed_at    TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX fundraisers_community_idx ON fundraisers (community_id, id DESC);

CREATE TABLE fundraiser_donations (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    fundraiser_id BIGINT      NOT NULL REFERENCES fundraisers(id) ON DELETE CASCADE,
    tx_id         BIGINT      NOT NULL UNIQUE REFERENCES wallet_transactions(id),
    user_id       BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    amount        BIGINT      NOT NULL CHECK (amount > 0),
    anonymous     BOOLEAN     NOT NULL DEFAULT false,
    message       TEXT        NOT NULL DEFAULT '' CHECK (char_length(message) <= 200),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX fundraiser_donations_idx ON fundraiser_donations (fundraiser_id, id DESC);
