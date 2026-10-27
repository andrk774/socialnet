package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// LatLon — точка на карте: [широта, долгота].
type LatLon [2]float64

type RouteWaypoint struct {
	Lat  float64  `json:"lat"`
	Lon  float64  `json:"lon"`
	Ele  *float64 `json:"ele,omitempty"`
	Name string   `json:"name"`
}

type ProfilePoint struct {
	D float64 `json:"d"` // пройдено, м
	E float64 `json:"e"` // высота, м
}

// Route — маршрут. Detail (подробная геометрия) отдаётся только в карточке
// маршрута, в ленте хватает Preview.
type Route struct {
	ID          string          `json:"id"`
	OwnerID     int64           `json:"owner_id"`
	Title       string          `json:"title"`
	DistanceM   int             `json:"distance_m"`
	GainM       int             `json:"gain_m"`
	LossM       int             `json:"loss_m"`
	MinEle      *float64        `json:"min_ele"`
	MaxEle      *float64        `json:"max_ele"`
	DurationSec int             `json:"duration_sec"`
	MovingSec   int             `json:"moving_sec"`
	Points      int             `json:"points"`
	HiddenEndsM int             `json:"hidden_ends_m"`
	BBox        [4]float64      `json:"bbox"` // min_lat, min_lon, max_lat, max_lon
	Preview     []LatLon        `json:"preview"`
	Profile     []ProfilePoint  `json:"profile"`
	Detail      [][]LatLon      `json:"detail,omitempty"`
	Waypoints   []RouteWaypoint `json:"waypoints,omitempty"`
	Marks       []RoutePoint    `json:"marks,omitempty"` // точки от ходивших: родники, стоянки…
	GPXURL      string          `json:"gpx_url"`
	Public      bool            `json:"public"`      // в открытом каталоге /trails
	PublicNote  string          `json:"public_note"` // описание для каталога
	CreatedAt   time.Time       `json:"created_at"`
	seq         int64
}

func (r Route) Cursor() int64 { return r.seq }

func RouteGPXURL(id string) string { return "/api/v1/routes/" + id + "/gpx" }

const routeCols = `r.id::text, r.owner_id, r.title, r.distance_m, r.gain_m, r.loss_m, r.min_ele, r.max_ele,
	r.duration_sec, r.moving_sec, r.points, r.hidden_ends_m, r.min_lat, r.min_lon, r.max_lat, r.max_lon,
	r.preview, r.profile, r.created_at, r.seq, r.public, r.public_note`

func scanRoutes(rows pgx.Rows, withDetail bool) ([]Route, error) {
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Route, error) {
		var r Route
		var minEle, maxEle *float32
		var preview, profile, detail, wps []byte
		dest := []any{&r.ID, &r.OwnerID, &r.Title, &r.DistanceM, &r.GainM, &r.LossM, &minEle, &maxEle,
			&r.DurationSec, &r.MovingSec, &r.Points, &r.HiddenEndsM, &r.BBox[0], &r.BBox[1], &r.BBox[2], &r.BBox[3],
			&preview, &profile, &r.CreatedAt, &r.seq, &r.Public, &r.PublicNote}
		if withDetail {
			dest = append(dest, &detail, &wps)
		}
		if err := row.Scan(dest...); err != nil {
			return r, err
		}
		return r, fillRoute(&r, minEle, maxEle, preview, profile, detail, wps)
	})
}

func fillRoute(r *Route, minEle, maxEle *float32, preview, profile, detail, wps []byte) error {
	if minEle != nil {
		v := float64(*minEle)
		r.MinEle = &v
	}
	if maxEle != nil {
		v := float64(*maxEle)
		r.MaxEle = &v
	}
	if err := json.Unmarshal(preview, &r.Preview); err != nil {
		return err
	}
	if err := json.Unmarshal(profile, &r.Profile); err != nil {
		return err
	}
	if detail != nil {
		if err := json.Unmarshal(detail, &r.Detail); err != nil {
			return err
		}
		if err := json.Unmarshal(wps, &r.Waypoints); err != nil {
			return err
		}
	}
	r.GPXURL = RouteGPXURL(r.ID)
	return nil
}

