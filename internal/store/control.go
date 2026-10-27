package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Контрольный срок похода и доверенные контакты участников.

type HikeContact struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

const (
	MaxContactsPerHike = 3
	MaxContactsPerWeek = 10 // против рассылки тревожных писем на чужие адреса
)

var (
	ErrNotParticipant = errors.New("not a participant")
	ErrNoControl      = errors.New("no control deadline")
	ErrRateLimited    = errors.New("rate limited")
)

// AddHikeContact: только идущий участник похода с контрольным сроком,
// пока группа не вернулась и тревога не поднята.
func (s *Store) AddHikeContact(ctx context.Context, event, user int64, name, email string) (*HikeContact, error) {
	var c HikeContact
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var kind string
		var control, returned, alerted *time.Time
		err := tx.QueryRow(ctx, `SELECT kind, control_at, returned_at, alert_sent_at FROM events WHERE id = $1 FOR UPDATE`,
			event).Scan(&kind, &control, &returned, &alerted)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if kind != "hike" || control == nil || returned != nil || alerted != nil {
			return ErrNoControl
		}
		var going bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM event_attendees WHERE event_id = $1 AND user_id = $2 AND status = 'going')`,
			event, user).Scan(&going); err != nil {
			return err
		}
		if !going {
			return ErrNotParticipant
		}
		var here, week int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE event_id = $1), count(*) FILTER (WHERE created_at > now() - interval '7 days')
			FROM hike_contacts WHERE user_id = $2`, event, user).Scan(&here, &week); err != nil {
			return err
		}
		if here >= MaxContactsPerHike {
			return ErrTooMany
		}
		if week >= MaxContactsPerWeek {
			return ErrRateLimited
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO hike_contacts (event_id, user_id, name, email) VALUES ($1, $2, $3, $4)
			ON CONFLICT (event_id, user_id, email) DO UPDATE SET name = EXCLUDED.name
			RETURNING id, name, email, created_at`, event, user, name, email).Scan(&c.ID, &c.Name, &c.Email, &c.CreatedAt)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// HikeContacts — только свои: даже организатор не видит чужих контактов.
func (s *Store) HikeContacts(ctx context.Context, event, user int64) ([]HikeContact, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, email, created_at FROM hike_contacts WHERE event_id = $1 AND user_id = $2 ORDER BY id`, event, user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[HikeContact])
}

func (s *Store) DeleteHikeContact(ctx context.Context, id, event, user int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM hike_contacts WHERE id = $1 AND event_id = $2 AND user_id = $3`, id, event, user)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

// ContactAlert — кому и о ком писать.
type ContactAlert struct {
	Name, Email string
	Participant UserCard
}

// ControlEvent — поход, по которому наступил срок (или пришло возвращение).
type ControlEvent struct {
	ID                         int64
	Title, Place, MeetingPoint string
	StartsAt, ControlAt        time.Time
	Organizer                  UserCard
	Going                      int
	RouteTitle                 string
	Participants               []int64 // идущие
	Contacts                   []ContactAlert
}

func (s *Store) loadControlEvent(ctx context.Context, q pgx.Tx, id int64) (*ControlEvent, error) {
	e := &ControlEvent{ID: id}
	var avatar *string
	var control *time.Time
	if err := q.QueryRow(ctx, `
		SELECT e.title, e.place, e.meeting_point, e.starts_at, e.control_at,
		       u.id, u.username, u.first_name, u.last_name, u.avatar_media_id::text, COALESCE(r.title, '')
		FROM events e JOIN users u ON u.id = e.creator_id LEFT JOIN routes r ON r.id = e.route_id
		WHERE e.id = $1`, id).Scan(&e.Title, &e.Place, &e.MeetingPoint, &e.StartsAt, &control,
		&e.Organizer.ID, &e.Organizer.Username, &e.Organizer.FirstName, &e.Organizer.LastName, &avatar, &e.RouteTitle); err != nil {
		return nil, err
	}
	if control != nil {
		e.ControlAt = *control
	}
	rows, err := q.Query(ctx, `SELECT user_id FROM event_attendees WHERE event_id = $1 AND status = 'going'`, id)
	if err != nil {
		return nil, err
	}
	if e.Participants, err = pgx.CollectRows(rows, pgx.RowTo[int64]); err != nil {
		return nil, err
	}
	e.Going = len(e.Participants)
	rows, err = q.Query(ctx, `
		SELECT c.name, c.email, u.id, u.username, u.first_name, u.last_name
		FROM hike_contacts c JOIN users u ON u.id = c.user_id
		JOIN event_attendees ea ON ea.event_id = c.event_id AND ea.user_id = c.user_id AND ea.status = 'going'
		WHERE c.event_id = $1 ORDER BY c.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a ContactAlert
		if err := rows.Scan(&a.Name, &a.Email, &a.Participant.ID, &a.Participant.Username,
			&a.Participant.FirstName, &a.Participant.LastName); err != nil {
			return nil, err
		}
		e.Contacts = append(e.Contacts, a)
	}
	return e, rows.Err()
}

// MarkReturned: группа вернулась. Если тревога уже ушла — возвращает
// поход с контактами, чтобы написать «всё в порядке». Контакты удаляются.
func (s *Store) MarkReturned(ctx context.Context, id int64) (alerted *ControlEvent, err error) {
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var wasAlerted bool
		err := tx.QueryRow(ctx, `
			UPDATE events SET returned_at = now() WHERE id = $1 AND kind = 'hike' AND returned_at IS NULL
			RETURNING alert_sent_at IS NOT NULL`, id).Scan(&wasAlerted)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if wasAlerted {
			if alerted, err = s.loadControlEvent(ctx, tx, id); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `DELETE FROM hike_contacts WHERE event_id = $1`, id)
		return err
	})
	return alerted, err
}

// claimControl отмечает и возвращает походы, подошедшие под условие.
// UPDATE … RETURNING атомарен: при нескольких экземплярах API письмо уйдёт один раз.
func (s *Store) claimControl(ctx context.Context, set, where string) ([]*ControlEvent, error) {
	var out []*ControlEvent
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE events SET `+set+` = now()
			WHERE kind = 'hike' AND control_at IS NOT NULL AND returned_at IS NULL AND alert_sent_at IS NULL AND `+where+`
			RETURNING id`)
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
		if err != nil {
			return err
		}
		for _, id := range ids {
			e, err := s.loadControlEvent(ctx, tx, id)
			if err != nil {
				return err
			}
			out = append(out, e)
		}
		return nil
	})
	return out, err
}

// DueReminders: за час до срока — напомнить участникам отметиться.
func (s *Store) DueReminders(ctx context.Context) ([]*ControlEvent, error) {
	return s.claimControl(ctx, "reminder_sent_at",
		`reminder_sent_at IS NULL AND control_at > now() AND control_at <= now() + interval '1 hour'`)
}

// DueAlerts: срок прошёл, группа не отметилась.
func (s *Store) DueAlerts(ctx context.Context) ([]*ControlEvent, error) {
	return s.claimControl(ctx, "alert_sent_at", `control_at <= now()`)
}

// CleanupHikeContacts: контакты не хранятся дольше нужного.
func (s *Store) CleanupHikeContacts(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM hike_contacts c USING events e
		WHERE e.id = c.event_id AND (e.returned_at IS NOT NULL OR e.control_at IS NULL OR e.control_at < now() - interval '7 days')`)
	return tag.RowsAffected(), err
}

// GoingIDs — кто идёт в поход.
func (s *Store) GoingIDs(ctx context.Context, event int64) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `SELECT user_id FROM event_attendees WHERE event_id = $1 AND status = 'going'`, event)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}
