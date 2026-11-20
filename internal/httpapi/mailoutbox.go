package httpapi

import (
	"context"
	"time"

	"socialnet/internal/mailer"
	"socialnet/internal/store"
)

// Доставка писем из очереди mail_outbox (тревоги по контрольному сроку и
// отбой). В отличие от sendMail, письмо не теряется при сбое SMTP или
// перезапуске API: оно повторяется с паузами, пока его не примут.

const (
	mailRetryBase   = 30 * time.Second
	mailRetryMax    = 15 * time.Minute
	mailGiveUpAge   = 24 * time.Hour // тревога старше суток уже никому не поможет
	mailLoudAfter   = 3              // с этой попытки сбой пишется как ошибка (будит дежурного)
	mailBatch       = 20
	mailSendTimeout = 30 * time.Second
	mailKickTimeout = 2 * time.Minute // немедленная попытка после постановки в очередь
)

// mailRetryIn — пауза перед следующей попыткой: 30 с, 1, 2, 4, 8 мин, затем 15 мин.
func mailRetryIn(attempts int) time.Duration {
	d := mailRetryBase
	for i := 1; i < attempts && d < mailRetryMax; i++ {
		d *= 2
	}
	return min(d, mailRetryMax)
}

// DeliverMail отправляет письма, которым пора уходить. Возвращает число
// отправленных; ошибка — только если не удалось работать с очередью.
func (s *Server) DeliverMail(ctx context.Context) (int, error) {
	sent := 0
	for {
		batch, err := s.store.ClaimMails(ctx, mailBatch)
		if err != nil || len(batch) == 0 {
			return sent, err
		}
		for _, m := range batch {
			if s.deliverOne(ctx, m) {
				sent++
			}
		}
		if len(batch) < mailBatch {
			return sent, nil
		}
	}
}

func (s *Server) deliverOne(ctx context.Context, m store.QueuedMail) bool {
	sendCtx, cancel := context.WithTimeout(ctx, mailSendTimeout)
	err := s.sendConfiguredMail(sendCtx, mailer.Message{To: m.To, Subject: m.Subject, Body: m.Body})
	cancel()
	if err == nil {
		if err := s.store.MailSent(ctx, m.ID); err != nil {
			// письмо ушло, но отметка нет: после аренды оно уйдёт повторно —
			// лучше дубль тревоги, чем ни одной
			s.log.Error("mail outbox: mark sent", "err", err, "id", m.ID)
		}
		return true
	}
	giveUp := time.Since(m.CreatedAt) > mailGiveUpAge
	if ferr := s.store.MailFailed(ctx, m.ID, err.Error(), mailRetryIn(m.Attempts), giveUp); ferr != nil {
		s.log.Error("mail outbox: mark failed", "err", ferr, "id", m.ID)
	}
	switch {
	case giveUp:
		s.log.Error("mail outbox: giving up", "err", err, "id", m.ID, "attempts", m.Attempts, "subject", m.Subject)
		s.m.business.Inc("mail_given_up")
	case m.Attempts >= mailLoudAfter:
		s.log.Error("mail outbox: send failed", "err", err, "id", m.ID, "attempts", m.Attempts, "subject", m.Subject)
	default:
		s.log.Warn("mail outbox: send failed, will retry", "err", err, "id", m.ID, "attempts", m.Attempts)
	}
	return false
}

// kickMail — отправить только что поставленные письма сразу, не дожидаясь
// цикла. Несколько одновременных запусков безопасны: письмо берётся с арендой.
func (s *Server) kickMail() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), mailKickTimeout)
		defer cancel()
		if _, err := s.DeliverMail(ctx); err != nil {
			s.log.Error("mail outbox", "err", err)
		}
	}()
}

// runMailOutbox — повторы и уборка очереди (работает до отмены ctx).
func (s *Server) runMailOutbox(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	lastCleanup := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := s.DeliverMail(ctx); err != nil && ctx.Err() == nil {
				s.log.Error("mail outbox", "err", err)
			}
			if time.Since(lastCleanup) > time.Hour {
				if n, err := s.store.CleanupMailOutbox(ctx); err != nil && ctx.Err() == nil {
					s.log.Error("mail outbox cleanup", "err", err)
				} else if n > 0 {
					s.log.Info("mail outbox cleaned", "count", n)
				}
				lastCleanup = time.Now()
			}
		}
	}
}