type NewRoute struct {
	ID          string
	Owner       int64
	Title       string
	DistanceM   int
	GainM       int
	LossM       int
	MinEle      *float64
	MaxEle      *float64
	DurationSec int
	MovingSec   int
	Points      int
	HiddenEndsM int
	BBox        [4]float64
	Preview     []LatLon
	Detail      [][]LatLon
	Profile     []ProfilePoint
	Waypoints   []RouteWaypoint
	SHA256      []byte
}

func (s *Store) CreateRoute(ctx context.Context, n NewRoute) (*Route, error) {
	preview, _ := json.Marshal(n.Preview)
	detail, _ := json.Marshal(n.Detail)
	profile, _ := json.Marshal(n.Profile)
	if n.Profile == nil {
		profile = []byte("[]")
	}
	if n.Waypoints == nil {
		n.Waypoints = []RouteWaypoint{}
	}
	wps, _ := json.Marshal(n.Waypoints)
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO routes (id, owner_id, title, distance_m, gain_m, loss_m, min_ele, max_ele, duration_sec, moving_sec,
		                    points, hidden_ends_m, min_lat, min_lon, max_lat, max_lon, preview, detail, profile, waypoints, sha256)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)`,
		n.ID, n.Owner, n.Title, n.DistanceM, n.GainM, n.LossM, n.MinEle, n.MaxEle, n.DurationSec, n.MovingSec,
		n.Points, n.HiddenEndsM, n.BBox[0], n.BBox[1], n.BBox[2], n.BBox[3], preview, detail, profile, wps, n.SHA256); err != nil {
		return nil, err
	}
	return s.Route(ctx, n.ID, true)
}

// Route — живой маршрут (не удалён и не снят модератором).
func (s *Store) Route(ctx context.Context, id string, withDetail bool) (*Route, error) {
	cols := routeCols
	if withDetail {
		cols += `, r.detail, r.waypoints`
	}
	rows, err := s.pool.Query(ctx, `SELECT `+cols+` FROM routes r
		WHERE r.id = $1::uuid AND r.deleted_at IS NULL AND r.hidden_at IS NULL`, id)
	if err != nil {
		return nil, err
	}
	list, err := scanRoutes(rows, withDetail)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

func (s *Store) MyRoutes(ctx context.Context, owner, before int64, limit int) ([]Route, error) {
	if before <= 0 {
		before = 1<<63 - 1
	}
	rows, err := s.pool.Query(ctx, `SELECT `+routeCols+` FROM routes r
		WHERE r.owner_id = $1 AND r.deleted_at IS NULL AND r.hidden_at IS NULL AND r.seq < $2
		ORDER BY r.seq DESC LIMIT $3`, owner, before, limit)
	if err != nil {
		return nil, err
	}
	return scanRoutes(rows, false)
}

func (s *Store) RenameRoute(ctx context.Context, id string, owner int64, title string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE routes SET title = $3 WHERE id = $1::uuid AND owner_id = $2 AND deleted_at IS NULL`,
		id, owner, title)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// SetRoutePublic: владелец показывает маршрут в открытом каталоге или убирает.
