-- Очередь писем, которые обязаны дойти (тревога по контрольному сроку
-- похода и отбой). Письмо ставится в очередь в той же транзакции, что и
-- отметка «тревога отправлена», и повторяется, пока SMTP не примет его:
-- сбой почты или перезапуск API во время отправки больше не теряют письмо.
CREATE TABLE mail_outbox (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    to_addr         TEXT        NOT NULL CHECK (char_length(to_addr) BETWEEN 3 AND 254),
    subject         TEXT        NOT NULL CHECK (char_length(subject) <= 300),
    body            TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- когда пробовать снова; на время попытки сдвигается вперёд (аренда),
    -- поэтому несколько экземпляров API не отправят одно письмо дважды подряд
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempts        INTEGER     NOT NULL DEFAULT 0,
    last_error      TEXT        NOT NULL DEFAULT '',
    sent_at         TIMESTAMPTZ,
    failed_at       TIMESTAMPTZ -- попытки исчерпаны
);
CREATE INDEX mail_outbox_due_idx ON mail_outbox (next_attempt_at) WHERE sent_at IS NULL AND failed_at IS NULL;
