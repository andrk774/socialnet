package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Fundraiser struct {
	ID          int64        `json:"id"`
	Community   CommunityRef `json:"community"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Goal        int64        `json:"goal"`
	Raised      int64        `json:"raised"`
	Donors      int          `json:"donors"`
	Deadline    time.Time    `json:"deadline"`
	Open        bool         `json:"open"` // принимает пожертвования
	ClosedAt    *time.Time   `json:"closed_at"`
	CreatedAt   time.Time    `json:"created_at"`
}

type Donation struct {
	User      *UserCard `json:"user"` // nil — анонимно
	Amount    int64     `json:"amount"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

var ErrFundraiserClosed = errors.New("fundraiser closed")

const fundraiserVisible = ` c.hidden_at IS NULL AND (c.visibility = 'open'
	OR EXISTS (SELECT 1 FROM community_members m WHERE m.community_id = c.id AND m.user_id = $1))`

const fundraiserSelect = `SELECT f.id, c.id, c.slug, c.name, f.title, f.description, f.goal,
	COALESCE((SELECT sum(amount) FROM fundraiser_donations d WHERE d.fundraiser_id = f.id), 0),
	(SELECT count(DISTINCT COALESCE(d.user_id, -d.id)) FROM fundraiser_donations d WHERE d.fundraiser_id = f.id),
	f.deadline, f.closed_at IS NULL AND f.deadline > now(), f.closed_at, f.created_at
	FROM fundraisers f JOIN communities c ON c.id = f.community_id WHERE` + fundraiserVisible

func (s *Store) queryFundraisers(ctx context.Context, sql string, args ...any) ([]Fundraiser, error) {
	rows, err := s.pool.Query(ctx, fundraiserSelect+sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Fundraiser{}
	for rows.Next() {
		var f Fundraiser
		if err := rows.Scan(&f.ID, &f.Community.ID, &f.Community.Slug, &f.Community.Name, &f.Title, &f.Description, &f.Goal,
			&f.Raised, &f.Donors, &f.Deadline, &f.Open, &f.ClosedAt, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) Fundraiser(ctx context.Context, viewer, id int64) (*Fundraiser, error) {
	list, err := s.queryFundraisers(ctx, ` AND f.id = $2`, viewer, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// Fundraisers — сборы сообщества (community > 0) или все открытые.
func (s *Store) Fundraisers(ctx context.Context, viewer, community int64, limit int) ([]Fundraiser, error) {
	if community > 0 {
		return s.queryFundraisers(ctx, ` AND f.community_id = $2 ORDER BY f.id DESC LIMIT $3`, viewer, community, limit)
	}
	return s.queryFundraisers(ctx, ` AND f.closed_at IS NULL AND f.deadline > now() ORDER BY f.deadline LIMIT $2`, viewer, limit)
}

func (s *Store) CreateFundraiser(ctx context.Context, community, by int64, title, desc string, goal int64, deadline time.Time) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `INSERT INTO fundraisers (community_id, created_by, title, description, goal, deadline)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, community, by, title, desc, goal, deadline).Scan(&id)
	return id, err
}

func (s *Store) CloseFundraiser(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE fundraisers SET closed_at = COALESCE(closed_at, now()) WHERE id = $1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

// Donate: перевод на счёт сообщества и запись о пожертвовании — одной
// транзакцией, под блокировкой сбора (закрытие и срок проверяются там же).
func (s *Store) Donate(ctx context.Context, fundraiser, user, amount int64, anonymous bool, message, idem string) (*TxRecord, error) {
	var rec *TxRecord
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var community int64
		var open bool
		var title string
		err := tx.QueryRow(ctx, `SELECT community_id, closed_at IS NULL AND deadline > now(), title FROM fundraisers WHERE id = $1 FOR UPDATE`,
			fundraiser).Scan(&community, &open, &title)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if prev, err := findByIdemKey(ctx, tx, user, idem); err != nil {
			return err
		} else if prev != nil { // повтор — без проверки срока и без второй записи
			rec, err = transferTx(ctx, tx, Transfer{Kind: "donation", From: UserAccount(user), To: CommunityAccount(community),
				Amount: amount, Initiator: user, IdemKey: idem})
			return err
		}
		if !open {
			return ErrFundraiserClosed
		}
		memo := "Сбор «" + title + "»"
		if len([]rune(memo)) > 200 {
			memo = string([]rune(memo)[:199]) + "…"
		}
		if rec, err = transferTx(ctx, tx, Transfer{Kind: "donation", From: UserAccount(user), To: CommunityAccount(community),
			Amount: amount, Memo: memo, Initiator: user, IdemKey: idem}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO fundraiser_donations (fundraiser_id, tx_id, user_id, amount, anonymous, message)
			VALUES ($1, $2, $3, $4, $5, $6)`, fundraiser, rec.ID, user, amount, anonymous, message)
		return err
	})
	if _, dup := isUnique(err); dup && idem != "" {
		return s.replay(ctx, Transfer{Kind: "donation", Amount: amount, Initiator: user, IdemKey: idem})
	}
	return rec, err
}

func (s *Store) Donations(ctx context.Context, fundraiser int64, limit int) ([]Donation, error) {
	rows, err := s.pool.Query(ctx, `SELECT d.anonymous, u.id, u.username, u.first_name, u.last_name, u.avatar_media_id::text,
		d.amount, d.message, d.created_at
		FROM fundraiser_donations d LEFT JOIN users u ON u.id = d.user_id AND u.banned_at IS NULL
		WHERE d.fundraiser_id = $1 ORDER BY d.id DESC LIMIT $2`, fundraiser, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Donation{}
	for rows.Next() {
		var d Donation
		var anon bool
		var id *int64
		var uname, first, last, avatar *string
		if err := rows.Scan(&anon, &id, &uname, &first, &last, &avatar, &d.Amount, &d.Message, &d.CreatedAt); err != nil {
			return nil, err
		}
		if !anon && id != nil {
			d.User = &UserCard{ID: *id, Username: *uname, FirstName: *first, LastName: *last, AvatarURL: MediaURL(avatar, "thumb")}
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
