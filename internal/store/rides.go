package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Ride struct {
	ID        int64     `json:"id"`
	Driver    UserCard  `json:"driver"`
	EventID   *int64    `json:"event_id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	DepartsAt time.Time `json:"departs_at"`
	Seats     int       `json:"seats"`
	Taken     int       `json:"taken"`   // принятые попутчики
	Pending   int       `json:"pending"` // заявки
	Note      string    `json:"note"`
	MyStatus  *string   `json:"my_status"` // pending | accepted | null
	CreatedAt time.Time `json:"created_at"`
}

type RideFields struct {
	EventID   *int64
	From, To  string
	DepartsAt time.Time
	Seats     int
	Note      string
}

// rideVisible: поездка к мероприятию — тем, кто видит мероприятие; без
// мероприятия — всем, кроме заблокированных (в любую сторону).
const rideVisible = ` u.banned_at IS NULL
	AND NOT EXISTS (SELECT 1 FROM blocks b WHERE (b.blocker_id = $1 AND b.blocked_id = r.driver_id)
	                                         OR (b.blocker_id = r.driver_id AND b.blocked_id = $1))
	AND (r.event_id IS NULL OR EXISTS (SELECT 1 FROM events e LEFT JOIN communities c ON c.id = e.community_id
	                                   WHERE e.id = r.event_id AND` + eventVisible + `))`

const rideSelect = `SELECT r.id, ` + cardCols + `, r.event_id, r.from_place, r.to_place, r.departs_at, r.seats,
	(SELECT count(*) FROM ride_requests q WHERE q.ride_id = r.id AND q.status = 'accepted'),
	(SELECT count(*) FROM ride_requests q WHERE q.ride_id = r.id AND q.status = 'pending'),
	r.note, (SELECT status FROM ride_requests q WHERE q.ride_id = r.id AND q.user_id = $1), r.created_at
	FROM rides r JOIN users u ON u.id = r.driver_id WHERE` + rideVisible

func (s *Store) queryRides(ctx context.Context, sql string, args ...any) ([]Ride, error) {
	rows, err := s.pool.Query(ctx, rideSelect+sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ride{}
	for rows.Next() {
		var r Ride
		var avatar *string
		var seats int16
		if err := rows.Scan(&r.ID, &r.Driver.ID, &r.Driver.Username, &r.Driver.FirstName, &r.Driver.LastName, &avatar,
			&r.EventID, &r.From, &r.To, &r.DepartsAt, &seats, &r.Taken, &r.Pending, &r.Note, &r.MyStatus, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Seats = int(seats)
		r.Driver.AvatarURL = MediaURL(avatar, "thumb")
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Ride(ctx context.Context, viewer, id int64) (*Ride, error) {
	list, err := s.queryRides(ctx, ` AND r.id = $2`, viewer, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// Rides — предстоящие: к мероприятию (event > 0) или все, куда — по подстроке.
func (s *Store) Rides(ctx context.Context, viewer, event int64, to string, limit int) ([]Ride, error) {
	var toArg *string
	if to != "" {
		p := likePattern(to)
		toArg = &p
	}
	return s.queryRides(ctx, ` AND r.departs_at > now() - interval '2 hours'
		AND ($2 = 0 OR r.event_id = $2) AND ($3::text IS NULL OR lower(r.to_place) LIKE $3)
		ORDER BY r.departs_at, r.id LIMIT $4`, viewer, event, toArg, limit)
}

const MaxRidesPerDay = 5

func (s *Store) CreateRide(ctx context.Context, driver int64, f RideFields) (int64, error) {
	var id int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, driver); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM rides WHERE driver_id = $1 AND created_at > now() - interval '1 day'`, driver).Scan(&n); err != nil {
			return err
		}
		if n >= MaxRidesPerDay {
			return ErrRateLimited
		}
		return tx.QueryRow(ctx, `
			INSERT INTO rides (driver_id, event_id, from_place, to_place, departs_at, seats, note)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			driver, f.EventID, f.From, f.To, f.DepartsAt, f.Seats, f.Note).Scan(&id)
	})
	return id, err
}

// DeleteRide: водитель отменяет; возвращает принятых попутчиков (уведомить).
func (s *Store) DeleteRide(ctx context.Context, id, driver int64) (riders []int64, err error) {
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT user_id FROM ride_requests WHERE ride_id = $1 AND status = 'accepted'`, id)
		if err != nil {
			return err
		}
		if riders, err = pgx.CollectRows(rows, pgx.RowTo[int64]); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM rides WHERE id = $1 AND driver_id = $2`, id, driver)
		if err == nil && tag.RowsAffected() == 0 {
			err = ErrNotFound
		}
		return err
	})
	return riders, err
}

var ErrRideFull = errors.New("no free seats")

// RequestSeat: попроситься (повторно — ничего не меняет).
func (s *Store) RequestSeat(ctx context.Context, ride, user int64, note string) (created bool, err error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO ride_requests (ride_id, user_id, note) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, ride, user, note)
	return tag.RowsAffected() == 1, err
}

// DecideSeat: водитель принимает (если есть место — под блокировкой поездки)
// или отклоняет заявку/высаживает попутчика.
func (s *Store) DecideSeat(ctx context.Context, ride, driver, user int64, accept bool) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var seats int16
		err := tx.QueryRow(ctx, `SELECT seats FROM rides WHERE id = $1 AND driver_id = $2 FOR UPDATE`, ride, driver).Scan(&seats)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !accept {
			tag, err := tx.Exec(ctx, `DELETE FROM ride_requests WHERE ride_id = $1 AND user_id = $2`, ride, user)
			if err == nil && tag.RowsAffected() == 0 {
				err = ErrNotFound
			}
			return err
		}
		var taken int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM ride_requests WHERE ride_id = $1 AND status = 'accepted'`, ride).Scan(&taken); err != nil {
			return err
		}
		if taken >= int(seats) {
			return ErrRideFull
		}
		tag, err := tx.Exec(ctx, `UPDATE ride_requests SET status = 'accepted' WHERE ride_id = $1 AND user_id = $2 AND status = 'pending'`, ride, user)
		if err == nil && tag.RowsAffected() == 0 {
			err = ErrNotFound
		}
		return err
	})
}

func (s *Store) LeaveRide(ctx context.Context, ride, user int64) (wasAccepted bool, err error) {
	var st string
	err = s.pool.QueryRow(ctx, `DELETE FROM ride_requests WHERE ride_id = $1 AND user_id = $2 RETURNING status`, ride, user).Scan(&st)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return st == "accepted", err
}

type RideRequest struct {
	User      UserCard  `json:"user"`
	Status    string    `json:"status"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

// RideRequests — водителю: заявки и принятые.
func (s *Store) RideRequests(ctx context.Context, ride int64) ([]RideRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+cardCols+`, q.status, q.note, q.created_at FROM ride_requests q JOIN users u ON u.id = q.user_id
		WHERE q.ride_id = $1 AND u.banned_at IS NULL ORDER BY q.status DESC, q.created_at`, ride)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RideRequest{}
	for rows.Next() {
		var q RideRequest
		var avatar *string
		if err := rows.Scan(&q.User.ID, &q.User.Username, &q.User.FirstName, &q.User.LastName, &avatar, &q.Status, &q.Note, &q.CreatedAt); err != nil {
			return nil, err
		}
		q.User.AvatarURL = MediaURL(avatar, "thumb")
		out = append(out, q)
	}
	return out, rows.Err()
}
