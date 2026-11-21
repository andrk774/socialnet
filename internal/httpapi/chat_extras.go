package httpapi

import (
	"net/http"
	"strings"
	"unicode/utf8"
)

func (s *Server) handleGetPin(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.conversation(w, r)
	if !ok {
		return
	}
	items, e := s.store.PinnedMessages(r.Context(), conv.ID)
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) handleSetPin(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.conversation(w, r)
	if !ok {
		return
	}
	if conv.Kind == "group" && conv.MyRole == "member" {
		writeError(w, 403, "forbidden", "only chat administrators can pin messages")
		return
	}
	var req struct {
		MessageID int64 `json:"message_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.MessageID < 0 {
		writeError(w, 422, "validation", "invalid message")
		return
	}
	if req.MessageID == 0 {
		if _, e := s.store.Pool().Exec(r.Context(), `DELETE FROM conversation_pins WHERE conversation_id=$1`, conv.ID); e != nil {
			s.internalError(w, r, e)
			return
		}
	} else {
		tag, e := s.store.Pool().Exec(r.Context(), `INSERT INTO conversation_pins(conversation_id,message_id,pinned_by) SELECT $1,id,$3 FROM messages WHERE id=$2 AND conversation_id=$1 AND kind='text' ON CONFLICT(conversation_id) DO UPDATE SET message_id=EXCLUDED.message_id,pinned_by=EXCLUDED.pinned_by,created_at=now()`, conv.ID, req.MessageID, userIDFrom(r.Context()))
		if e != nil {
			s.internalError(w, r, e)
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, 404, "not_found", "message not found in conversation")
			return
		}
	}
	for _, uid := range conv.Members {
		s.hub.Publish(uid, "conversation_pin", map[string]int64{"conversation_id": conv.ID})
	}
	w.WriteHeader(204)
}
func (s *Server) handleStickers(w http.ResponseWriter, r *http.Request) {
	rows, e := s.store.Pool().Query(r.Context(), `SELECT id::text,title FROM custom_stickers WHERE owner_id=$1 ORDER BY created_at DESC LIMIT 500`, userIDFrom(r.Context()))
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var id, title string
		if e = rows.Scan(&id, &title); e != nil {
			s.internalError(w, r, e)
			return
		}
		items = append(items, map[string]string{"id": id, "title": title, "url": "/media/" + id + "/full?acl=2", "thumb_url": "/media/" + id + "/thumb?acl=2"})
	}
	if e = rows.Err(); e != nil {
		s.internalError(w, r, e)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) handleCreateSticker(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if !validUUID(req.ID) || req.Title == "" || utf8.RuneCountInString(req.Title) > 64 {
		writeError(w, 422, "validation", "image and title (1-64 characters) required")
		return
	}
	tag, e := s.store.Pool().Exec(r.Context(), `INSERT INTO custom_stickers(id,owner_id,title) SELECT id,owner_id,$3 FROM media WHERE id=$1::uuid AND owner_id=$2 ON CONFLICT(id) DO UPDATE SET title=EXCLUDED.title`, req.ID, userIDFrom(r.Context()), req.Title)
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "not_found", "own image required")
		return
	}
	w.WriteHeader(204)
}
func (s *Server) handleDeleteSticker(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validUUID(req.ID) {
		writeError(w, 422, "validation", "invalid id")
		return
	}
	_, e := s.store.Pool().Exec(r.Context(), `DELETE FROM custom_stickers WHERE id=$1::uuid AND owner_id=$2`, req.ID, userIDFrom(r.Context()))
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	w.WriteHeader(204)
}