func (s *Store) SetRoutePublic(ctx context.Context, id string, owner int64, public bool, note string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE routes SET public = $3, public_note = $4,
		       public_at = CASE WHEN $3 THEN COALESCE(public_at, now()) ELSE NULL END
		WHERE id = $1::uuid AND owner_id = $2 AND deleted_at IS NULL`, id, owner, public, note)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// PublicRoute — маршрут из каталога с автором (без входа).
type PublicRoute struct {
	Route
	Author   UserCard
	PublicAt time.Time
}

const publicWhere = ` r.public AND r.deleted_at IS NULL AND r.hidden_at IS NULL AND u.banned_at IS NULL`

func (s *Store) scanPublic(ctx context.Context, withDetail bool, sql string, args ...any) ([]PublicRoute, error) {
	cols := routeCols
	if withDetail {
		cols += `, r.detail, r.waypoints`
	}
	rows, err := s.pool.Query(ctx, `SELECT `+cols+`, `+cardCols+`, r.public_at FROM routes r JOIN users u ON u.id = r.owner_id WHERE`+publicWhere+sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PublicRoute
	for rows.Next() {
		var pr PublicRoute
		var minEle, maxEle *float32
		var preview, profile, detail, wps []byte
		var avatar *string
		dest := []any{&pr.ID, &pr.OwnerID, &pr.Title, &pr.DistanceM, &pr.GainM, &pr.LossM, &minEle, &maxEle,
			&pr.DurationSec, &pr.MovingSec, &pr.Points, &pr.HiddenEndsM, &pr.BBox[0], &pr.BBox[1], &pr.BBox[2], &pr.BBox[3],
			&preview, &profile, &pr.CreatedAt, &pr.seq, &pr.Public, &pr.PublicNote}
		if withDetail {
			dest = append(dest, &detail, &wps)
		}
		dest = append(dest, &pr.Author.ID, &pr.Author.Username, &pr.Author.FirstName, &pr.Author.LastName, &avatar, &pr.PublicAt)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		if err := fillRoute(&pr.Route, minEle, maxEle, preview, profile, detail, wps); err != nil {
			return nil, err
		}
		pr.Author.AvatarURL = MediaURL(avatar, "thumb")
		out = append(out, pr)
	}
	return out, rows.Err()
}

func (s *Store) PublicRoutes(ctx context.Context, limit, offset int) ([]PublicRoute, error) {
	return s.scanPublic(ctx, false, ` ORDER BY r.public_at DESC, r.id LIMIT $1 OFFSET $2`, limit, offset)
}

func (s *Store) PublicRouteByID(ctx context.Context, id string) (*PublicRoute, error) {
	list, err := s.scanPublic(ctx, true, ` AND r.id = $1::uuid`, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// PublicRouteIDs — для sitemap.xml.
func (s *Store) PublicRouteIDs(ctx context.Context, limit int) ([]PublicRoute, error) {
	return s.scanPublic(ctx, false, ` ORDER BY r.public_at DESC LIMIT $1`, limit)
}

func (s *Store) DeleteRoute(ctx context.Context, id string, owner int64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE routes SET deleted_at = now() WHERE id = $1::uuid AND owner_id = $2 AND deleted_at IS NULL`,
		id, owner)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// RouteAccess: маршрут видит владелец и тот, кому видна запись с этим маршрутом.
func (s *Store) RouteAccess(ctx context.Context, viewer int64, id string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM routes WHERE id = $2::uuid AND owner_id = $1)
		    OR EXISTS (SELECT 1 FROM post_routes pr
		               JOIN posts p ON p.id = pr.post_id
		               JOIN users a ON a.id = p.author_id
		               LEFT JOIN communities c ON c.id = p.community_id
		               WHERE pr.route_id = $2::uuid AND `+postVisible+`)
		    OR EXISTS (SELECT 1 FROM routes r JOIN users u ON u.id = r.owner_id WHERE r.id = $2::uuid AND`+publicWhere+`)
		    OR EXISTS (SELECT 1 FROM articles a JOIN users u ON u.id = a.author_id
		               WHERE a.route_id = $2::uuid AND a.status = 'published' AND`+articleVisible+`)
		    OR EXISTS (SELECT 1 FROM events e LEFT JOIN communities c ON c.id = e.community_id
		               WHERE e.route_id = $2::uuid AND `+eventVisible+`)`, viewer, id).Scan(&ok)
	return ok, err
}
