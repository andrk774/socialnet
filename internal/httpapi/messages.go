package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/av"
	"socialnet/internal/store"
)

type attachmentReq struct {
	Kind string `json:"kind"` // photo | track | voice
	ID   string `json:"id"`
}

type sendMessageRequest struct {
	Body        string          `json:"body"`
	Ciphertext  []byte          `json:"ciphertext"` // base64; для сквозного шифрования
	Nonce       []byte          `json:"nonce"`
	Attachments []attachmentReq `json:"attachments"`
}

// validate проверяет сообщение. group=true — сквозное шифрование недоступно.
func (req *sendMessageRequest) validate(group bool) map[string]string {
	errs := map[string]string{}
	req.Body = strings.TrimSpace(req.Body)
	if len(req.Attachments) > 10 {
		errs["attachments"] = "up to 10 attachments"
	}
	for i := range req.Attachments {
		a := &req.Attachments[i]
		a.ID = strings.ToLower(a.ID)
		if (a.Kind != "photo" && a.Kind != "track" && a.Kind != "voice") || !validUUID(a.ID) {
			errs["attachments"] = "each attachment needs kind photo|track|voice and a valid id"
		}
	}
	switch {
	case req.Ciphertext != nil:
		if group {
			errs["ciphertext"] = "end-to-end encryption is available only in direct conversations"
		} else if req.Body != "" || len(req.Attachments) > 0 || len(req.Ciphertext) == 0 || len(req.Ciphertext) > 16384 ||
			len(req.Nonce) == 0 || len(req.Nonce) > 64 {
			errs["ciphertext"] = "encrypted message needs ciphertext (<=16 KiB) and nonce, and no body or attachments"
		}
	default:
		n := utf8.RuneCountInString(req.Body)
		if n > 4000 {
			errs["body"] = "max 4000 characters"
		}
		if n == 0 && len(req.Attachments) == 0 {
			errs["body"] = "message needs text or attachments"
		}
	}
	return errs
}

// handleSendMessage: POST /users/{id}/messages — в личный диалог (создаётся при первом сообщении).
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
	if errs := req.validate(false); len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if !canMessage(target.MessagePrivacy, rel) {
		writeError(w, http.StatusForbidden, "privacy", "this user does not accept messages from you")
		return
	}
	// Бот не пишет первым — только отвечает тем, кто ему написал.
	if viewerFrom(r.Context()).Bot != 0 {
		wrote, err := s.store.UserWroteTo(r.Context(), target.ID, me)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if !wrote {
			writeError(w, http.StatusForbidden, "bot_cannot_initiate", "bots can only reply to users who wrote first")
			return
		}
	}
	convID, err := s.store.Conversation(r.Context(), me, target.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	conv, err := s.store.ConvAccess(r.Context(), convID, me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.deliver(w, r, conv, req)
}

// handleSendToConversation: POST /conversations/{id}/messages — в любой свой диалог.
func (s *Server) handleSendToConversation(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.conversation(w, r)
	if !ok {
		return
	}
	var req sendMessageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if errs := req.validate(conv.Kind == "group"); len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := userIDFrom(r.Context())
	if conv.Kind == "direct" {
		// в личном диалоге правила собеседника (блокировка, приватность) действуют всегда
		peer, err := s.store.UserByID(r.Context(), conv.Peer(me))
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		rel, err := s.store.Relationship(r.Context(), me, peer.ID)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if peer.BannedAt != nil || !canMessage(peer.MessagePrivacy, rel) {
			writeError(w, http.StatusForbidden, "privacy", "this user does not accept messages from you")
			return
		}
	}
	s.deliver(w, r, conv, req)
}

// deliver сохраняет сообщение и рассылает его участникам.
func (s *Server) deliver(w http.ResponseWriter, r *http.Request, conv *store.ConvInfo, req sendMessageRequest) {
	me := userIDFrom(r.Context())
	if !s.msgLimiter.allow(strconv.FormatInt(me, 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many messages, slow down")
		return
	}
	atts := make([]store.AttachmentRef, len(req.Attachments))
	for i, a := range req.Attachments {
		atts[i] = store.AttachmentRef{Kind: a.Kind, ID: a.ID}
	}
	msg, err := s.store.SendMessage(r.Context(), store.NewMessage{ConversationID: conv.ID, SenderID: me,
		Body: req.Body, Ciphertext: req.Ciphertext, Nonce: req.Nonce, Attachments: atts})
	if errors.Is(err, store.ErrBadMedia) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"attachments": "unknown attachment, or photo/voice is not yours"})
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.broadcast(r.Context(), conv, me, msg)
	s.emitBotMessages(r.Context(), conv, me, msg)
	writeJSON(w, http.StatusCreated, msg)
}

