package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const MaxChatMembers = 500

type Attachment struct {
	Kind        string `json:"kind"` // photo | track | voice
	ID          string `json:"id"`
	URL         string `json:"url"`
	ThumbURL    string `json:"thumb_url,omitempty"`
	Title       string `json:"title,omitempty"`
	Artist      string `json:"artist,omitempty"`
	DurationSec int    `json:"duration_sec,omitempty"`
}

type Message struct {
	ID             int64        `json:"id"`
	ConversationID int64        `json:"conversation_id"`
	SenderID       int64        `json:"sender_id"`
	Kind           string       `json:"kind"` // text | service
	Body           string       `json:"body,omitempty"`
	Ciphertext     []byte       `json:"ciphertext,omitempty"` // base64 в JSON
	Nonce          []byte       `json:"nonce,omitempty"`
	Encrypted      bool         `json:"encrypted"`
	Attachments    []Attachment `json:"attachments"`
	CreatedAt      time.Time    `json:"created_at"`
}

func VoiceURL(id string) string { return "/api/v1/voice/" + id }

func pair(a, b int64) (int64, int64) {
	if a < b {
		return a, b
	}
	return b, a
}

// Conversation возвращает (или создаёт) личный диалог двух пользователей.
func (s *Store) Conversation(ctx context.Context, a, b int64) (int64, error) {
	x, y := pair(a, b)
	var id int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO conversations (user_a, user_b) VALUES ($1, $2)
			ON CONFLICT (user_a, user_b) DO UPDATE SET user_a = EXCLUDED.user_a
			RETURNING id`, x, y).Scan(&id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO conversation_state (conversation_id, user_id) VALUES ($1, $2), ($1, $3)
			ON CONFLICT DO NOTHING`, id, x, y)
		return err
	})
	return id, err
}

// ConvInfo — диалог глазами участника.
type ConvInfo struct {
	ID      int64
	Kind    string // direct | group
	Title   string
	OwnerID *int64
	MyRole  string
	Members []int64
}

// Peer — собеседник в личном диалоге.
func (c *ConvInfo) Peer(me int64) int64 {
	for _, m := range c.Members {
		if m != me {
			return m
		}
	}
	return 0
}

// ConvAccess проверяет участие и возвращает диалог. Для чужого — ErrNotFound
// (не раскрываем, что такой диалог существует).
func (s *Store) ConvAccess(ctx context.Context, convID, user int64) (*ConvInfo, error) {
	c := &ConvInfo{ID: convID}
	err := s.pool.QueryRow(ctx, `
		SELECT c.kind, c.title, c.owner_id, me.role,
		       ARRAY(SELECT user_id FROM conversation_state WHERE conversation_id = c.id ORDER BY user_id)
		FROM conversations c
		JOIN conversation_state me ON me.conversation_id = c.id AND me.user_id = $2
		WHERE c.id = $1`, convID, user).Scan(&c.Kind, &c.Title, &c.OwnerID, &c.MyRole, &c.Members)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// ConversationPeer — для совместимости: собеседник в личном диалоге.
func (s *Store) ConversationPeer(ctx context.Context, convID, user int64) (int64, error) {
	c, err := s.ConvAccess(ctx, convID, user)
	if err != nil {
		return 0, err
	}
	return c.Peer(user), nil
}

// ---------- групповые чаты ----------

func (s *Store) CreateGroup(ctx context.Context, owner int64, title string, members []int64) (int64, error) {
	var id int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO conversations (kind, title, owner_id) VALUES ('group', $1, $2) RETURNING id`,
			title, owner).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO conversation_state (conversation_id, user_id, role) VALUES ($1, $2, 'owner')`, id, owner); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO conversation_state (conversation_id, user_id)
			SELECT $1, u FROM unnest($2::bigint[]) AS u WHERE u <> $3
			ON CONFLICT DO NOTHING`, id, members, owner)
		return err
	})
	return id, err
}

// AddChatMember: возвращает false, если уже участник. Лимит — MaxChatMembers.
func (s *Store) AddChatMember(ctx context.Context, convID, user int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO conversation_state (conversation_id, user_id, last_read_id, last_delivered_id)
		SELECT $1, $2, COALESCE(c.last_message_id, 0), COALESCE(c.last_message_id, 0)
		FROM conversations c
		WHERE c.id = $1 AND c.kind = 'group'
		  AND (SELECT count(*) FROM conversation_state WHERE conversation_id = $1) < $3
		ON CONFLICT DO NOTHING`, convID, user, MaxChatMembers)
	return tag.RowsAffected() == 1, err
}

