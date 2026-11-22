package httpapi

import (
	"net/http"
	"strings"
	"unicode/utf8"
)

func (s *Server) handleEditMessage(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.conversation(w, r)
	if !ok {
		return
	}
	mid, ok := pathID(w, r, "mid")
	if !ok {
		return
	}
	body := "Сообщение удалено"
	deleting := r.Method == http.MethodDelete
	if !deleting {
		var req struct {
			Body string `json:"body"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		body = strings.TrimSpace(req.Body)
		if utf8.RuneCountInString(body) < 1 || utf8.RuneCountInString(body) > 4000 {
			writeError(w, 422, "validation_failed", "body must be 1-4000 characters")
			return
		}
	}
	tx, err := s.store.Pool().Begin(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), `UPDATE messages SET body=$1,edited_at=now(),deleted_at=CASE WHEN $5 THEN now() ELSE NULL END WHERE id=$2 AND conversation_id=$3 AND sender_id=$4 AND kind='text' AND ciphertext IS NULL AND deleted_at IS NULL`, body, mid, conv.ID, userIDFrom(r.Context()), deleting)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if tag.RowsAffected() != 1 {
		writeError(w, 404, "not_found", "editable message not found")
		return
	}
	if deleting {
		if _, err = tx.Exec(r.Context(), `DELETE FROM message_attachments WHERE message_id=$1`, mid); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.internalError(w, r, err)
		return
	}
	msg, err := s.store.MessageByID(r.Context(), mid)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	for _, uid := range conv.Members {
		s.hub.Publish(uid, "message_updated", msg)
	}
	writeJSON(w, 200, msg)
}
