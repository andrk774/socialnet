package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrIdempotencyReuse  = errors.New("idempotency key reused with different parameters")
	ErrAlreadyClaimed    = errors.New("already claimed")
)

// Account — владелец счёта.
type Account struct {
	Kind  string // user | community | system
	Owner int64
}

func UserAccount(id int64) Account      { return Account{"user", id} }
func CommunityAccount(id int64) Account { return Account{"community", id} }

var SystemAccount = Account{"system", 0}

type Transfer struct {
	Kind      string // transfer | gift | donation | reward | mint | payout
	From, To  Account
	Amount    int64
	Memo      string
	Initiator int64
	IdemKey   string
}

type TxRecord struct {
	ID        int64
	Kind      string
	From, To  int64 // id счетов
	Amount    int64
	Memo      string
	CreatedAt time.Time
	Replayed  bool // запрос повторён с тем же ключом, деньги не списывались второй раз
}

func accountID(ctx context.Context, tx pgx.Tx, a Account) (int64, error) {
	if _, err := tx.Exec(ctx, `
		INSERT INTO accounts (kind, owner_id) VALUES ($1, $2) ON CONFLICT (kind, owner_id) DO NOTHING`,
		a.Kind, a.Owner); err != nil {
		return 0, err
	}
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM accounts WHERE kind = $1 AND owner_id = $2`, a.Kind, a.Owner).Scan(&id)
	return id, err
}

func findByIdemKey(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, initiator int64, key string) (*TxRecord, error) {
	r := &TxRecord{Replayed: true}
	err := q.QueryRow(ctx, `
		SELECT id, kind, from_account, to_account, amount, memo, created_at
		FROM wallet_transactions WHERE initiator_id = $1 AND idempotency_key = $2`, initiator, key).
		Scan(&r.ID, &r.Kind, &r.From, &r.To, &r.Amount, &r.Memo, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

// transferTx переводит монеты внутри уже открытой транзакции.
func transferTx(ctx context.Context, tx pgx.Tx, t Transfer) (*TxRecord, error) {
	from, err := accountID(ctx, tx, t.From)
	if err != nil {
		return nil, err
	}
	to, err := accountID(ctx, tx, t.To)
	if err != nil {
		return nil, err
	}

	if t.IdemKey != "" {
		prev, err := findByIdemKey(ctx, tx, t.Initiator, t.IdemKey)
		if err != nil {
			return nil, err
		}
		if prev != nil {
			if prev.From != from || prev.To != to || prev.Amount != t.Amount || prev.Kind != t.Kind {
				return nil, ErrIdempotencyReuse
			}
			return prev, nil
		}
	}

	// Блокируем оба счёта всегда в порядке возрастания id: два встречных
	// перевода A→B и B→A не заблокируют друг друга (deadlock).
	rows, err := tx.Query(ctx, `
		SELECT id, kind, balance FROM accounts WHERE id IN ($1, $2) ORDER BY id FOR UPDATE`, from, to)
	if err != nil {
		return nil, err
	}
	var fromBalance int64
	var fromKind string
	for rows.Next() {
		var id, bal int64
		var kind string
		if err := rows.Scan(&id, &kind, &bal); err != nil {
			rows.Close()
			return nil, err
		}
		if id == from {
			fromBalance, fromKind = bal, kind
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if fromKind != "system" && fromBalance < t.Amount {
		return nil, ErrInsufficientFunds
	}

	if _, err := tx.Exec(ctx, `UPDATE accounts SET balance = balance - $2 WHERE id = $1`, from, t.Amount); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE accounts SET balance = balance + $2 WHERE id = $1`, to, t.Amount); err != nil {
		return nil, err
	}

	var key *string
	if t.IdemKey != "" {
		key = &t.IdemKey
	}
	var initiator *int64
	if t.Initiator != 0 {
		initiator = &t.Initiator
	}
	r := &TxRecord{Kind: t.Kind, From: from, To: to, Amount: t.Amount, Memo: t.Memo}
	err = tx.QueryRow(ctx, `
		INSERT INTO wallet_transactions (kind, from_account, to_account, amount, memo, initiator_id, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`,
		t.Kind, from, to, t.Amount, t.Memo, initiator, key).Scan(&r.ID, &r.CreatedAt)
	return r, err
}

// inTx выполняет fn в транзакции.
func (s *Store) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Transfer переводит монеты. Если параллельный запрос с тем же ключом
// идемпотентности успел первым (unique violation), возвращается его результат.
func (s *Store) Transfer(ctx context.Context, t Transfer) (*TxRecord, error) {
	var rec *TxRecord
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		rec, err = transferTx(ctx, tx, t)
		return err
	})
	if _, dup := isUnique(err); dup && t.IdemKey != "" {
		return s.replay(ctx, t)
	}
	return rec, err
}

func (s *Store) replay(ctx context.Context, t Transfer) (*TxRecord, error) {
	prev, err := findByIdemKey(ctx, s.pool, t.Initiator, t.IdemKey)
	if err != nil || prev == nil {
		return nil, err
	}
	if prev.Amount != t.Amount || prev.Kind != t.Kind {
		return nil, ErrIdempotencyReuse
	}
	return prev, nil
}

func (s *Store) Balance(ctx context.Context, a Account) (int64, error) {
	var b int64
	err := s.pool.QueryRow(ctx, `SELECT balance FROM accounts WHERE kind = $1 AND owner_id = $2`, a.Kind, a.Owner).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return b, err
}

