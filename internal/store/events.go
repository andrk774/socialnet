package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Event struct {
	ID           int64         `json:"id"`
	CreatorID    int64         `json:"creator_id"`
	Community    *CommunityRef `json:"community,omitempty"`
	Kind         string        `json:"kind"` // event | hike
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	Place        string        `json:"place"`
	StartsAt     time.Time     `json:"starts_at"`
	EndsAt       *time.Time    `json:"ends_at,omitempty"`
	Difficulty   *int          `json:"difficulty"` // 1..5
	Capacity     *int          `json:"capacity"`   // null — без ограничения
	Gear         string        `json:"gear"`
	RouteID      *string       `json:"route_id"`
	MeetingPoint string        `json:"meeting_point"`
	Approval     bool          `json:"approval"` // участие по одобрению организатора
	Going        int           `json:"going_count"`
	Maybe        int           `json:"maybe_count"`
	Pending      int           `json:"pending_count"`
	Waitlist     int           `json:"waitlist_count"`
	MyStatus     *string       `json:"my_status"` // going | maybe | pending | waitlist | null
	MyPosition   *int          `json:"my_waitlist_position,omitempty"`
	ControlAt    *time.Time    `json:"control_at"`  // контрольный срок возвращения
	ReturnedAt   *time.Time    `json:"returned_at"` // группа отметила, что вернулась
	AlertSent    bool          `json:"alert_sent"`  // срок прошёл — доверенным контактам ушли письма
	CreatedAt    time.Time     `json:"created_at"`
}

// eventVisible: мероприятие закрытого сообщества — только участникам.
const eventVisible = `
	(e.community_id IS NULL OR c.visibility = 'open'
	 OR EXISTS (SELECT 1 FROM community_members m WHERE m.community_id = e.community_id AND m.user_id = $1))
	AND (c.id IS NULL OR c.hidden_at IS NULL)`

const eventSelect = `
	SELECT e.id, e.creator_id, c.id, c.slug, c.name, e.kind, e.title, e.description, e.place, e.starts_at, e.ends_at,
	       e.difficulty, e.capacity, e.gear, e.route_id::text, e.meeting_point, e.approval,
	       (SELECT count(*) FROM event_attendees x WHERE x.event_id = e.id AND x.status = 'going'),
	       (SELECT count(*) FROM event_attendees x WHERE x.event_id = e.id AND x.status = 'maybe'),
	       (SELECT count(*) FROM event_attendees x WHERE x.event_id = e.id AND x.status = 'pending'),
	       (SELECT count(*) FROM event_attendees x WHERE x.event_id = e.id AND x.status = 'waitlist'),
	       me.status,
	       CASE WHEN me.status = 'waitlist' THEN (SELECT count(*) FROM event_attendees x
	            WHERE x.event_id = e.id AND x.status = 'waitlist'
	              AND (x.updated_at, x.user_id) <= (me.updated_at, me.user_id)) END,
	       e.control_at, e.returned_at, e.alert_sent_at IS NOT NULL, e.created_at
	FROM events e LEFT JOIN communities c ON c.id = e.community_id
	LEFT JOIN event_attendees me ON me.event_id = e.id AND me.user_id = $1
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
		var diff *int16
		var pos *int64
		if err := rows.Scan(&e.ID, &e.CreatorID, &cID, &cSlug, &cName, &e.Kind, &e.Title, &e.Description, &e.Place,
			&e.StartsAt, &e.EndsAt, &diff, &e.Capacity, &e.Gear, &e.RouteID, &e.MeetingPoint, &e.Approval,
			&e.Going, &e.Maybe, &e.Pending, &e.Waitlist, &e.MyStatus, &pos,
			&e.ControlAt, &e.ReturnedAt, &e.AlertSent, &e.CreatedAt); err != nil {
			return nil, err
		}
		if cID != nil {
			e.Community = &CommunityRef{ID: *cID, Slug: *cSlug, Name: *cName}
		}
		if diff != nil {
			d := int(*diff)
			e.Difficulty = &d
		}
		if pos != nil {
			p := int(*pos)
			e.MyPosition = &p
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EventFields — всё, что задаёт организатор (кроме сообщества).
type EventFields struct {
	Kind         string
	Title        string
	Description  string
	Place        string
	StartsAt     time.Time
	EndsAt       *time.Time
	Difficulty   *int
	Capacity     *int
	Gear         string
	RouteID      *string
	MeetingPoint string
	Approval     bool
	ControlAt    *time.Time
}

type NewEvent struct {
	EventFields
	CreatorID   int64
	CommunityID *int64
}

func (s *Store) CreateEvent(ctx context.Context, e NewEvent) (int64, error) {
	var id int64
	if e.Kind == "" {
		e.Kind = "event"
	}
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO events (creator_id, community_id, kind, title, description, place, starts_at, ends_at,
			                    difficulty, capacity, gear, route_id, meeting_point, approval, control_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::uuid, $13, $14, $15) RETURNING id`,
			e.CreatorID, e.CommunityID, e.Kind, e.Title, e.Description, e.Place, e.StartsAt, e.EndsAt,
			e.Difficulty, e.Capacity, e.Gear, e.RouteID, e.MeetingPoint, e.Approval, e.ControlAt).Scan(&id); err != nil {
			return err
		}
		// организатор идёт на своё мероприятие и занимает место
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

