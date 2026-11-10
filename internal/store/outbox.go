package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Очередь писем, которые обязаны дойти (см. миграцию 0030).

// OutboxMail — письмо для очереди.
type OutboxMail struct {
	To, Subject, Body string
}

// QueuedMail — письмо, взятое из очереди на отправку.
type QueuedMail struct {
	ID        int64
	To        string
	Subject   string
	Body      string
	Attempts  int // включая текущую попытку
	CreatedAt time.Time
}

// MailLease — на сколько письмо закрепляется за одной попыткой: столько
// не возьмёт другой экземпляр API (дольше таймаута отправки).
const MailLease = 2 * time.Minute

// enqueueMails ставит письма в очередь внутри транзакции вызывающего:
// письмо появится ровно тогда, когда зафиксируется то, о чём оно сообщает.
func enqueueMails(ctx context.Context, tx pgx.Tx, mails []OutboxMail) error {
	for _, m := range mails {
		if _, err := tx.Exec(ctx, `INSERT INTO mail_outbox (to_addr, subject, body) VALUES ($1, $2, $3)`,
			m.To, truncate(m.Subject, 300), m.Body); err != nil {
			return err
		}
	}
	return nil
}

// ClaimMails берёт письма, которым пора уходить, и закрепляет их за
// вызывающим на MailLease. SKIP LOCKED — экземпляры API не ждут друг друга.
func (s *Store) ClaimMails(ctx context.Context, limit int) ([]QueuedMail, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE mail_outbox SET next_attempt_at = now() + $2::interval, attempts = attempts + 1
		WHERE id IN (
			SELECT id FROM mail_outbox
			WHERE sent_at IS NULL AND failed_at IS NULL AND next_attempt_at <= now()
			ORDER BY next_attempt_at, id LIMIT $1 FOR UPDATE SKIP LOCKED)
		RETURNING id, to_addr, subject, body, attempts, created_at`, limit, MailLease.String())
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (QueuedMail, error) {
		var m QueuedMail
		return m, r.Scan(&m.ID, &m.To, &m.Subject, &m.Body, &m.Attempts, &m.CreatedAt)
	})
}

func (s *Store) MailSent(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE mail_outbox SET sent_at = now(), last_error = '' WHERE id = $1`, id)
	return err
}

// MailFailed: попытка не удалась. giveUp — больше не пытаться, иначе
// повторить через retryIn.
func (s *Store) MailFailed(ctx context.Context, id int64, msg string, retryIn time.Duration, giveUp bool) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE mail_outbox SET last_error = $2,
			next_attempt_at = now() + $3::interval,
			failed_at = CASE WHEN $4 THEN now() END
		WHERE id = $1`, id, truncate(msg, 500), retryIn.String(), giveUp)
	return err
}

// CleanupMailOutbox: письма с адресами доверенных контактов не храним
// дольше нужного — ни отправленные, ни брошенные.
func (s *Store) CleanupMailOutbox(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM mail_outbox WHERE (sent_at IS NOT NULL OR failed_at IS NOT NULL) AND created_at < now() - interval '7 days'`)
	return tag.RowsAffected(), err
}
