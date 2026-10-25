package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"socialnet/internal/store"
)

type sendMessageRequest struct {
	Body       string `json:"body"`
	Ciphertext []byte `json:"ciphertext"` // base64; для сквозного шифрования
	Nonce      []byte `json:"nonce"`
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	target, rel, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	if target.ID == me {
		writeError(w, http.StatusBadRequest, "bad_request", "cannot message yourself")
		return
	}
	var req sendMessageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.msgLimiter.allow(strconv.FormatInt(me, 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many messages, slow down")
		return
	}

	req.Body = strings.TrimSpace(req.Body)
	switch {
	case req.Ciphertext != nil:
		if req.Body != "" || len(req.Ciphertext) == 0 || len(req.Ciphertext) > 16384 ||
			len(req.Nonce) == 0 || len(req.Nonce) > 64 {
			writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
				map[string]string{"ciphertext": "encrypted message needs ciphertext (<=16 KiB) and nonce, and no body"})
			return
		}
	default:
		if n := utf8.RuneCountInString(req.Body); n < 1 || n > 4000 {
			writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
				map[string]string{"body": "1-4000 characters"})
			return
		}
	}

	if !canMessage(target.MessagePrivacy, rel) {
		writeError(w, http.StatusForbidden, "privacy", "this user does not accept messages from you")
		return
	}

	convID, err := s.store.Conversation(r.Context(), me, target.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	msg, err := s.store.SendMessage(r.Context(), store.NewMessage{
		ConversationID: convID, SenderID: me, Body: req.Body, Ciphertext: req.Ciphertext, Nonce: req.Nonce,
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	s.hub.Publish(me, "message", msg) // другие устройства отправителя
	if !s.hub.Publish(target.ID, "message", msg) {
		// получатель не в приложении: push без текста сообщения (он может быть зашифрован)
		if sender, err := s.store.UserByID(r.Context(), me); err == nil {
			s.push(target.ID, pushPayload{Type: "message", Title: "Новое сообщение",
				Body: "Сообщение от " + sender.FirstName, URL: "/messages/" + strconv.FormatInt(convID, 10)})
		}
	} else {
		// получатель онлайн: сообщение доставлено
		if up, err := s.store.MarkDelivered(r.Context(), convID, target.ID, msg.ID); err == nil {
			s.hub.Publish(me, "delivered", map[string]int64{"conversation_id": convID, "user_id": target.ID, "up_to": up})
		}
	}
	writeJSON(w, http.StatusCreated, msg)
}

func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r)
	items, err := s.store.Conversations(r.Context(), userIDFrom(r.Context()), p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, func(c store.ConversationItem) int64 { return c.LastMessage.ID }))
}

// conversation проверяет участие в диалоге {id} и возвращает собеседника.
func (s *Server) conversation(w http.ResponseWriter, r *http.Request) (convID, peer int64, ok bool) {
	convID, ok = pathID(w, r, "id")
	if !ok {
		return 0, 0, false
	}
	peer, err := s.store.ConversationPeer(r.Context(), convID, userIDFrom(r.Context()))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "conversation not found")
		return 0, 0, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return 0, 0, false
	}
	return convID, peer, true
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	convID, _, ok := s.conversation(w, r)
	if !ok {
		return
	}
	p := pageParams(r)
	items, err := s.store.Messages(r.Context(), convID, p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, func(m store.Message) int64 { return m.ID }))
}

func (s *Server) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	convID, peer, ok := s.conversation(w, r)
	if !ok {
		return
	}
	var req struct {
		UpTo int64 `json:"up_to"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	me := userIDFrom(r.Context())
	up, err := s.store.MarkRead(r.Context(), convID, me, req.UpTo)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.hub.Publish(peer, "read", map[string]int64{"conversation_id": convID, "user_id": me, "up_to": up})
	writeJSON(w, http.StatusOK, map[string]int64{"last_read_id": up})
}

func (s *Server) handleTyping(w http.ResponseWriter, r *http.Request) {
	convID, peer, ok := s.conversation(w, r)
	if !ok {
		return
	}
	s.hub.Publish(peer, "typing", map[string]int64{"conversation_id": convID, "user_id": userIDFrom(r.Context())})
	w.WriteHeader(http.StatusNoContent)
}

// ---------- ключи для сквозного шифрования ----------

func (s *Server) handleSetKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IdentityKey []byte `json:"identity_key"` // 32 байта X25519, base64
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.IdentityKey) != 32 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"identity_key": "must be 32 bytes (X25519 public key), base64"})
		return
	}
	if err := s.store.SetIdentityKey(r.Context(), userIDFrom(r.Context()), req.IdentityKey); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetKey(w http.ResponseWriter, r *http.Request) {
	target, rel, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	if rel.Blocked() {
		writeError(w, http.StatusNotFound, "not_found", "no key")
		return
	}
	key, updated, err := s.store.IdentityKey(r.Context(), target.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "user has no key")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": target.ID, "identity_key": key, "updated_at": updated})
}
