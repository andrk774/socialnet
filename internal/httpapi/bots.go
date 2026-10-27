package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"socialnet/internal/store"
)

// ---------- токены бота сообщества ----------

// communityAdmin: сообщество, где текущий пользователь — админ или владелец.
func (s *Server) communityAdmin(w http.ResponseWriter, r *http.Request) (*store.Community, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	c, err := s.store.CommunityByID(r.Context(), userIDFrom(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && c.Hidden) {
		writeError(w, http.StatusNotFound, "not_found", "community not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	if !store.RoleAtLeast(c.MyRole, "admin") || viewerFrom(r.Context()).Bot != 0 {
		writeError(w, http.StatusForbidden, "forbidden", "only community admins can do this")
		return nil, false
	}
	return c, true
}

func (s *Server) handleCreateBotToken(w http.ResponseWriter, r *http.Request) {
	c, ok := s.communityAdmin(w, r)
	if !ok {
		return
	}
	var req struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	scopes, bad := parseScopes(strings.Join(req.Scopes, " "), botScopes)
	if bad != "" || len(scopes) == 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"scopes": "choose from: " + strings.Join(botScopes, ", ")})
		return
	}
	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	bot, err := s.store.EnsureBot(r.Context(), c.ID, c.Name, hex.EncodeToString(suffix))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if n, err := s.store.CountBotTokens(r.Context(), bot); err != nil || n >= 10 {
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		writeError(w, http.StatusConflict, "limit", "at most 10 active tokens per bot")
		return
	}
	token, hash := randomToken(apiTokenPrefix, 32)
	id, err := s.store.CreateAPIToken(r.Context(), store.NewAPIToken{Hash: hash, Kind: "bot", UserID: bot,
		Scopes: scopes, Name: cleanText(req.Name, 100)})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.event(r, userIDFrom(r.Context()), "bot_token_created")
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "token": token, "scopes": scopes, "bot_user_id": bot})
}

func (s *Server) handleBotTokens(w http.ResponseWriter, r *http.Request) {
	c, ok := s.communityAdmin(w, r)
	if !ok {
		return
	}
	bot, err := s.store.BotOf(r.Context(), c.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"bot_user_id": nil, "items": []any{}})
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	tokens, err := s.store.BotTokens(r.Context(), bot)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bot_user_id": bot, "items": tokens})
}