// TotalBalance — сумма всех счетов. По построению всегда 0 (проверка в тестах).
func (s *Store) TotalBalance(ctx context.Context) (int64, error) {
	var t int64
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(sum(balance), 0) FROM accounts`).Scan(&t)
	return t, err
}

type Counterparty struct {
	Type string `json:"type"` // user | community | system
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name"`
}

type TxView struct {
	ID           int64        `json:"id"`
	Kind         string       `json:"kind"`
	Direction    string       `json:"direction"` // in | out
	Amount       int64        `json:"amount"`
	Counterparty Counterparty `json:"counterparty"`
	Memo         string       `json:"memo"`
	CreatedAt    time.Time    `json:"created_at"`
}

// History — операции счёта, свежие сверху.
func (s *Store) History(ctx context.Context, a Account, before int64, limit int) ([]TxView, error) {
	rows, err := s.pool.Query(ctx, `
		WITH me AS (SELECT id FROM accounts WHERE kind = $1 AND owner_id = $2)
		SELECT t.id, t.kind, t.amount, t.memo, t.created_at,
		       CASE WHEN t.to_account = me.id THEN 'in' ELSE 'out' END,
		       other.kind, other.owner_id,
		       COALESCE(u.first_name || ' ' || u.last_name, c.name, 'Socialnet')
		FROM wallet_transactions t
		JOIN me ON t.from_account = me.id OR t.to_account = me.id
		JOIN accounts other ON other.id = CASE WHEN t.to_account = me.id THEN t.from_account ELSE t.to_account END
		LEFT JOIN users u ON other.kind = 'user' AND u.id = other.owner_id
		LEFT JOIN communities c ON other.kind = 'community' AND c.id = other.owner_id
		WHERE t.id < $3
		ORDER BY t.id DESC LIMIT $4`, a.Kind, a.Owner, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TxView{}
	for rows.Next() {
		var v TxView
		if err := rows.Scan(&v.ID, &v.Kind, &v.Amount, &v.Memo, &v.CreatedAt, &v.Direction,
			&v.Counterparty.Type, &v.Counterparty.ID, &v.Counterparty.Name); err != nil {
			return nil, err
		}
		if v.Counterparty.Type == "system" {
			v.Counterparty.ID = 0
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// DailyBonus — раз в сутки (по UTC).
func (s *Store) DailyBonus(ctx context.Context, userID, amount int64) (*TxRecord, error) {
	var rec *TxRecord
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO daily_bonuses (user_id, day) VALUES ($1, (now() AT TIME ZONE 'UTC')::date)
			ON CONFLICT DO NOTHING`, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrAlreadyClaimed
		}
		rec, err = transferTx(ctx, tx, Transfer{Kind: "reward", From: SystemAccount, To: UserAccount(userID),
			Amount: amount, Memo: "Ежедневный бонус", Initiator: userID})
		return err
	})
	return rec, err
}

// ---------- подарки ----------

type GiftType struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Price int64  `json:"price"`
}

func (s *Store) GiftCatalog(ctx context.Context) ([]GiftType, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, price FROM gift_catalog ORDER BY price`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GiftType
	for rows.Next() {
		var g GiftType
		if err := rows.Scan(&g.ID, &g.Name, &g.Price); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

type SentGift struct {
	ID        int64     `json:"id"`
	Gift      GiftType  `json:"gift"`
	Sender    *UserCard `json:"sender"` // nil для анонимного подарка
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// SendGift: монеты сгорают (уходят на системный счёт), подарок появляется в профиле.
func (s *Store) SendGift(ctx context.Context, sender, recipient int64, giftID int, message string, anonymous bool, idem string) (int64, bool, error) {
	var giftRowID int64
	var replayed bool
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var price int64
		err := tx.QueryRow(ctx, `SELECT price FROM gift_catalog WHERE id = $1`, giftID).Scan(&price)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		rec, err := transferTx(ctx, tx, Transfer{Kind: "gift", From: UserAccount(sender), To: SystemAccount,
			Amount: price, Memo: "Подарок", Initiator: sender, IdemKey: idem})
		if err != nil {
			return err
		}
		if rec.Replayed {
			replayed = true
			return tx.QueryRow(ctx, `SELECT id FROM gifts WHERE transaction_id = $1`, rec.ID).Scan(&giftRowID)
		}
		return tx.QueryRow(ctx, `
			INSERT INTO gifts (gift_id, sender_id, recipient_id, message, anonymous, transaction_id)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			giftID, sender, recipient, message, anonymous, rec.ID).Scan(&giftRowID)
	})
	return giftRowID, replayed, err
}

func (s *Store) UserGifts(ctx context.Context, recipient, before int64, limit int) ([]SentGift, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT g.id, gc.id, gc.name, gc.price, g.message, g.created_at, g.anonymous,
		       u.id, u.username, u.first_name, u.last_name, u.avatar_media_id::text
		FROM gifts g
		JOIN gift_catalog gc ON gc.id = g.gift_id
		JOIN users u ON u.id = g.sender_id
		WHERE g.recipient_id = $1 AND g.id < $2
		ORDER BY g.id DESC LIMIT $3`, recipient, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SentGift{}
	for rows.Next() {
		var (
			g      SentGift
			anon   bool
			c      UserCard
			avatar *string
		)
		if err := rows.Scan(&g.ID, &g.Gift.ID, &g.Gift.Name, &g.Gift.Price, &g.Message, &g.CreatedAt, &anon,
			&c.ID, &c.Username, &c.FirstName, &c.LastName, &avatar); err != nil {
			return nil, err
		}
		if !anon { // анонимный отправитель не попадает в ответ вообще
			c.AvatarURL = MediaURL(avatar, "thumb")
			g.Sender = &c
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