// emitBotMessages: сообщение боту (или в беседу с ботом) — событие message_new его сообществу.
func (s *Server) emitBotMessages(ctx context.Context, conv *store.ConvInfo, sender int64, msg *store.Message) {
	for _, uid := range conv.Members {
		if uid == sender {
			continue
		}
		community, err := s.store.BotCommunity(ctx, uid)
		if err != nil {
			s.log.Warn("bot lookup", "err", err)
			continue
		}
		if community != nil {
			s.emit(ctx, *community, "message_new", map[string]any{"message": msg, "conversation_kind": conv.Kind})
		}
	}
}

// broadcast: всем участникам; тем, кто не в приложении, — push без текста.
func (s *Server) broadcast(ctx context.Context, conv *store.ConvInfo, sender int64, msg *store.Message) {
	s.hub.Publish(sender, "message", msg) // другие устройства отправителя
	var senderName string
	for _, uid := range conv.Members {
		if uid == sender {
			continue
		}
		if s.hub.Publish(uid, "message", msg) {
			if conv.Kind == "direct" {
				if up, err := s.store.MarkDelivered(ctx, conv.ID, uid, msg.ID); err == nil {
					s.hub.Publish(sender, "delivered", map[string]int64{"conversation_id": conv.ID, "user_id": uid, "up_to": up})
				}
			}
			continue
		}
		if msg.Kind == "service" {
			continue
		}
		if senderName == "" {
			if u, err := s.store.UserByID(ctx, sender); err == nil {
				senderName = u.FirstName
			}
		}
		body := "Сообщение от " + senderName
		if conv.Kind == "group" {
			body = senderName + " в «" + conv.Title + "»"
		}
		s.push(uid, pushPayload{Type: "message", Title: "Новое сообщение", Body: body,
			URL: "/messages/" + strconv.FormatInt(conv.ID, 10)})
	}
}

// serviceMessage — «Анна добавила Бориса» и подобные.
func (s *Server) serviceMessage(ctx context.Context, conv *store.ConvInfo, actor int64, text string) {
	msg, err := s.store.SendMessage(ctx, store.NewMessage{ConversationID: conv.ID, SenderID: actor, Kind: "service", Body: text})
	if err != nil {
		s.log.Error("service message", "err", err)
		return
	}
	s.broadcast(ctx, conv, actor, msg)
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

// conversation проверяет участие в диалоге {id}. Чужой — 404.
func (s *Server) conversation(w http.ResponseWriter, r *http.Request) (*store.ConvInfo, bool) {
	convID, ok := pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	conv, err := s.store.ConvAccess(r.Context(), convID, userIDFrom(r.Context()))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "conversation not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	return conv, true
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.conversation(w, r)
	if !ok {
		return
	}
	p := pageParams(r)
	items, err := s.store.Messages(r.Context(), conv.ID, p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, func(m store.Message) int64 { return m.ID }))
}

func (s *Server) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.conversation(w, r)
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
	up, err := s.store.MarkRead(r.Context(), conv.ID, me, req.UpTo)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	for _, uid := range conv.Members {
		if uid != me {
			s.hub.Publish(uid, "read", map[string]int64{"conversation_id": conv.ID, "user_id": me, "up_to": up})
		}
	}
	writeJSON(w, http.StatusOK, map[string]int64{"last_read_id": up})
}