func (s *Server) handleRevokeBotToken(w http.ResponseWriter, r *http.Request) {
	c, ok := s.communityAdmin(w, r)
	if !ok {
		return
	}
	tid, ok := pathID(w, r, "tid")
	if !ok {
		return
	}
	bot, err := s.store.BotOf(r.Context(), c.ID)
	if err == nil {
		err = s.store.RevokeBotToken(r.Context(), bot, tid)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "token not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- Callback API ----------

var callbackEvents = []string{"message_new", "wall_post_new", "wall_reply_new", "group_join", "group_leave"}

const (
	callbackMaxAttempts = 6
	callbackMaxBody     = 64 << 10
)

func (s *Server) validCallbackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 500 || u.User != nil || u.Host == "" || u.Fragment != "" {
		return false
	}
	if s.callbackInsecure {
		return u.Scheme == "https" || u.Scheme == "http"
	}
	return u.Scheme == "https"
}

func (s *Server) handleSetCallback(w http.ResponseWriter, r *http.Request) {
	c, ok := s.communityAdmin(w, r)
	if !ok {
		return
	}
	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
		Secret string   `json:"secret"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	if !s.validCallbackURL(req.URL) {
		errs["url"] = "https URL without credentials"
	}
	events := []string{}
	for _, e := range req.Events {
		if !slices.Contains(callbackEvents, e) {
			errs["events"] = "choose from: " + strings.Join(callbackEvents, ", ")
		} else if !slices.Contains(events, e) {
			events = append(events, e)
		}
	}
	if len(events) == 0 && errs["events"] == "" {
		errs["events"] = "at least one event"
	}
	generated := req.Secret == ""
	if generated {
		b := make([]byte, 24)
		_, _ = rand.Read(b)
		req.Secret = hex.EncodeToString(b)
	} else if len(req.Secret) < 16 || len(req.Secret) > 64 {
		errs["secret"] = "16-64 characters"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	enc, err := s.box.Seal([]byte(req.Secret))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	cb := make([]byte, 4)
	_, _ = rand.Read(cb)
	code := hex.EncodeToString(cb)
	if err := s.store.SetCallbackServer(r.Context(), store.CallbackServer{CommunityID: c.ID, URL: req.URL,
		Events: events, Status: "unconfirmed", SecretEnc: enc, ConfirmationCode: code}); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.event(r, userIDFrom(r.Context()), "callback_server_set")
	resp := map[string]any{"status": "unconfirmed", "confirmation_code": code,
		"next": "make your server answer the confirmation event with this code, then call POST .../callback/confirm"}
	if generated {
		resp["secret"] = req.Secret // показывается один раз
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleConfirmCallback шлёт событие confirmation и ждёт в ответ код —
// так доказывается, что сервер принадлежит админу (а не чужой адрес для DDoS).
func (s *Server) handleConfirmCallback(w http.ResponseWriter, r *http.Request) {
	c, ok := s.communityAdmin(w, r)
	if !ok {
		return
	}
	cs, err := s.store.CallbackServer(r.Context(), c.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "callback server is not set")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	body, err := s.sendCallback(r.Context(), cs.URL, cs.SecretEnc, "confirmation", newEventID(), c.ID, nil)
	if err == nil && strings.TrimSpace(body) != cs.ConfirmationCode {
		err = errors.New("server did not return the confirmation code")
	}
	if err != nil {
		_ = s.store.SetCallbackStatus(r.Context(), c.ID, "unconfirmed", err.Error())
		writeError(w, http.StatusUnprocessableEntity, "confirmation_failed", err.Error())
		return
	}
	if err := s.store.SetCallbackStatus(r.Context(), c.ID, "active", ""); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "active"})
}

func (s *Server) handleGetCallback(w http.ResponseWriter, r *http.Request) {
	c, ok := s.communityAdmin(w, r)
	if !ok {
		return
	}
	cs, err := s.store.CallbackServer(r.Context(), c.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "callback server is not set")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	recent, err := s.store.CallbackDeliveries(r.Context(), c.ID, 20)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"server": cs, "confirmation_code": cs.ConfirmationCode, "recent": recent})
}

func (s *Server) handleDeleteCallback(w http.ResponseWriter, r *http.Request) {
	c, ok := s.communityAdmin(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteCallbackServer(r.Context(), c.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func newEventID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CallbackSignature: HMAC-SHA256(secret, timestamp + "." + body) в hex.
// Метка времени в подписи не даёт переиграть старое событие.
func CallbackSignature(secret []byte, ts string, body []byte) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(ts))
	m.Write([]byte("."))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// sendCallback отправляет одно событие и возвращает тело ответа (до 1 КБ).
func (s *Server) sendCallback(ctx context.Context, target string, secretEnc []byte, typ, eventID string,
	community int64, object json.RawMessage) (string, error) {
	secret, err := s.box.Open(secretEnc)
	if err != nil {
		return "", err
	}
	if !s.validCallbackURL(target) {
		return "", errors.New("callback URL is not allowed")
	}
	payload := map[string]any{"type": typ, "event_id": eventID, "group_id": community, "v": "1"}
	if object != nil {
		payload["object"] = object
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if len(body) > callbackMaxBody {
		return "", errors.New("event too large")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Socialnet-Callback/1")
	req.Header.Set("X-Socialnet-Event", typ)
	req.Header.Set("X-Socialnet-Event-Id", eventID)
	req.Header.Set("X-Socialnet-Timestamp", ts)
	req.Header.Set("X-Socialnet-Signature", CallbackSignature(secret, ts, body))
	resp, err := s.callbackClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("callback server responded %d", resp.StatusCode)
	}
	return string(b), nil
}

// DispatchCallbacks разбирает очередь, пока в ней есть готовые события
// (но не больше max за вызов). Вызывается фоновым циклом API.
func (s *Server) DispatchCallbacks(ctx context.Context, max int) (int, error) {
	n := 0
	for ; n < max; n++ {
		d, err := s.store.ClaimCallback(ctx)
		if err != nil || d == nil {
			return n, err
		}
		_, err = s.sendCallback(ctx, d.URL, d.SecretEnc, d.Type, d.EventID, d.CommunityID, d.Payload)
		if err == nil {
			if err := s.store.FinishCallback(ctx, d.ID); err != nil {
				return n, err
			}
			_ = s.store.SetCallbackStatus(ctx, d.CommunityID, "active", "")
			s.m.business.Inc("callback_delivered")
			continue
		}
		s.m.business.Inc("callback_failed")
		backoff := 10 * time.Second << (d.Attempts - 1) // 10 с, 20 с, 40 с … ~5 мин
		final, ferr := s.store.RetryCallback(ctx, d.ID, err.Error(), backoff, callbackMaxAttempts)
		if ferr != nil {
			return n, ferr
		}
		if final {
			_ = s.store.SetCallbackStatus(ctx, d.CommunityID, "failing", err.Error())
		}
		s.log.Warn("callback delivery failed", "community", d.CommunityID, "type", d.Type, "attempt", d.Attempts, "err", err)
	}
	return n, nil
}

// RunCallbacks — фоновый цикл доставки в процессе API.
func (s *Server) RunCallbacks(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	stale := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if time.Since(stale) > time.Minute {
			_ = s.store.RequeueStaleCallbacks(ctx, 2*time.Minute)
			stale = time.Now()
		}
		if _, err := s.DispatchCallbacks(ctx, 100); err != nil && ctx.Err() == nil {
			s.log.Warn("callback dispatch", "err", err)
		}
	}
}

// emit ставит событие в очередь Callback API; ошибка не ломает основной запрос.
func (s *Server) emit(ctx context.Context, community int64, typ string, object any) {
	if err := s.store.EnqueueCallback(ctx, community, typ, object); err != nil {
		s.log.Warn("enqueue callback", "err", err, "type", typ)
	}
}
