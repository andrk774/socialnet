package httpapi

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"net/http"
	"socialnet/internal/store"
	"strconv"
	"strings"
	"time"
)

func validCallSDP(s string) bool { return len(s) > 0 && len(s) <= 64000 && strings.HasPrefix(s, "v=0") }

func (s *Server) handleCallConfig(w http.ResponseWriter, r *http.Request) {
	settings, err := s.loadTURN(r.Context())
	if err != nil {
		writeError(w, 503, "unavailable", "cannot load call configuration")
		return
	}
	servers := []map[string]any{}
	if url := settings.STUN; strings.HasPrefix(url, "stun:") {
		servers = append(servers, map[string]any{"urls": url})
	}
	relay := false
	if url, secret := settings.TURN, settings.Secret; settings.Enabled && secret != "" && (strings.HasPrefix(url, "turn:") || strings.HasPrefix(url, "turns:")) {
		username := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + ":" + strconv.FormatInt(userIDFrom(r.Context()), 10)
		mac := hmac.New(sha1.New, []byte(secret))
		mac.Write([]byte(username))
		servers = append(servers, map[string]any{"urls": url, "username": username, "credential": base64.StdEncoding.EncodeToString(mac.Sum(nil))})
		relay = true
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"ice_servers": servers, "relay_configured": relay})
}

func (s *Server) handleCreateCall(w http.ResponseWriter, r *http.Request) {
	target, rel, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	uid := userIDFrom(r.Context())
	if uid == target.ID || viewerFrom(r.Context()).Bot != 0 {
		writeError(w, 403, "forbidden", "cannot call this user")
		return
	}
	allowed, err := s.mayMessage(r.Context(), uid, target.ID, target.MessagePrivacy, rel)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !allowed {
		writeError(w, 403, "privacy", "calls are restricted by message privacy")
		return
	}
	if !s.msgLimiter.allow(strconv.FormatInt(uid, 10)) {
		writeError(w, 429, "rate_limited", "try later")
		return
	}
	var req struct {
		Video bool   `json:"video"`
		Offer string `json:"offer"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validCallSDP(req.Offer) {
		writeError(w, 422, "validation_failed", "invalid offer")
		return
	}
	id, err := s.store.CreateCall(r.Context(), uid, target.ID, req.Video, req.Offer)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, 409, "busy", "one participant is already in a call")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.hub.Publish(target.ID, "call", map[string]int64{"id": id})
	writeJSON(w, 201, map[string]int64{"id": id})
}

func (s *Server) handleCalls(w http.ResponseWriter, r *http.Request) {
	var before int64
	if raw := r.URL.Query().Get("before"); raw != "" {
		var err error
		before, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || before < 0 {
			writeError(w, 400, "bad_request", "invalid cursor")
			return
		}
	}
	items, err := s.store.Calls(r.Context(), userIDFrom(r.Context()), 0, before)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	var next *int64
	if len(items) == 100 {
		last := items[len(items)-1].ID
		next = &last
	}
	writeJSON(w, 200, map[string]any{"items": items, "next_cursor": next})
}

func (s *Server) handleCall(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	items, err := s.store.Calls(r.Context(), userIDFrom(r.Context()), id, 0)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if len(items) == 0 {
		writeError(w, 404, "not_found", "call not found")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, items[0])
}

func (s *Server) handleCallAction(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	uid := userIDFrom(r.Context())
	items, err := s.store.Calls(r.Context(), uid, id, 0)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if len(items) == 0 {
		writeError(w, 404, "not_found", "call not found")
		return
	}
	var req struct {
		Action string `json:"action"`
		Answer string `json:"answer"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Action != "answer" && req.Action != "decline" && req.Action != "end" && req.Action != "heartbeat" {
		writeError(w, 422, "validation_failed", "invalid action")
		return
	}
	if req.Action == "answer" {
		if !validCallSDP(req.Answer) {
			writeError(w, 422, "validation_failed", "invalid answer")
			return
		}
		// Recheck both-direction blocking at acceptance, including blocks added after ringing.
		relation, e := s.store.Relationship(r.Context(), uid, items[0].Peer.ID)
		if e != nil {
			s.internalError(w, r, e)
			return
		}
		if relation.Blocked() {
			writeError(w, 403, "blocked", "call blocked")
			return
		}
		callee, e := s.store.UserByID(r.Context(), items[0].CalleeID)
		if e != nil {
			s.internalError(w, r, e)
			return
		}
		callerRel, e := s.store.Relationship(r.Context(), items[0].CallerID, items[0].CalleeID)
		if e != nil {
			s.internalError(w, r, e)
			return
		}
		allowed, e := s.mayMessage(r.Context(), items[0].CallerID, items[0].CalleeID, callee.MessagePrivacy, callerRel)
		if e != nil {
			s.internalError(w, r, e)
			return
		}
		if !allowed {
			writeError(w, 403, "privacy", "calls restricted")
			return
		}
	} else {
		req.Answer = ""
	}
	err = s.store.CallAction(r.Context(), uid, id, req.Action, req.Answer)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, 409, "conflict", "call state changed")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if req.Action != "heartbeat" {
		s.hub.Publish(items[0].Peer.ID, "call", map[string]int64{"id": id})
	}
	w.WriteHeader(204)
}

func (s *Server) handleClearCallHistory(w http.ResponseWriter, r *http.Request) {
	if err := s.store.ClearCallHistory(r.Context(), userIDFrom(r.Context())); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(204)
}
