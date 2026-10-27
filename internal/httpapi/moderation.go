package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/store"
)

// ---------- жалобы пользователей ----------

func (s *Server) handleCreateReport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetType string `json:"target_type"`
		TargetID   int64  `json:"target_id"`
		TargetUUID string `json:"target_uuid"` // для track, video и document
		Reason     string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	errs := map[string]string{}
	oneOf(errs, "target_type", &req.TargetType, "post", "comment", "user", "community", "track", "video", "document", "route", "companion", "route_point", "article")
	var targetUUID *string
	if req.TargetType == "track" || req.TargetType == "video" || req.TargetType == "document" || req.TargetType == "route" {
		u := strings.ToLower(req.TargetUUID)
		if !validUUID(u) {
			errs["target_uuid"] = "required for tracks, videos, documents and routes"
		}
		targetUUID, req.TargetID = &u, 0
	}
	if n := utf8.RuneCountInString(req.Reason); n < 1 || n > 1000 {
		errs["reason"] = "1-1000 characters"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := userIDFrom(r.Context())
	// жаловаться можно только на то, что видишь
	if req.TargetType == "post" {
		if _, err := s.store.PostForViewer(r.Context(), me, req.TargetID); err != nil {
			writeError(w, http.StatusNotFound, "not_found", "target not found")
			return
		}
	}
	if req.TargetType == "document" { // документ — только тот, к которому есть доступ
		if ok, err := s.store.DocumentAccess(r.Context(), me, *targetUUID); err != nil {
			s.internalError(w, r, err)
			return
		} else if !ok {
			writeError(w, http.StatusNotFound, "not_found", "target not found")
			return
		}
	}
	if req.TargetType == "companion" { // объявление — только видимое (не от заблокированных)
		if _, err := s.store.CompanionAd(r.Context(), me, req.TargetID); err != nil {
			writeError(w, http.StatusNotFound, "not_found", "target not found")
			return
		}
	}
	if req.TargetType == "article" { // статья — только видимая
		if _, err := s.store.Article(r.Context(), me, req.TargetID); err != nil {
			writeError(w, http.StatusNotFound, "not_found", "target not found")
			return
		}
	}
	if req.TargetType == "route_point" { // точка — только на видимом маршруте
		rid, err := s.store.RoutePointRoute(r.Context(), req.TargetID)
		ok := err == nil
		if ok {
			if ok, err = s.store.RouteAccess(r.Context(), me, rid); err != nil {
				s.internalError(w, r, err)
				return
			}
		}
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "target not found")
			return
		}
	}
	if req.TargetType == "route" { // маршрут — только тот, что виден
		if ok, err := s.store.RouteAccess(r.Context(), me, *targetUUID); err != nil {
			s.internalError(w, r, err)
			return
		} else if !ok {
			writeError(w, http.StatusNotFound, "not_found", "target not found")
			return
		}
	}
	var author int64
	var err error
	if targetUUID != nil {
		author, err = s.store.TargetAuthorUUID(r.Context(), req.TargetType, *targetUUID)
	} else {
		author, err = s.store.TargetAuthor(r.Context(), req.TargetType, req.TargetID)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "target not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if author == me && req.TargetType == "user" {
		writeError(w, http.StatusBadRequest, "bad_request", "cannot report yourself")
		return
	}
	id, err := s.store.CreateReport(r.Context(), me, req.TargetType, req.TargetID, targetUUID, req.Reason)
	if errors.Is(err, store.ErrDuplicate) {
		writeError(w, http.StatusConflict, "duplicate", "you have already reported this")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "open"})
}

// ---------- панель модератора ----------

func (s *Server) handleModReports(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "open"
	}
	if status != "open" && status != "resolved" && status != "dismissed" {
		writeError(w, http.StatusBadRequest, "bad_request", "unknown status")
		return
	}
	p := pageParams(r)
	items, err := s.store.Reports(r.Context(), status, p.After, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, func(x store.Report) int64 { return x.ID }))
}

// canBan: модератор банит только обычных пользователей, админ — ещё и модераторов.
// Админа не может забанить никто (снимите роль через /admin сначала).
func canBan(actorRole, targetRole string) bool {
	switch targetRole {
	case "user":
		return true
	case "moderator":
		return actorRole == "admin"
	}
	return false
}

