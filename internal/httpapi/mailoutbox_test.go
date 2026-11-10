package httpapi

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"socialnet/internal/mailer"
)

func TestMailRetryIn(t *testing.T) {
	for attempts, want := range map[int]time.Duration{
		1: 30 * time.Second, 2: time.Minute, 3: 2 * time.Minute, 5: 8 * time.Minute, 6: 15 * time.Minute, 40: 15 * time.Minute,
	} {
		if got := mailRetryIn(attempts); got != want {
			t.Errorf("mailRetryIn(%d) = %v, want %v", attempts, got, want)
		}
	}
}

// flakyMailer отказывает первые fail раз — как недоступный SMTP.
type flakyMailer struct {
	mailer.Memory
	mu   sync.Mutex
	fail int
}

func (f *flakyMailer) Send(ctx context.Context, m mailer.Message) error {
	f.mu.Lock()
	if f.fail > 0 {
		f.fail--
		f.mu.Unlock()
		return errors.New("smtp: connection refused")
	}
	f.mu.Unlock()
	return f.Memory.Send(ctx, m)
}

// Письмо из очереди переживает сбой SMTP: попытка повторяется, пока не пройдёт;
// безнадёжно старое письмо бросается, а не повторяется вечно.
func TestMailOutboxRetries(t *testing.T) {
	_, srv, _ := setupFull(t, 1000)
	ctx := t.Context()
	flaky := &flakyMailer{fail: 1}
	srv.mailer = flaky
	pool := srv.store.Pool()
	if _, err := pool.Exec(ctx, `INSERT INTO mail_outbox (to_addr, subject, body) VALUES ('mama@example.com', 'Тревога', 'текст')`); err != nil {
		t.Fatal(err)
	}

	if n, err := srv.DeliverMail(ctx); err != nil || n != 0 {
		t.Fatalf("first attempt: sent %d, err %v", n, err)
	}
	var attempts int
	var lastErr string
	var later bool
	if err := pool.QueryRow(ctx, `SELECT attempts, last_error, next_attempt_at > now() FROM mail_outbox`).
		Scan(&attempts, &lastErr, &later); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || lastErr == "" || !later {
		t.Fatalf("after failure: attempts=%d err=%q retry later=%v", attempts, lastErr, later)
	}
	// до паузы письмо не берётся повторно
	if n, _ := srv.DeliverMail(ctx); n != 0 {
		t.Fatal("retried before the pause")
	}

	if _, err := pool.Exec(ctx, `UPDATE mail_outbox SET next_attempt_at = now()`); err != nil {
		t.Fatal(err)
	}
	if n, err := srv.DeliverMail(ctx); err != nil || n != 1 {
		t.Fatalf("retry: sent %d, err %v", n, err)
	}
	if got := flaky.Sent(); len(got) != 1 || got[0].To != "mama@example.com" {
		t.Fatalf("sent: %v", got)
	}
	if n, _ := srv.DeliverMail(ctx); n != 0 {
		t.Fatal("sent twice")
	}

	// письмо старше суток при сбое бросается
	flaky.mu.Lock()
	flaky.fail = 1
	flaky.mu.Unlock()
	if _, err := pool.Exec(ctx, `INSERT INTO mail_outbox (to_addr, subject, body, created_at)
		VALUES ('old@example.com', 'Тревога', 'текст', now() - interval '25 hours')`); err != nil {
		t.Fatal(err)
	}
	if n, _ := srv.DeliverMail(ctx); n != 0 {
		t.Fatal("old mail sent despite failure")
	}
	var failed bool
	if err := pool.QueryRow(ctx, `SELECT failed_at IS NOT NULL FROM mail_outbox WHERE to_addr = 'old@example.com'`).Scan(&failed); err != nil {
		t.Fatal(err)
	}
	if !failed {
		t.Fatal("old mail not given up")
	}
}
