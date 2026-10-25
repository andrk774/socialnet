package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Event struct {
	ID          int64         `json:"id"`
	CreatorID   int64         `json:"creator_id"`
	Community   *CommunityRef `json:"community,omitempty"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Place       string        `json:"place"`
	StartsAt    time.Time     `json:"starts_at"`
	EndsAt      *time.Time    `json:"ends_at,omitempty"`
	Going       int           `json:"going_count"`
	Maybe       int           `json:"maybe_count"`
	MyStatus    *string       `json:"my_status"` // going | maybe | null
	CreatedAt   time.Time     `json:"created_at"`
}

// eventVisible: мероприятие закрытого сообщества — только участникам.
const eventVisible = `
	(e.community_id IS NULL OR c.visibility = 'open'
	 OR EXISTS (SELECT 1 FROM community_members m WHERE m.community_id = e.community_id AND m.user_id = $1))
	AND (c.id IS NULL OR c.hidden_at IS NULL)`

const eventSelect = `
	SELECT e.id, e.creator_id, c.id, c.slug, c.name, e.title, e.description, e.place, e.starts_at, e.ends_at,
	       (SELECT count(*) FROM event_attendees x WHERE x.event_id = e.id AND x.status = 'going'),
	       (SELECT count(*) FROM event_attendees x WHERE x.event_id = e.id AND x.status = 'maybe'),
	       (SELECT status FROM event_attendees x WHERE x.event_id = e.id AND x.user_id = $1),
	       e.created_at
	FROM events e LEFT JOIN communities c ON c.id = e.community_id
	WHERE ` + eventVisible

func (s *Store) queryEvents(ctx context.Context, sql string, args ...any) ([]Event, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var cID *int64
		var cSlug, cName *string
		if err := rows.Scan(&e.ID, &e.CreatorID, &cID, &cSlug, &cName, &e.Title, &e.Description, &e.Place,
			&e.StartsAt, &e.EndsAt, &e.Going, &e.Maybe, &e.MyStatus, &e.CreatedAt); err != nil {
			return nil, err
		}
		if cID != nil {
			e.Community = &CommunityRef{ID: *cID, Slug: *cSlug, Name: *cName}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type NewEvent struct {
	CreatorID   int64
	CommunityID *int64
	Title       string
	Description string
	Place       string
	StartsAt    time.Time
	EndsAt      *time.Time
}

func (s *Store) CreateEvent(ctx context.Context, e NewEvent) (int64, error) {
	var id int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO events (creator_id, community_id, title, description, place, starts_at, ends_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			e.CreatorID, e.CommunityID, e.Title, e.Description, e.Place, e.StartsAt, e.EndsAt).Scan(&id); err != nil {
			return err
		}
		// организатор идёт на своё мероприятие
		_, err := tx.Exec(ctx, `INSERT INTO event_attendees (event_id, user_id, status) VALUES ($1, $2, 'going')`, id, e.CreatorID)
		return err
	})
	return id, err
}

func (s *Store) Event(ctx context.Context, viewer, id int64) (*Event, error) {
	list, err := s.queryEvents(ctx, eventSelect+` AND e.id = $2`, viewer, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

func (s *Store) UpdateEvent(ctx context.Context, id int64, title, desc, place *string, starts *time.Time, ends **time.Time) error {
	var endsVal *time.Time
	setEnds := ends != nil
	if setEnds {
		endsVal = *ends
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE events SET title = COALESCE($2, title), description = COALESCE($3, description),
		       place = COALESCE($4, place), starts_at = COALESCE($5, starts_at),
		       ends_at = CASE WHEN $6 THEN $7 ELSE ends_at END
		WHERE id = $1`, id, title, desc, place, starts, setEnds, endsVal)
	return err
}

func (s *Store) DeleteEvent(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM events WHERE id = $1`, id)
	return err
}

func (s *Store) RSVP(ctx context.Context, event, user int64, status string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO event_attendees (event_id, user_id, status) VALUES ($1, $2, $3)
		ON CONFLICT (event_id, user_id) DO UPDATE SET status = EXCLUDED.status, updated_at = now()`, event, user, status)
	return err
}

func (s *Store) CancelRSVP(ctx context.Context, event, user int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM event_attendees WHERE event_id = $1 AND user_id = $2`, event, user)
	return err
}

func (s *Store) Attendees(ctx context.Context, event int64, status string, after int64, limit int) ([]UserCard, error) {
	return s.scanCards(ctx, `
		SELECT `+cardCols+` FROM event_attendees ea JOIN users u ON u.id = ea.user_id
		WHERE ea.event_id = $1 AND ea.status = $2 AND u.id > $3 AND u.banned_at IS NULL
		ORDER BY u.id LIMIT $4`, event, status, after, limit)
}

// UpcomingEvents: куда я иду, а также мероприятия моих сообществ.
func (s *Store) UpcomingEvents(ctx context.Context, viewer int64, limit int) ([]Event, error) {
	return s.queryEvents(ctx, eventSelect+`
		AND COALESCE(e.ends_at, e.starts_at + interval '6 hours') > now()
		AND (EXISTS (SELECT 1 FROM event_attendees x WHERE x.event_id = e.id AND x.user_id = $1)
		     OR e.community_id IN (SELECT community_id FROM community_members WHERE user_id = $1))
		ORDER BY e.starts_at LIMIT $2`, viewer, limit)
}

func (s *Store) CommunityEvents(ctx context.Context, viewer, community int64, limit int) ([]Event, error) {
	return s.queryEvents(ctx, eventSelect+`
		AND e.community_id = $2 AND COALESCE(e.ends_at, e.starts_at + interval '6 hours') > now()
		ORDER BY e.starts_at LIMIT $3`, viewer, community, limit)
}

// ---------- закрытые сообщества ----------

func (s *Store) SetCommunityVisibility(ctx context.Context, id int64, visibility string) error {
	_, err := s.pool.Exec(ctx, `UPDATE communities SET visibility = $2 WHERE id = $1`, id, visibility)
	return err
}

func (s *Store) RequestJoin(ctx context.Context, community, user int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO community_join_requests (community_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, community, user)
	return tag.RowsAffected() == 1, err
}

func (s *Store) JoinRequests(ctx context.Context, community, after int64, limit int) ([]UserCard, error) {
	return s.scanCards(ctx, `
		SELECT `+cardCols+` FROM community_join_requests r JOIN users u ON u.id = r.user_id
		WHERE r.community_id = $1 AND u.id > $2 AND u.banned_at IS NULL ORDER BY u.id LIMIT $3`, community, after, limit)
}

// ResolveJoinRequest: approve=true — принять в участники.
func (s *Store) ResolveJoinRequest(ctx context.Context, community, user int64, approve bool) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM community_join_requests WHERE community_id = $1 AND user_id = $2`, community, user)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if approve {
			_, err = tx.Exec(ctx, `
				INSERT INTO community_members (community_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, community, user)
		}
		return err
	})
}

func (s *Store) HasJoinRequest(ctx context.Context, community, user int64) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM community_join_requests WHERE community_id = $1 AND user_id = $2)`,
		community, user).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return ok, err
}
