package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Message struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversation_id"`
	SenderID       int64     `json:"sender_id"`
	Body           string    `json:"body,omitempty"`
	Ciphertext     []byte    `json:"ciphertext,omitempty"` // base64 в JSON
	Nonce          []byte    `json:"nonce,omitempty"`
	Encrypted      bool      `json:"encrypted"`
	CreatedAt      time.Time `json:"created_at"`
}

func pair(a, b int64) (int64, int64) {
	if a < b {
		return a, b
	}
	return b, a
}

// Conversation возвращает (или создаёт) диалог двух пользователей.
func (s *Store) Conversation(ctx context.Context, a, b int64) (int64, error) {
	x, y := pair(a, b)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO conversations (user_a, user_b) VALUES ($1, $2)
		ON CONFLICT (user_a, user_b) DO UPDATE SET user_a = EXCLUDED.user_a
		RETURNING id`, x, y).Scan(&id); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO conversation_state (conversation_id, user_id) VALUES ($1, $2), ($1, $3)
		ON CONFLICT DO NOTHING`, id, x, y); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

// ConversationPeer проверяет, что user участник диалога, и возвращает собеседника.
// Для чужого диалога — ErrNotFound (защита от IDOR).
func (s *Store) ConversationPeer(ctx context.Context, convID, user int64) (int64, error) {
	var a, b int64
	err := s.pool.QueryRow(ctx, `
		SELECT user_a, user_b FROM conversations WHERE id = $1 AND (user_a = $2 OR user_b = $2)`,
		convID, user).Scan(&a, &b)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if a == user {
		return b, err
	}
	return a, err
}

type NewMessage struct {
	ConversationID int64
	SenderID       int64
	Body           string
	Ciphertext     []byte
	Nonce          []byte
}

func (s *Store) SendMessage(ctx context.Context, m NewMessage) (*Message, error) {
	msg := &Message{ConversationID: m.ConversationID, SenderID: m.SenderID, Body: m.Body,
		Ciphertext: m.Ciphertext, Nonce: m.Nonce, Encrypted: m.Ciphertext != nil}
	err := s.pool.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO messages (conversation_id, sender_id, body, ciphertext, nonce)
			VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at
		), upd AS (
			UPDATE conversations SET last_message_id = (SELECT id FROM ins) WHERE id = $1
		), st AS (
			-- своё сообщение отправитель уже «прочитал»
			UPDATE conversation_state SET last_read_id = (SELECT id FROM ins), last_delivered_id = (SELECT id FROM ins)
			WHERE conversation_id = $1 AND user_id = $2
		)
		SELECT id, created_at FROM ins`,
		m.ConversationID, m.SenderID, m.Body, m.Ciphertext, m.Nonce).Scan(&msg.ID, &msg.CreatedAt)
	return msg, err
}

func (s *Store) Messages(ctx context.Context, convID, before int64, limit int) ([]Message, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, conversation_id, sender_id, body, ciphertext, nonce, created_at
		FROM messages WHERE conversation_id = $1 AND id < $2
		ORDER BY id DESC LIMIT $3`, convID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Body, &m.Ciphertext, &m.Nonce, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Encrypted = m.Ciphertext != nil
		out = append(out, m)
	}
	return out, rows.Err()
}

type ConversationItem struct {
	ID          int64    `json:"id"`
	Peer        UserCard `json:"peer"`
	LastMessage Message  `json:"last_message"`
	Unread      int      `json:"unread"`
	// до какого сообщения собеседник получил / прочитал мои сообщения
	PeerDeliveredID int64 `json:"peer_delivered_id"`
	PeerReadID      int64 `json:"peer_read_id"`
}

// Conversations: диалоги с сообщениями, свежие сверху. Курсор — last_message_id.
func (s *Store) Conversations(ctx context.Context, user, before int64, limit int) ([]ConversationItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, u.id, u.username, u.first_name, u.last_name, u.avatar_media_id::text,
		       m.id, m.sender_id, m.body, m.ciphertext, m.nonce, m.created_at,
		       (SELECT count(*) FROM messages x
		         WHERE x.conversation_id = c.id AND x.id > me.last_read_id AND x.sender_id <> $1),
		       peer.last_delivered_id, peer.last_read_id
		FROM conversations c
		JOIN users u ON u.id = CASE WHEN c.user_a = $1 THEN c.user_b ELSE c.user_a END
		JOIN messages m ON m.id = c.last_message_id
		JOIN conversation_state me   ON me.conversation_id = c.id AND me.user_id = $1
		JOIN conversation_state peer ON peer.conversation_id = c.id AND peer.user_id = u.id
		WHERE (c.user_a = $1 OR c.user_b = $1) AND c.last_message_id < $2
		ORDER BY c.last_message_id DESC LIMIT $3`, user, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConversationItem{}
	for rows.Next() {
		var c ConversationItem
		var avatar *string
		if err := rows.Scan(&c.ID, &c.Peer.ID, &c.Peer.Username, &c.Peer.FirstName, &c.Peer.LastName, &avatar,
			&c.LastMessage.ID, &c.LastMessage.SenderID, &c.LastMessage.Body, &c.LastMessage.Ciphertext,
			&c.LastMessage.Nonce, &c.LastMessage.CreatedAt, &c.Unread, &c.PeerDeliveredID, &c.PeerReadID); err != nil {
			return nil, err
		}
		c.Peer.AvatarURL = MediaURL(avatar, "thumb")
		c.LastMessage.ConversationID = c.ID
		c.LastMessage.Encrypted = c.LastMessage.Ciphertext != nil
		out = append(out, c)
	}
	return out, rows.Err()
}

// MarkRead / MarkDelivered двигают отметку только вперёд и не дальше
// последнего сообщения диалога.
func (s *Store) MarkRead(ctx context.Context, convID, user, upTo int64) (int64, error) {
	var v int64
	err := s.pool.QueryRow(ctx, `
		UPDATE conversation_state st SET
			last_read_id = GREATEST(st.last_read_id, LEAST($3, c.last_message_id)),
			last_delivered_id = GREATEST(st.last_delivered_id, LEAST($3, c.last_message_id))
		FROM conversations c
		WHERE c.id = st.conversation_id AND st.conversation_id = $1 AND st.user_id = $2
		RETURNING st.last_read_id`, convID, user, upTo).Scan(&v)
	return v, err
}

func (s *Store) MarkDelivered(ctx context.Context, convID, user, upTo int64) (int64, error) {
	var v int64
	err := s.pool.QueryRow(ctx, `
		UPDATE conversation_state st SET
			last_delivered_id = GREATEST(st.last_delivered_id, LEAST($3, c.last_message_id))
		FROM conversations c
		WHERE c.id = st.conversation_id AND st.conversation_id = $1 AND st.user_id = $2
		RETURNING st.last_delivered_id`, convID, user, upTo).Scan(&v)
	return v, err
}

// ---------- ключи для сквозного шифрования ----------

func (s *Store) SetIdentityKey(ctx context.Context, user int64, key []byte) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_keys (user_id, identity_key) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET identity_key = EXCLUDED.identity_key, updated_at = now()`,
		user, key)
	return err
}

func (s *Store) IdentityKey(ctx context.Context, user int64) ([]byte, time.Time, error) {
	var key []byte
	var updated time.Time
	err := s.pool.QueryRow(ctx, `SELECT identity_key, updated_at FROM user_keys WHERE user_id = $1`,
		user).Scan(&key, &updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, updated, ErrNotFound
	}
	return key, updated, err
}