// UpdateEvent сохраняет всё сразу; если мест стало больше (или ограничение
// снято) — из листа ожидания переходят следующие. Возвращает, кто перешёл.
func (s *Store) UpdateEvent(ctx context.Context, id int64, f EventFields) (promoted []int64, err error) {
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE events SET title = $2, description = $3, place = $4, starts_at = $5, ends_at = $6,
			       difficulty = $7, capacity = $8, gear = $9, route_id = $10::uuid, meeting_point = $11, approval = $12,
			       control_at = $13,
			       reminder_sent_at = CASE WHEN control_at IS DISTINCT FROM $13 THEN NULL ELSE reminder_sent_at END
			WHERE id = $1`, id, f.Title, f.Description, f.Place, f.StartsAt, f.EndsAt,
			f.Difficulty, f.Capacity, f.Gear, f.RouteID, f.MeetingPoint, f.Approval, f.ControlAt)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		promoted, err = promoteWaitlist(ctx, tx, id)
		return err
	})
	return promoted, err
}

func (s *Store) DeleteEvent(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM events WHERE id = $1`, id)
	return err
}

// lockEvent: строка мероприятия под FOR UPDATE — все решения о местах идут
// по очереди, и два последних желающих не займут одно место.
func lockEvent(ctx context.Context, tx pgx.Tx, id int64) (capacity *int, approval bool, kind string, err error) {
	err = tx.QueryRow(ctx, `SELECT capacity, approval, kind FROM events WHERE id = $1 FOR UPDATE`, id).Scan(&capacity, &approval, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return
}

// hasSeat: есть ли свободное место (под блокировкой мероприятия).
func hasSeat(ctx context.Context, tx pgx.Tx, id int64, capacity *int) (bool, error) {
	if capacity == nil {
		return true, nil
	}
	var going int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM event_attendees WHERE event_id = $1 AND status = 'going'`, id).Scan(&going)
	return going < *capacity, err
}

// promoteWaitlist: свободные места — первым в очереди. Вызывать под lockEvent
// (или в той же транзакции после UPDATE events, который тоже блокирует строку).
func promoteWaitlist(ctx context.Context, tx pgx.Tx, id int64) ([]int64, error) {
	rows, err := tx.Query(ctx, `
		WITH ev AS (SELECT capacity FROM events WHERE id = $1),
		free AS (SELECT CASE WHEN ev.capacity IS NULL THEN NULL
		                     ELSE GREATEST(ev.capacity - (SELECT count(*) FROM event_attendees
		                          WHERE event_id = $1 AND status = 'going'), 0) END AS n FROM ev),
		next AS (SELECT user_id FROM event_attendees WHERE event_id = $1 AND status = 'waitlist'
		         ORDER BY updated_at, user_id LIMIT (SELECT n FROM free))
		UPDATE event_attendees a SET status = 'going', updated_at = now()
		FROM next WHERE a.event_id = $1 AND a.user_id = next.user_id
		RETURNING a.user_id`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

var ErrNoMaybe = errors.New("hikes have no 'maybe'")

// Attend: going — занять место (или встать в очередь, или подать заявку, если
// нужно одобрение); maybe — «возможно» (не для походов). Повторный going
// ничего не меняет: место в очереди и заявка сохраняются. Возвращает итоговый
// статус и тех, кто перешёл из очереди на освободившееся место.
func (s *Store) Attend(ctx context.Context, event, user int64, status, note string) (result string, promoted []int64, err error) {
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		capacity, approval, kind, err := lockEvent(ctx, tx, event)
		if err != nil {
			return err
		}
		var prev string
		if err := tx.QueryRow(ctx, `SELECT status FROM event_attendees WHERE event_id = $1 AND user_id = $2`,
			event, user).Scan(&prev); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if status == "maybe" {
			if kind == "hike" {
				return ErrNoMaybe
			}
			result = "maybe"
		} else {
			switch prev {
			case "going", "pending", "waitlist":
				result = prev
			default:
				if approval {
					result = "pending"
				} else if ok, err := hasSeat(ctx, tx, event, capacity); err != nil {
					return err
				} else if ok {
					result = "going"
				} else {
					result = "waitlist"
				}
			}
		}
		if result != prev {
			if _, err := tx.Exec(ctx, `
				INSERT INTO event_attendees (event_id, user_id, status, note) VALUES ($1, $2, $3, $4)
				ON CONFLICT (event_id, user_id) DO UPDATE SET status = EXCLUDED.status, note = EXCLUDED.note, updated_at = now()`,
				event, user, result, note); err != nil {
				return err
			}
		}
		if prev == "going" && result != "going" {
			promoted, err = promoteWaitlist(ctx, tx, event)
		}
		return err
	})
	return result, promoted, err
}

// Leave: участник уходит сам или его убирает организатор. Освободившееся
// место — следующему в очереди. ErrNotFound — его и не было.
func (s *Store) Leave(ctx context.Context, event, user int64) (promoted []int64, err error) {
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if _, _, _, err := lockEvent(ctx, tx, event); err != nil {
			return err
		}
		var prev string
		err := tx.QueryRow(ctx, `DELETE FROM event_attendees WHERE event_id = $1 AND user_id = $2 RETURNING status`,
			event, user).Scan(&prev)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if prev == "going" {
			promoted, err = promoteWaitlist(ctx, tx, event)
		}
		return err
	})
	return promoted, err
}

// DecideApplication: организатор принимает заявку (место или очередь, если
// мест уже нет) или отклоняет её. ErrNotFound — заявки нет.
func (s *Store) DecideApplication(ctx context.Context, event, user int64, approve bool) (result string, err error) {
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		capacity, _, _, err := lockEvent(ctx, tx, event)
		if err != nil {
			return err
		}
		var prev string
		err = tx.QueryRow(ctx, `SELECT status FROM event_attendees WHERE event_id = $1 AND user_id = $2 FOR UPDATE`,
			event, user).Scan(&prev)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && prev != "pending" {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !approve {
			result = "rejected"
			_, err = tx.Exec(ctx, `DELETE FROM event_attendees WHERE event_id = $1 AND user_id = $2`, event, user)
			return err
		}
		result = "waitlist"
		if ok, err := hasSeat(ctx, tx, event, capacity); err != nil {
			return err
		} else if ok {
			result = "going"
		}
		_, err = tx.Exec(ctx, `UPDATE event_attendees SET status = $3, updated_at = now() WHERE event_id = $1 AND user_id = $2`,
			event, user, result)
		return err
	})
	return result, err
}

type Application struct {
	User      UserCard  `json:"user"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

// Applications: заявки, ждущие решения, — первыми самые ранние.
func (s *Store) Applications(ctx context.Context, event int64) ([]Application, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.username, u.first_name, u.last_name, u.avatar_media_id::text, ea.note, ea.updated_at
		FROM event_attendees ea JOIN users u ON u.id = ea.user_id
		WHERE ea.event_id = $1 AND ea.status = 'pending' AND u.banned_at IS NULL
		ORDER BY ea.updated_at, u.id LIMIT 500`, event)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Application{}
	for rows.Next() {
		var a Application
		var avatar *string
		if err := rows.Scan(&a.User.ID, &a.User.Username, &a.User.FirstName, &a.User.LastName, &avatar, &a.Note, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.User.AvatarURL = MediaURL(avatar, "thumb")
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Attendees(ctx context.Context, event int64, status string, after int64, limit int) ([]UserCard, error) {
	return s.scanCards(ctx, `
		SELECT `+cardCols+` FROM event_attendees ea JOIN users u ON u.id = ea.user_id
		WHERE ea.event_id = $1 AND ea.status = $2 AND u.id > $3 AND u.banned_at IS NULL
		ORDER BY u.id LIMIT $4`, event, status, after, limit)
}

// UpcomingEvents: куда я иду (или в очереди, или подал заявку), а также
// мероприятия моих сообществ.
func (s *Store) UpcomingEvents(ctx context.Context, viewer int64, limit int) ([]Event, error) {
	return s.queryEvents(ctx, eventSelect+`
		AND COALESCE(e.ends_at, e.starts_at + interval '6 hours') > now()
		AND (me.user_id IS NOT NULL
		     OR e.community_id IN (SELECT community_id FROM community_members WHERE user_id = $1))
		ORDER BY e.starts_at LIMIT $2`, viewer, limit)
}

func (s *Store) CommunityEvents(ctx context.Context, viewer, community int64, limit int) ([]Event, error) {
	return s.queryEvents(ctx, eventSelect+`
		AND e.community_id = $2 AND COALESCE(e.ends_at, e.starts_at + interval '6 hours') > now()
		ORDER BY e.starts_at LIMIT $3`, viewer, community, limit)
}

// Hikes: ближайшие походы, которые видит пользователь; maxDifficulty 0 — любые.
func (s *Store) Hikes(ctx context.Context, viewer int64, maxDifficulty, limit int) ([]Event, error) {
	return s.queryEvents(ctx, eventSelect+`
		AND e.kind = 'hike' AND COALESCE(e.ends_at, e.starts_at + interval '6 hours') > now()
		AND ($2 = 0 OR e.difficulty IS NULL OR e.difficulty <= $2)
		AND NOT EXISTS (SELECT 1 FROM blocks b WHERE (b.blocker_id = $1 AND b.blocked_id = e.creator_id)
		                                         OR (b.blocker_id = e.creator_id AND b.blocked_id = $1))
		ORDER BY e.starts_at, e.id LIMIT $3`, viewer, maxDifficulty, limit)
}

// EventAttendeeStatus: статус пользователя (пусто — не участвует).
func (s *Store) EventAttendeeStatus(ctx context.Context, event, user int64) (string, error) {
	var st string
	err := s.pool.QueryRow(ctx, `SELECT status FROM event_attendees WHERE event_id = $1 AND user_id = $2`, event, user).Scan(&st)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return st, err
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