// handleResolveReport: {"action": "dismiss" | "hide" | "ban", "note": "..."}
//   - hide — скрыть пост, комментарий или сообщество;
//   - ban  — забанить пользователя (или автора контента).
func (s *Server) handleResolveReport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Action string `json:"action"`
		Note   string `json:"note"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if utf8.RuneCountInString(req.Note) > 1000 {
		writeError(w, http.StatusBadRequest, "bad_request", "note too long")
		return
	}
	rep, err := s.store.Report(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "report not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if rep.Status != "open" {
		writeError(w, http.StatusConflict, "already_handled", "report already handled")
		return
	}
	mod := viewerFrom(r.Context())
	status := "resolved"

	switch req.Action {
	case "dismiss":
		status = "dismissed"
	case "hide":
		if rep.TargetType == "user" {
			writeError(w, http.StatusBadRequest, "bad_request", "users can be banned, not hidden")
			return
		}
		var err error
		if rep.TargetUUID != nil {
			// трек или видео: снятие + блок-лист по хэшу, чтобы не перезалили
			err = s.store.Takedown(r.Context(), rep.TargetType, *rep.TargetUUID, true, req.Note)
		} else {
			err = s.store.SetHidden(r.Context(), rep.TargetType, rep.TargetID, true)
		}
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			s.internalError(w, r, err)
			return
		}
	case "ban":
		var author int64
		var err error
		if rep.TargetUUID != nil {
			author, err = s.store.TargetAuthorUUID(r.Context(), rep.TargetType, *rep.TargetUUID)
		} else {
			author, err = s.store.TargetAuthor(r.Context(), rep.TargetType, rep.TargetID)
		}
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "target no longer exists")
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if !s.ban(w, r, mod, author, req.Note) {
			return
		}
	default:
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"action": "must be one of: dismiss, hide, ban"})
		return
	}

	if err := s.store.CloseReports(r.Context(), rep.TargetType, rep.TargetID, rep.TargetUUID, mod.ID, status); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.modLogUUID(r, req.Action, rep.TargetType, rep.TargetID, rep.TargetUUID, &rep.ID, req.Note)
	w.WriteHeader(http.StatusNoContent)
}

// ban: проверка прав, бан, запись в журнал. Отвечает сам при ошибке.
func (s *Server) ban(w http.ResponseWriter, r *http.Request, mod viewer, userID int64, reason string) bool {
	target, err := s.store.UserByID(r.Context(), userID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return false
	}
	if err != nil {
		s.internalError(w, r, err)
		return false
	}
	if target.ID == mod.ID || !canBan(mod.Role, target.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "you cannot ban this user")
		return false
	}
	if err := s.store.Ban(r.Context(), userID, strings.TrimSpace(reason)); err != nil {
		s.internalError(w, r, err)
		return false
	}
	return true
}

func (s *Server) modLog(r *http.Request, action, typ string, id int64, reportID *int64, note string) {
	s.modLogUUID(r, action, typ, id, nil, reportID, note)
}

func (s *Server) modLogUUID(r *http.Request, action, typ string, id int64, uuid *string, reportID *int64, note string) {
	err := s.store.LogModeration(r.Context(), store.ModAction{
		ModeratorID: userIDFrom(r.Context()), Action: action, TargetType: typ, TargetID: id,
		TargetUUID: uuid, ReportID: reportID, Note: note,
	})
	if err != nil {
		s.log.Error("moderation log", "err", err)
	}
}

func (s *Server) handleBan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.ban(w, r, viewerFrom(r.Context()), id, req.Reason) {
		return
	}
	s.modLog(r, "ban", "user", id, nil, req.Reason)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnban(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.Unban(r.Context(), id); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.modLog(r, "unban", "user", id, nil, "")
	w.WriteHeader(http.StatusNoContent)
}

// handleSetHidden: POST /mod/{type}/{id}/hide и /unhide, type: posts|comments|communities.
func (s *Server) handleSetHidden(hidden bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		action := "hide"
		if !hidden {
			action = "unhide"
		}
		// треки и видео: id — UUID, снятие с блок-листом
		if typ := map[string]string{"tracks": "track", "videos": "video", "documents": "document", "routes": "route"}[chi.URLParam(r, "type")]; typ != "" {
			uid := strings.ToLower(chi.URLParam(r, "id"))
			if !validUUID(uid) {
				writeError(w, http.StatusNotFound, "not_found", "not found")
				return
			}
			err := s.store.Takedown(r.Context(), typ, uid, hidden, "")
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "not found")
				return
			}
			if err != nil {
				s.internalError(w, r, err)
				return
			}
			s.modLogUUID(r, action, typ, 0, &uid, nil, "")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		typ := map[string]string{"posts": "post", "comments": "comment", "communities": "community", "companions": "companion", "route-points": "route_point", "articles": "article"}[chi.URLParam(r, "type")]
		id, ok := pathID(w, r, "id")
		if !ok || typ == "" {
			if ok {
				writeError(w, http.StatusNotFound, "not_found", "not found")
			}
			return
		}
		err := s.store.SetHidden(r.Context(), typ, id, hidden)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		s.modLog(r, action, typ, id, nil, "")
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleModLog(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r)
	items, err := s.store.ModerationLog(r.Context(), p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, func(a store.ModAction) int64 { return a.ID }))
}

// handleSetRole (только админ): {"role": "user" | "moderator" | "admin"}.
func (s *Server) handleSetRole(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	oneOf(errs, "role", &req.Role, "user", "moderator", "admin")
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if id == userIDFrom(r.Context()) {
		writeError(w, http.StatusBadRequest, "bad_request", "cannot change your own role")
		return
	}
	err := s.store.SetRole(r.Context(), id, req.Role)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.modLog(r, "set_role:"+req.Role, "user", id, nil, "")
	w.WriteHeader(http.StatusNoContent)
}

// ---------- security.txt и приём уязвимостей ----------

// handleSecurityTxt: /.well-known/security.txt по RFC 9116.
func (s *Server) handleSecurityTxt(w http.ResponseWriter, _ *http.Request) {
	expires := time.Now().UTC().AddDate(1, 0, 0).Truncate(24 * time.Hour).Format(time.RFC3339)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(
		"Contact: mailto:" + s.cfg.SecurityContact + "\n" +
			"Contact: " + s.cfg.AppURL + "/security/report\n" +
			"Expires: " + expires + "\n" +
			"Preferred-Languages: ru, en\n" +
			"Policy: " + s.cfg.AppURL + "/security/policy\n" +
			"Canonical: " + s.cfg.AppURL + "/.well-known/security.txt\n"))
}

func (s *Server) handleVulnReport(w http.ResponseWriter, r *http.Request) {
	if !s.vulnLimiter.allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many reports, try again later")
		return
	}
	var req struct {
		Contact     string `json:"contact"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Contact, req.Title = strings.TrimSpace(req.Contact), strings.TrimSpace(req.Title)
	errs := map[string]string{}
	if n := utf8.RuneCountInString(req.Contact); n < 3 || n > 254 {
		errs["contact"] = "email or other contact, 3-254 characters"
	}
	if n := utf8.RuneCountInString(req.Title); n < 1 || n > 200 {
		errs["title"] = "1-200 characters"
	}
	if n := utf8.RuneCountInString(req.Description); n < 1 || n > 20000 {
		errs["description"] = "1-20000 characters"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	id, err := s.store.CreateVulnReport(r.Context(), req.Contact, req.Title, req.Description, sessionMeta(r).IP)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	// Текст отчёта в письмо не вставляем: он от анонима и может содержать что угодно.
	s.sendMail(s.cfg.SecurityContact, "Новое сообщение об уязвимости",
		"Поступило новое сообщение об уязвимости. Откройте панель администратора, чтобы прочитать его.\n")
	writeJSON(w, http.StatusAccepted, map[string]any{"id": id, "message": "thank you, we will contact you"})
}

func (s *Server) handleVulnList(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r)
	items, err := s.store.VulnReports(r.Context(), p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, func(v store.VulnReport) int64 { return v.ID }))
}

func (s *Server) handleVulnStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	oneOf(errs, "status", &req.Status, "new", "triaged", "fixed", "rejected")
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	err := s.store.SetVulnStatus(r.Context(), id, req.Status)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
