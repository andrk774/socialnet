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

// RefundResult — что сделал возврат.
type RefundResult struct {
	PaymentID string
	UserID    int64
	Coins     int64 // положено списать за этот возврат
	Debited   int64 // списано сразу
	Debt      int64 // ушло в долг (монеты уже потрачены)
	Replayed  bool  // этот возврат уже был обработан
}

var ErrRefundMismatch = errors.New("refund does not match payment")

// ApplyRefund списывает монеты за возврат (данные уже проверены у ЮKassa).
// Монеты — пропорционально возвращённой сумме, с округлением вверх: частичный
// возврат не должен оставлять «бесплатные» монеты. Повтор того же возврата
// ничего не меняет.
func (s *Store) ApplyRefund(ctx context.Context, providerRefundID, providerPaymentID string, kopecks int64) (*RefundResult, error) {
	res := &RefundResult{}
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var status string
		var coins, amount, refunded int64
		err := tx.QueryRow(ctx, `
			SELECT id::text, user_id, status, coins, amount_kopecks, refunded_kopecks FROM payments
			WHERE provider_payment_id = $1 FOR UPDATE`, providerPaymentID).
			Scan(&res.PaymentID, &res.UserID, &status, &coins, &amount, &refunded)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM payment_refunds WHERE provider_refund_id = $1)`,
			providerRefundID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			res.Replayed = true
			return nil
		}
		if status != "succeeded" && status != "partially_refunded" {
			return ErrRefundMismatch
		}
		total := refunded + kopecks
		if kopecks <= 0 || total > amount {
			return ErrRefundMismatch
		}
		var already int64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(coins), 0) FROM payment_refunds WHERE payment_id = $1::uuid`,
			res.PaymentID).Scan(&already); err != nil {
			return err
		}
		due := (coins*total+amount-1)/amount - already // ceil(coins*total/amount) минус уже списанное
		res.Coins = max(due, 0)

		acc, err := accountID(ctx, tx, UserAccount(res.UserID))
		if err != nil {
			return err
		}
		var balance int64
		if err := tx.QueryRow(ctx, `SELECT balance FROM accounts WHERE id = $1 FOR UPDATE`, acc).Scan(&balance); err != nil {
			return err
		}
		res.Debited = min(res.Coins, balance)
		res.Debt = res.Coins - res.Debited
		if res.Debited > 0 {
			if _, err := debit(ctx, tx, acc, res.Debited, "refund", "Возврат платежа"); err != nil {
				return err
			}
		}
		if res.Debt > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO wallet_debts (user_id, amount) VALUES ($1, $2)
				ON CONFLICT (user_id) DO UPDATE SET amount = wallet_debts.amount + EXCLUDED.amount, updated_at = now()`,
				res.UserID, res.Debt); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO payment_refunds (provider_refund_id, payment_id, amount_kopecks, coins, debited)
			VALUES ($1, $2::uuid, $3, $4, $5)`, providerRefundID, res.PaymentID, kopecks, res.Coins, res.Debited); err != nil {
			return err
		}
		newStatus := "partially_refunded"
		if total == amount {
			newStatus = "refunded"
		}
		_, err = tx.Exec(ctx, `UPDATE payments SET refunded_kopecks = $2, status = $3, updated_at = now() WHERE id = $1::uuid`,
			res.PaymentID, total, newStatus)
		return err
	})
	return res, err
}
