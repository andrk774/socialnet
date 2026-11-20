package store

import (
	"context"
	"time"
)

type Call struct {
	ID         int64      `json:"id"`
	CallerID   int64      `json:"caller_id"`
	CalleeID   int64      `json:"callee_id"`
	Video      bool       `json:"video"`
	Status     string     `json:"status"`
	Offer      string     `json:"offer,omitempty"`
	Answer     string     `json:"answer,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	AnsweredAt *time.Time `json:"answered_at,omitempty"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	Peer       UserCard   `json:"peer"`
}

const expireCalls = `UPDATE calls SET status=CASE WHEN status='ringing' THEN 'missed' ELSE 'ended' END,ended_at=now(),offer='',answer='' WHERE status IN ('ringing','active') AND ((status='ringing' AND created_at<now()-interval '60 seconds') OR (status='active' AND (least(caller_seen,callee_seen)<now()-interval '90 seconds' OR answered_at<now()-interval '4 hours')))`

func (s *Store) CreateCall(ctx context.Context, caller, callee int64, video bool, offer string) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Lock both user rows in a stable order: a user cannot take part in two live calls.
	rows, err := tx.Query(ctx, `SELECT id FROM users WHERE id IN ($1,$2) ORDER BY id FOR UPDATE`, caller, callee)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, expireCalls); err != nil {
		return 0, err
	}
	var busy bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM calls WHERE status IN ('ringing','active') AND (caller_id IN ($1,$2) OR callee_id IN ($1,$2)))`, caller, callee).Scan(&busy)
	if err != nil {
		return 0, err
	}
	if busy {
		return 0, ErrConflict
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO calls(caller_id,callee_id,video,offer) VALUES($1,$2,$3,$4) RETURNING id`, caller, callee, video, offer).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

func (s *Store) Calls(ctx context.Context, user, id, before int64) ([]Call, error) {
	if _, err := s.pool.Exec(ctx, expireCalls+` AND (caller_id=$1 OR callee_id=$1)`, user); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT c.id,c.caller_id,c.callee_id,c.video,c.status,
 CASE WHEN $2::bigint>0 THEN c.offer ELSE '' END,CASE WHEN $2::bigint>0 THEN c.answer ELSE '' END,
 c.created_at,c.answered_at,c.ended_at,u.id,u.username,u.first_name,u.last_name,u.avatar_media_id::text
 FROM calls c JOIN users u ON u.id=CASE WHEN c.caller_id=$1 THEN c.callee_id ELSE c.caller_id END
 WHERE (c.caller_id=$1 OR c.callee_id=$1) AND ($2::bigint=0 OR c.id=$2) AND ($3::bigint=0 OR c.id<$3)
 AND ($2::bigint>0 OR NOT EXISTS(SELECT 1 FROM call_history_hidden h WHERE h.call_id=c.id AND h.user_id=$1))
 ORDER BY c.id DESC LIMIT 100`, user, id, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Call{}
	for rows.Next() {
		var c Call
		var avatar *string
		if err := rows.Scan(&c.ID, &c.CallerID, &c.CalleeID, &c.Video, &c.Status, &c.Offer, &c.Answer, &c.CreatedAt, &c.AnsweredAt, &c.EndedAt, &c.Peer.ID, &c.Peer.Username, &c.Peer.FirstName, &c.Peer.LastName, &avatar); err != nil {
			return nil, err
		}
		c.Peer.AvatarURL = MediaURL(avatar, "thumb")
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CallAction(ctx context.Context, user, id int64, action, answer string) error {
	var sql string
	switch action {
	case "answer":
		sql = `UPDATE calls SET status='active',answer=$3,answered_at=now(),caller_seen=now(),callee_seen=now() WHERE id=$1 AND callee_id=$2 AND status='ringing' AND created_at>now()-interval '60 seconds'`
	case "decline":
		sql = `UPDATE calls SET status='declined',ended_at=now(),offer='',answer='' WHERE id=$1 AND callee_id=$2 AND status='ringing' AND $3=''`
	case "end":
		sql = `UPDATE calls SET status='ended',ended_at=coalesce(ended_at,now()),offer='',answer='' WHERE id=$1 AND (caller_id=$2 OR callee_id=$2) AND status IN ('ringing','active') AND $3=''`
	case "heartbeat":
		sql = `UPDATE calls SET caller_seen=CASE WHEN caller_id=$2 THEN now() ELSE caller_seen END,callee_seen=CASE WHEN callee_id=$2 THEN now() ELSE callee_seen END WHERE id=$1 AND (caller_id=$2 OR callee_id=$2) AND status='active' AND $3=''`
	default:
		return ErrNotFound
	}
	result, err := s.pool.Exec(ctx, sql, id, user, answer)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func (s *Store) ClearCallHistory(ctx context.Context, user int64) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO call_history_hidden(call_id,user_id) SELECT id,$1 FROM calls WHERE (caller_id=$1 OR callee_id=$1) AND status NOT IN ('ringing','active') ON CONFLICT DO NOTHING`, user)
	return err
}
