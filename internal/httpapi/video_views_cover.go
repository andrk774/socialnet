package httpapi

import (
	"net/http"
)

func (s *Server) handleVideoView(w http.ResponseWriter, r *http.Request) {
	v, ok := s.video(w, r, false)
	if !ok {
		return
	}
	if v.Status != "ready" {
		writeError(w, 409, "not_ready", "video not ready")
		return
	}
	_, e := s.store.Pool().Exec(r.Context(), `INSERT INTO video_views(video_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, v.ID, userIDFrom(r.Context()))
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	var n int64
	e = s.store.Pool().QueryRow(r.Context(), `SELECT count(*) FROM video_views WHERE video_id=$1`, v.ID).Scan(&n)
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	writeJSON(w, 200, map[string]any{"views": n})
}
func (s *Server) handleGetCover(w http.ResponseWriter, r *http.Request) {
	u, _, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	var id string
	e := s.store.Pool().QueryRow(r.Context(), `SELECT coalesce((SELECT media_id::text FROM profile_covers WHERE user_id=$1),'')`, u.ID).Scan(&id)
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	link := ""
	if id != "" {
		link = "/media/" + id + "/full"
	}
	writeJSON(w, 200, map[string]string{"url": link})
}
func (s *Server) handlePutCover(w http.ResponseWriter, r *http.Request) {
	var q struct {
		MediaID *string `json:"media_id"`
	}
	if !decodeJSON(w, r, &q) {
		return
	}
	me := userIDFrom(r.Context())
	if q.MediaID == nil {
		_, e := s.store.Pool().Exec(r.Context(), `DELETE FROM profile_covers WHERE user_id=$1`, me)
		if e != nil {
			s.internalError(w, r, e)
			return
		}
	} else {
		var exists bool
		e := s.store.Pool().QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM media WHERE id::text=$1 AND owner_id=$2)`, *q.MediaID, me).Scan(&exists)
		if e != nil {
			s.internalError(w, r, e)
			return
		}
		if !exists {
			writeError(w, 404, "not_found", "media not found")
			return
		}
		_, e = s.store.Pool().Exec(r.Context(), `INSERT INTO profile_covers(user_id,media_id) VALUES($1,$2::uuid) ON CONFLICT(user_id) DO UPDATE SET media_id=excluded.media_id`, me, *q.MediaID)
		if e != nil {
			s.internalError(w, r, e)
			return
		}
	}
	w.WriteHeader(204)
}
