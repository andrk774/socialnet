package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Payment struct {
	ID                string    `json:"id"`
	UserID            int64     `json:"-"`
	ProviderPaymentID *string   `json:"-"`
	PackageID         string    `json:"package_id"`
	Coins             int64     `json:"coins"`
	AmountKopecks     int64     `json:"amount_kopecks"`
	Currency          string    `json:"currency"`
	Status            string    `json:"status"`
	ConfirmationURL   *string   `json:"confirmation_url,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

const paymentCols = `id::text, user_id, provider_payment_id, package_id, coins, amount_kopecks, currency, status, confirmation_url, created_at`

func scanPayment(row pgx.Row) (*Payment, error) {
	p := &Payment{}
	err := row.Scan(&p.ID, &p.UserID, &p.ProviderPaymentID, &p.PackageID, &p.Coins, &p.AmountKopecks,
		&p.Currency, &p.Status, &p.ConfirmationURL, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// CreatePayment: не больше 5 незавершённых покупок за час — защита от
// накрутки запросов к платёжному шлюзу.
func (s *Store) CreatePayment(ctx context.Context, user int64, pkg string, coins, kopecks int64, email string) (*Payment, error) {
	var n int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM payments WHERE user_id = $1 AND status IN ('created', 'pending')
		AND created_at > now() - interval '1 hour'`, user).Scan(&n); err != nil {
		return nil, err
	}
	if n >= 5 {
		return nil, ErrLimit
	}
	return scanPayment(s.pool.QueryRow(ctx, `
		INSERT INTO payments (user_id, package_id, coins, amount_kopecks, receipt_email)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+paymentCols, user, pkg, coins, kopecks, email))
}

func (s *Store) PaymentCreated(ctx context.Context, id, providerID, confirmationURL string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE payments SET provider_payment_id = $2, confirmation_url = $3, status = 'pending', updated_at = now()
		WHERE id = $1::uuid AND status = 'created'`, id, providerID, confirmationURL)
	return err
}

func (s *Store) PaymentFailed(ctx context.Context, id, msg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE payments SET status = 'error', error = $2, updated_at = now() WHERE id = $1::uuid AND status = 'created'`,
		id, truncate(msg, 500))
	return err
}

func (s *Store) OwnPayment(ctx context.Context, user int64, id string) (*Payment, error) {
	return scanPayment(s.pool.QueryRow(ctx, `SELECT `+paymentCols+` FROM payments WHERE id = $1::uuid AND user_id = $2`, id, user))
}

func (s *Store) PaymentByProviderID(ctx context.Context, providerID string) (*Payment, error) {
	return scanPayment(s.pool.QueryRow(ctx, `SELECT `+paymentCols+` FROM payments WHERE provider_payment_id = $1`, providerID))
}

func (s *Store) MyPayments(ctx context.Context, user int64, limit int) ([]Payment, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+paymentCols+` FROM payments WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`, user, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Payment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// SettlePayment переводит платёж в итоговый статус по данным, уже
// проверенным у ЮKassa. succeeded — начислить монеты (ровно один раз).
// Возвращает true, если монеты начислены именно этим вызовом.
func (s *Store) SettlePayment(ctx context.Context, id, status string) (credited bool, err error) {
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var cur string
		var user, coins int64
		if err := tx.QueryRow(ctx, `SELECT status, user_id, coins FROM payments WHERE id = $1::uuid FOR UPDATE`, id).
			Scan(&cur, &user, &coins); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if cur != "pending" { // уже обработан (или не создан у ЮKassa)
			return nil
		}
		switch status {
		case "succeeded":
			rec, err := transferTx(ctx, tx, Transfer{Kind: "purchase", From: SystemAccount, To: UserAccount(user),
				Amount: coins, Memo: "Покупка монет", Initiator: user, IdemKey: "pay:" + id})
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE payments SET status = 'succeeded', transaction_id = $2, updated_at = now() WHERE id = $1::uuid`, id, rec.ID)
			credited = err == nil
			return err
		case "canceled":
			_, err := tx.Exec(ctx, `UPDATE payments SET status = 'canceled', updated_at = now() WHERE id = $1::uuid`, id)
			return err
		}
		return nil // pending / waiting_for_capture — ждём
	})
	return credited, err
}
