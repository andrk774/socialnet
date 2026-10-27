package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// Plus — состояние подписки «Плюс».
type Plus struct {
	Active    bool       `json:"active"`
	ExpiresAt *time.Time `json:"expires_at"`
	AutoRenew bool       `json:"auto_renew"`
}

func (s *Store) PlusStatus(ctx context.Context, user int64) (Plus, error) {
	var p Plus
	var exp time.Time
	err := s.pool.QueryRow(ctx, `SELECT expires_at, auto_renew FROM subscriptions WHERE user_id = $1`, user).Scan(&exp, &p.AutoRenew)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	p.ExpiresAt, p.Active = &exp, exp.After(time.Now())
	return p, nil
}

// HasPlus — для лимитов: действует ли подписка прямо сейчас.
func (s *Store) HasPlus(ctx context.Context, user int64) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM subscriptions WHERE user_id = $1 AND expires_at > now())`, user).Scan(&ok)
	return ok, err
}

// BuyPlus списывает цену и продлевает подписку на days дней от конца текущей
// (или от сейчас) — в одной транзакции: нет денег — нет продления, и наоборот.
// Повтор с тем же ключом идемпотентности ничего не списывает.
func (s *Store) BuyPlus(ctx context.Context, user, price int64, days int, idem string) (*TxRecord, Plus, error) {
	var rec *TxRecord
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		rec, err = transferTx(ctx, tx, Transfer{Kind: "subscription", From: UserAccount(user), To: SystemAccount,
			Amount: price, Memo: "Подписка «Плюс» на " + strconv.Itoa(days) + " дней", Initiator: user, IdemKey: idem})
		if err != nil || rec.Replayed {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO subscriptions (user_id, expires_at) VALUES ($1, now() + make_interval(days => $2))
			ON CONFLICT (user_id) DO UPDATE SET
				expires_at = GREATEST(subscriptions.expires_at, now()) + make_interval(days => $2), updated_at = now()`, user, days)
		return err
	})
	if _, dup := isUnique(err); dup && idem != "" {
		rec, err = s.replay(ctx, Transfer{Kind: "subscription", Amount: price, Initiator: user, IdemKey: idem})
	}
	if err != nil {
		return nil, Plus{}, err
	}
	p, err := s.PlusStatus(ctx, user)
	return rec, p, err
}

func (s *Store) SetPlusAutoRenew(ctx context.Context, user int64, on bool) error {
	tag, err := s.pool.Exec(ctx, `UPDATE subscriptions SET auto_renew = $2, updated_at = now() WHERE user_id = $1`, user, on)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

// DueRenewals — у кого подписка кончается в ближайший час и включено продление.
func (s *Store) DueRenewals(ctx context.Context, limit int) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `SELECT user_id FROM subscriptions
		WHERE auto_renew AND expires_at < now() + interval '1 hour' AND expires_at > now() - interval '3 days'
		ORDER BY expires_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// PlusMembers — у кого из списка есть подписка (для значков).
func (s *Store) PlusMembers(ctx context.Context, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	rows, err := s.pool.Query(ctx, `SELECT user_id FROM subscriptions WHERE user_id = ANY($1) AND expires_at > now()`, ids)
	if err != nil {
		return nil, err
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	for _, id := range got {
		out[id] = true
	}
	return out, err
}