func (s *Server) handleTyping(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.conversation(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	for _, uid := range conv.Members {
		if uid != me {
			s.hub.Publish(uid, "typing", map[string]int64{"conversation_id": conv.ID, "user_id": me})
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- групповые чаты ----------

// canInvite: в группу можно добавить того, кому можно написать
// (не в блокировке и его настройки приватности сообщений это разрешают).
func (s *Server) canInvite(ctx context.Context, inviter, target int64) (bool, error) {
	u, err := s.store.UserByID(ctx, target)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rel, err := s.store.Relationship(ctx, inviter, target)
	if err != nil {
		return false, err
	}
	return u.BannedAt == nil && canMessage(u.MessagePrivacy, rel), nil
}

func (s *Server) handleCreateChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title     string  `json:"title"`
		MemberIDs []int64 `json:"member_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	title := cleanText(req.Title, 100)
	errs := map[string]string{}
	if title == "" {
		errs["title"] = "1-100 characters"
	}
	if len(req.MemberIDs) == 0 || len(req.MemberIDs) > 100 {
		errs["member_ids"] = "1-100 members to start with"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := userIDFrom(r.Context())
	for _, id := range req.MemberIDs {
		if id == me {
			continue
		}
		ok, err := s.canInvite(r.Context(), me, id)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if !ok {
			writeFieldErrors(w, http.StatusForbidden, "privacy", "cannot add some users",
				map[string]string{"member_ids": "user " + strconv.FormatInt(id, 10) + " cannot be added"})
			return
		}
	}
	id, err := s.store.CreateGroup(r.Context(), me, title, req.MemberIDs)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	conv, err := s.store.ConvAccess(r.Context(), id, me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.serviceMessage(r.Context(), conv, me, "создал(а) беседу «"+title+"»")
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "title": title, "members_count": len(conv.Members)})
}

func (s *Server) groupConv(w http.ResponseWriter, r *http.Request) (*store.ConvInfo, bool) {
	conv, ok := s.conversation(w, r)
	if ok && conv.Kind != "group" {
		writeError(w, http.StatusBadRequest, "bad_request", "not a group chat")
		return nil, false
	}
	return conv, ok
}

func (s *Server) handleChatMembers(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.conversation(w, r)
	if !ok {
		return
	}
	items, err := s.store.ChatMembers(r.Context(), conv.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleRenameChat(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.groupConv(w, r)
	if !ok {
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	title := cleanText(req.Title, 100)
	if title == "" {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"title": "1-100 characters"})
		return
	}
	if conv.MyRole == "member" {
		writeError(w, http.StatusForbidden, "forbidden", "only admins can rename the chat")
		return
	}
	if err := s.store.SetChatTitle(r.Context(), conv.ID, title); err != nil {
		s.internalError(w, r, err)
		return
	}
	conv.Title = title
	s.serviceMessage(r.Context(), conv, userIDFrom(r.Context()), "изменил(а) название на «"+title+"»")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddChatMember(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.groupConv(w, r)
	if !ok {
		return
	}
	var req struct {
		UserID int64 `json:"user_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	me := userIDFrom(r.Context())
	if conv.MyRole == "member" {
		writeError(w, http.StatusForbidden, "forbidden", "only admins can add members")
		return
	}
	allowed, err := s.canInvite(r.Context(), me, req.UserID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "privacy", "this user cannot be added")
		return
	}
	added, err := s.store.AddChatMember(r.Context(), conv.ID, req.UserID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !added {
		writeError(w, http.StatusConflict, "conflict", "already a member or the chat is full")
		return
	}
	conv.Members = append(conv.Members, req.UserID)
	if u, err := s.store.UserByID(r.Context(), req.UserID); err == nil {
		s.serviceMessage(r.Context(), conv, me, "добавил(а) "+u.FirstName)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveChatMember: выйти самому или исключить (админ — участников,
// владелец — и админов).
func (s *Server) handleRemoveChatMember(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.groupConv(w, r)
	if !ok {
		return
	}
	uid, ok := pathID(w, r, "uid")
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	if uid != me {
		members, err := s.store.ChatMembers(r.Context(), conv.ID)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		target := ""
		for _, m := range members {
			if m.ID == uid {
				target = m.Role
			}
		}
		allowed := (conv.MyRole == "owner" && target != "") || (conv.MyRole == "admin" && target == "member")
		if target == "" {
			writeError(w, http.StatusNotFound, "not_found", "member not found")
			return
		}
		if !allowed {
			writeError(w, http.StatusForbidden, "forbidden", "not allowed")
			return
		}
	}
	if err := s.store.RemoveChatMember(r.Context(), conv.ID, uid); err != nil {
		s.internalError(w, r, err)
		return
	}
	text := "покинул(а) беседу"
	if uid != me {
		if u, err := s.store.UserByID(r.Context(), uid); err == nil {
			text = "исключил(а) " + u.FirstName
		}
	}
	s.serviceMessage(r.Context(), conv, me, text)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetChatRole(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.groupConv(w, r)
	if !ok {
		return
	}
	uid, ok := pathID(w, r, "uid")
	if !ok {
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Role != "admin" && req.Role != "member" {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"role": "admin or member"})
		return
	}
	if conv.MyRole != "owner" {
		writeError(w, http.StatusForbidden, "forbidden", "only the owner can change roles")
		return
	}
	err := s.store.SetChatRole(r.Context(), conv.ID, uid, req.Role)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "member not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- голосовые сообщения ----------

const (
	maxVoiceBytes   = 5 << 20
	maxVoiceSeconds = 5 * 60
)

func (s *Server) handleUploadVoice(w http.ResponseWriter, r *http.Request) {
	if !av.Available() {
		writeError(w, http.StatusServiceUnavailable, "media_disabled", "media processing is not available on this server")
		return
	}
	me := userIDFrom(r.Context())
	if !s.uploadLimiter.allow(strconv.FormatInt(me, 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many uploads, slow down")
		return
	}
	up, ok := s.receiveUpload(w, r, maxVoiceBytes)
	if !ok {
		return
	}
	defer up.cleanup()
	probe, err := av.ProbeFile(r.Context(), up.Path)
	if errors.Is(err, av.ErrUnsupported) {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type", "supported: OGG/Opus, WEBM, M4A, MP3, WAV")
		return
	}
	if err != nil || probe.Audio() == nil || probe.Seconds < 0.5 || probe.Seconds > maxVoiceSeconds {
		writeError(w, http.StatusUnprocessableEntity, "bad_file", "voice message must be audio up to 5 minutes")
		return
	}
	release, ok := s.acquireAV(w)
	if !ok {
		return
	}
	defer release()
	id := av.NewID()
	dir, _ := s.avStore.Dir("voices", id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		s.internalError(w, r, err)
		return
	}
	out, _ := s.avStore.Path("voices", id, "voice.mp3")
	if err := av.TranscodeVoice(r.Context(), up.Path, out, probe.Format, maxVoiceSeconds); err != nil {
		_ = s.avStore.Delete("voices", id)
		writeError(w, http.StatusUnprocessableEntity, "bad_file", "cannot process the audio")
		return
	}
	secs := int(probe.Seconds + 0.5)
	if secs < 1 {
		secs = 1
	}
	if err := s.store.CreateVoice(r.Context(), id, me, secs); err != nil {
		_ = s.avStore.Delete("voices", id)
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "duration_sec": secs, "url": store.VoiceURL(id)})
}

// handleServeVoice: GET /api/v1/voice/{id} — только автору и участникам чата,
// где голосовое отправлено. Плеер получает его через fetch с токеном.
func (s *Server) handleServeVoice(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(chi.URLParam(r, "vid"))
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	ok, err := s.store.VoiceAccess(r.Context(), id, userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=3600") // личное — не в общие кэши
	s.serveAVFile(w, r, "voices", id, "voice.mp3", "audio/mpeg")
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
