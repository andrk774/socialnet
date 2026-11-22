package httpapi

import (
	"math"
	"net/http"
)

// Only rounded locations are stored. Presence expires unless explicitly renewed.
func (s *Server) handleNearby(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, me := r.Context(), userIDFrom(r.Context())
	pool := s.store.Pool()
	if r.Method == http.MethodDelete {
		if _, err := pool.Exec(ctx, `DELETE FROM nearby_presence WHERE user_id=$1`, me); err != nil {
			s.internalError(w, r, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"visible": false})
		return
	}
	if r.Method == http.MethodPost {
		var q struct {
			Lat     *float64 `json:"lat"`
			Lon     *float64 `json:"lon"`
			Consent bool     `json:"consent"`
		}
		if !decodeJSON(w, r, &q) {
			return
		}
		if !q.Consent || q.Lat == nil || q.Lon == nil || !validNearbyCoordinates(*q.Lat, *q.Lon) {
			writeError(w, 422, "validation_failed", "Location and explicit consent required")
			return
		}
		_, err := pool.Exec(ctx, `INSERT INTO nearby_presence(user_id,lat,lon,expires_at) VALUES($1,$2,$3,now()+interval '15 minutes') ON CONFLICT(user_id) DO UPDATE SET lat=excluded.lat,lon=excluded.lon,expires_at=excluded.expires_at`, me, math.Round(*q.Lat*100)/100, math.Round(*q.Lon*100)/100)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	var visible bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nearby_presence WHERE user_id=$1 AND expires_at>now())`, me).Scan(&visible); err != nil {
		s.internalError(w, r, err)
		return
	}
	type person struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		First    string `json:"first_name"`
		Last     string `json:"last_name"`
		Distance string `json:"distance"`
	}
	people := []person{}
	rows, err := pool.Query(ctx, `SELECT u.id,u.username,u.first_name,u.last_name,
 6371*acos(least(1.0,greatest(-1.0,sin(radians(a.lat))*sin(radians(b.lat))+cos(radians(a.lat))*cos(radians(b.lat))*cos(radians(a.lon-b.lon))))) AS km
 FROM nearby_presence a JOIN nearby_presence b ON b.user_id<>a.user_id JOIN users u ON u.id=b.user_id
 WHERE a.user_id=$1 AND a.expires_at>now() AND b.expires_at>now() AND u.banned_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM blocks WHERE (blocker_id=$1 AND blocked_id=u.id) OR (blocked_id=$1 AND blocker_id=u.id))
 ORDER BY km,u.id LIMIT 100`, me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var p person
		var km float64
		if err := rows.Scan(&p.ID, &p.Username, &p.First, &p.Last, &km); err != nil {
			s.internalError(w, r, err)
			return
		}
		if km > 25 {
			continue
		}
		switch {
		case km < 2:
			p.Distance = "до 2 км"
		case km < 5:
			p.Distance = "2–5 км"
		case km < 10:
			p.Distance = "5–10 км"
		default:
			p.Distance = "10–25 км"
		}
		people = append(people, p)
	}
	if err := rows.Err(); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"visible": visible, "people": people})
}

func validNearbyCoordinates(lat, lon float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lon) && !math.IsInf(lat, 0) && !math.IsInf(lon, 0) && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}