// RemoveChatMember; если ушёл владелец — им становится самый давний админ или участник.
func (s *Store) RemoveChatMember(ctx context.Context, convID, user int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var role string
		err := tx.QueryRow(ctx, `
			DELETE FROM conversation_state WHERE conversation_id = $1 AND user_id = $2 RETURNING role`,
			convID, user).Scan(&role)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil || role != "owner" {
			return err
		}
		_, err = tx.Exec(ctx, `
			WITH heir AS (
				SELECT user_id FROM conversation_state WHERE conversation_id = $1
				ORDER BY (role = 'admin') DESC, joined_at, user_id LIMIT 1
			), st AS (
				UPDATE conversation_state SET role = 'owner'
				WHERE conversation_id = $1 AND user_id = (SELECT user_id FROM heir)
			)
			UPDATE conversations SET owner_id = (SELECT user_id FROM heir) WHERE id = $1`, convID)
		return err
	})
}

func (s *Store) SetChatTitle(ctx context.Context, convID int64, title string) error {
	_, err := s.pool.Exec(ctx, `UPDATE conversations SET title = $2 WHERE id = $1 AND kind = 'group'`, convID, title)
	return err
}

func (s *Store) SetChatRole(ctx context.Context, convID, user int64, role string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE conversation_state SET role = $3
		WHERE conversation_id = $1 AND user_id = $2 AND role <> 'owner'`, convID, user, role)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

type ChatMember struct {
	UserCard
	Role string `json:"role"`
}

func (s *Store) ChatMembers(ctx context.Context, convID int64) ([]ChatMember, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+cardCols+`, st.role
		FROM conversation_state st JOIN users u ON u.id = st.user_id
		WHERE st.conversation_id = $1 ORDER BY st.joined_at, u.id`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatMember{}
	for rows.Next() {
		var m ChatMember
		var avatar *string
		if err := rows.Scan(&m.ID, &m.Username, &m.FirstName, &m.LastName, &avatar, &m.Role); err != nil {
			return nil, err
		}
		m.AvatarURL = MediaURL(avatar, "thumb")
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---------- сообщения ----------

type AttachmentRef struct {
	Kind string // photo | track | voice
	ID   string
}

type NewMessage struct {
	ConversationID int64
	SenderID       int64
	Kind           string // text (по умолчанию) | service
	Body           string
	Ciphertext     []byte
	Nonce          []byte
	Attachments    []AttachmentRef
}

// SendMessage сохраняет сообщение с вложениями. Фото и голосовые — только
// свои, треки — любые доступные (иначе ErrBadMedia).
func (s *Store) SendMessage(ctx context.Context, m NewMessage) (*Message, error) {
	if m.Kind == "" {
		m.Kind = "text"
	}
	var id int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO messages (conversation_id, sender_id, kind, body, ciphertext, nonce)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			m.ConversationID, m.SenderID, m.Kind, m.Body, m.Ciphertext, m.Nonce).Scan(&id); err != nil {
			return err
		}
		for i, a := range m.Attachments {
			var q string
			switch a.Kind {
			case "photo":
				q = `INSERT INTO message_attachments (message_id, position, kind, media_id)
				     SELECT $1, $2, 'photo', id FROM media WHERE id = $3::uuid AND owner_id = $4`
			case "track":
				q = `INSERT INTO message_attachments (message_id, position, kind, track_id)
				     SELECT $1, $2, 'track', id FROM tracks WHERE id = $3::uuid AND removed_at IS NULL AND $4::bigint > 0`
			case "voice":
				q = `INSERT INTO message_attachments (message_id, position, kind, voice_id)
				     SELECT $1, $2, 'voice', id FROM voices WHERE id = $3::uuid AND owner_id = $4`
			default:
				return ErrBadMedia
			}
			tag, err := tx.Exec(ctx, q, id, i, a.ID, m.SenderID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return ErrBadMedia
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE conversations SET last_message_id = $2 WHERE id = $1`, m.ConversationID, id); err != nil {
			return err
		}
		// своё сообщение отправитель уже «прочитал»
		_, err := tx.Exec(ctx, `
			UPDATE conversation_state SET last_read_id = $3, last_delivered_id = $3
			WHERE conversation_id = $1 AND user_id = $2`, m.ConversationID, m.SenderID, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	list, err := s.messages(ctx, `m.id = $1`, id, int64(0), 1)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

const messageSelect = `
	SELECT m.id, m.conversation_id, m.sender_id, m.kind, m.body, m.ciphertext, m.nonce, m.created_at,
	       COALESCE((SELECT json_agg(json_build_object(
	                   'kind', a.kind, 'media_id', a.media_id, 'track_id', a.track_id, 'voice_id', a.voice_id,
	                   'title', t.title, 'artist', t.artist,
	                   'duration_sec', COALESCE(t.duration_sec, v.duration_sec)) ORDER BY a.position)
	                 FROM message_attachments a
	                 LEFT JOIN tracks t ON t.id = a.track_id
	                 LEFT JOIN voices v ON v.id = a.voice_id
	                 WHERE a.message_id = m.id AND (a.track_id IS NULL OR t.removed_at IS NULL)), '[]')
	FROM messages m`

func (s *Store) messages(ctx context.Context, where string, arg any, before int64, limit int) ([]Message, error) {
	q := messageSelect + ` WHERE ` + where
	args := []any{arg}
	if before > 0 {
		q += ` AND m.id < $2 ORDER BY m.id DESC LIMIT $3`
		args = append(args, before, limit)
	}
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		var att []byte
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Kind, &m.Body, &m.Ciphertext, &m.Nonce,
			&m.CreatedAt, &att); err != nil {
			return nil, err
		}
		m.Encrypted = m.Ciphertext != nil
		if m.Attachments, err = decodeAttachments(att); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func decodeAttachments(raw []byte) ([]Attachment, error) {
	var rows []struct {
		Kind     string  `json:"kind"`
		MediaID  *string `json:"media_id"`
		TrackID  *string `json:"track_id"`
		VoiceID  *string `json:"voice_id"`
		Title    *string `json:"title"`
		Artist   *string `json:"artist"`
		Duration *int    `json:"duration_sec"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	out := make([]Attachment, 0, len(rows))
	for _, r := range rows {
		a := Attachment{Kind: r.Kind}
		switch {
		case r.MediaID != nil:
			a.ID, a.URL, a.ThumbURL = *r.MediaID, *MediaURL(r.MediaID, "full"), *MediaURL(r.MediaID, "thumb")
		case r.TrackID != nil:
			a.ID, a.URL = *r.TrackID, AudioURL(*r.TrackID)
		case r.VoiceID != nil:
			a.ID, a.URL = *r.VoiceID, VoiceURL(*r.VoiceID)
		}
		if r.Title != nil {
			a.Title = *r.Title
		}
		if r.Artist != nil {
			a.Artist = *r.Artist
		}
		if r.Duration != nil {
			a.DurationSec = *r.Duration
		}
		out = append(out, a)
	}
	return out, nil
}

func (s *Store) Messages(ctx context.Context, convID, before int64, limit int) ([]Message, error) {
	return s.messages(ctx, `m.conversation_id = $1`, convID, before, limit)
}

type ConversationItem struct {
	ID           int64     `json:"id"`
	Kind         string    `json:"kind"`
	Title        string    `json:"title,omitempty"`         // для групп
	MembersCount int       `json:"members_count,omitempty"` // для групп
	Peer         *UserCard `json:"peer,omitempty"`          // для личных диалогов
	LastMessage  Message   `json:"last_message"`
	Unread       int       `json:"unread"`
	// до какого сообщения собеседник получил / прочитал мои сообщения (личные диалоги)
	PeerDeliveredID int64 `json:"peer_delivered_id"`
	PeerReadID      int64 `json:"peer_read_id"`
}

// Conversations: диалоги и группы с сообщениями, свежие сверху. Курсор — last_message_id.
func (s *Store) Conversations(ctx context.Context, user, before int64, limit int) ([]ConversationItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.kind, c.title,
		       (SELECT count(*) FROM conversation_state x WHERE x.conversation_id = c.id),
		       u.id, u.username, u.first_name, u.last_name, u.avatar_media_id::text,
		       m.id, m.sender_id, m.kind, m.body, m.ciphertext, m.nonce, m.created_at,
		       (SELECT count(*) FROM messages x
		         WHERE x.conversation_id = c.id AND x.id > me.last_read_id AND x.sender_id <> $1),
		       COALESCE(peer.last_delivered_id, 0), COALESCE(peer.last_read_id, 0)
		FROM conversation_state me
		JOIN conversations c ON c.id = me.conversation_id
		JOIN messages m ON m.id = c.last_message_id
		LEFT JOIN users u ON c.kind = 'direct' AND u.id = CASE WHEN c.user_a = $1 THEN c.user_b ELSE c.user_a END
		LEFT JOIN conversation_state peer ON peer.conversation_id = c.id AND peer.user_id = u.id
		WHERE me.user_id = $1 AND c.last_message_id < $2
		ORDER BY c.last_message_id DESC LIMIT $3`, user, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConversationItem{}
	for rows.Next() {
		var (
			c                        ConversationItem
			pID                      *int64
			pUser, pFirst, pLast, av *string
		)
		if err := rows.Scan(&c.ID, &c.Kind, &c.Title, &c.MembersCount, &pID, &pUser, &pFirst, &pLast, &av,
			&c.LastMessage.ID, &c.LastMessage.SenderID, &c.LastMessage.Kind, &c.LastMessage.Body, &c.LastMessage.Ciphertext,
			&c.LastMessage.Nonce, &c.LastMessage.CreatedAt, &c.Unread, &c.PeerDeliveredID, &c.PeerReadID); err != nil {
			return nil, err
		}
		if pID != nil {
			c.Peer = &UserCard{ID: *pID, Username: *pUser, FirstName: *pFirst, LastName: *pLast, AvatarURL: MediaURL(av, "thumb")}
		}
		if c.Kind == "direct" {
			c.MembersCount = 0
		}
		c.LastMessage.ConversationID = c.ID
		c.LastMessage.Encrypted = c.LastMessage.Ciphertext != nil
		c.LastMessage.Attachments = []Attachment{}
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

// ---------- голосовые ----------

func (s *Store) CreateVoice(ctx context.Context, id string, owner int64, seconds int) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO voices (id, owner_id, duration_sec) VALUES ($1::uuid, $2, $3)`, id, owner, seconds)
	return err
}

// VoiceAccess: слушать голосовое может автор и участники чатов, где оно отправлено.
func (s *Store) VoiceAccess(ctx context.Context, id string, user int64) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM voices WHERE id = $1::uuid AND owner_id = $2)
		    OR EXISTS (SELECT 1 FROM message_attachments a
		               JOIN messages m ON m.id = a.message_id
		               JOIN conversation_state st ON st.conversation_id = m.conversation_id AND st.user_id = $2
		               WHERE a.voice_id = $1::uuid)`, id, user).Scan(&ok)
	return ok, err
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
