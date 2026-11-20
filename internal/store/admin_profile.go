package store

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type AdminProfile struct {
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Email     string    `json:"email"`
	Birthday  *string   `json:"birthday"`
	CreatedAt time.Time `json:"created_at"`
}

func revokeCredentialsTx(ctx context.Context, tx pgx.Tx, id int64) error {
	for _, q := range []string{
		`UPDATE refresh_tokens SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`,
		`UPDATE api_tokens SET revoked_at=now() WHERE user_id=$1 AND kind='oauth' AND revoked_at IS NULL`,
		`UPDATE oauth_codes SET used_at=now() WHERE user_id=$1 AND used_at IS NULL`,
		`UPDATE auth_tokens SET used_at=now() WHERE user_id=$1 AND used_at IS NULL`,
		`UPDATE qr_logins SET rejected_at=now() WHERE approved_by=$1 AND consumed_at IS NULL`,
	} {
		if _, err := tx.Exec(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) AdminUpdateProfile(ctx context.Context, id int64, p AdminProfile) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var changed bool
		if err := tx.QueryRow(ctx, `SELECT email <> $2::citext FROM users WHERE id=$1 FOR UPDATE`, id, p.Email).Scan(&changed); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE users SET first_name=$2,last_name=$3,email=$4,birthday=$5::date,created_at=$6,email_verified_at=CASE WHEN email <> $4::citext THEN NULL ELSE email_verified_at END,updated_at=now() WHERE id=$1`, id, p.FirstName, p.LastName, p.Email, p.Birthday, p.CreatedAt)
		if err != nil {
			return err
		}
		if changed {
			return revokeCredentialsTx(ctx, tx, id)
		}
		return nil
	})
}
func (s *Store) AdminSetPassword(ctx context.Context, id int64, hash string) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE users SET password_hash=$2,updated_at=now() WHERE id=$1 AND bot_community_id IS NULL`, id, hash)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return revokeCredentialsTx(ctx, tx, id)
	})
}
