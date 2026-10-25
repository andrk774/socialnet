package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"crypto/ecdh"

	"socialnet/internal/store"
	"socialnet/internal/webpush"
)

// ---------- онлайн-статус ----------

const onlineWindow = 3 * time.Minute

type presenceView struct {
	UserID   int64      `json:"user_id"`
	Online   *bool      `json:"online,omitempty"`
	LastSeen *time.Time `json:"last_seen,omitempty"`
	// Если человек скрыл время захода, показываем примерно, как во ВКонтакте:
	// recently | within_week | within_month | long_ago
	LastSeenApprox string `json:"last_seen_approx,omitempty"`
}

func (s *Server) presence(id int64, lastSeen *time.Time, visibility string, self bool, rel store.Relationship) presenceView {
	v := presenceView{UserID: id}
	if !self && rel.Blocked() {
		return v // заблокированным — ничего
	}
	online := s.hub.Online(id) || (lastSeen != nil && time.Since(*lastSeen) < onlineWindow)
	if canSee(visibility, self, rel) {
		v.Online, v.LastSeen = &online, lastSeen
		return v
	}
	switch {
	case online || (lastSeen != nil && time.Since(*lastSeen) < 24*time.Hour):
		v.LastSeenApprox = "recently"
	case lastSeen != nil && time.Since(*lastSeen) < 7*24*time.Hour:
		v.LastSeenApprox = "within_week"
	case lastSeen != nil && time.Since(*lastSeen) < 30*24*time.Hour:
		v.LastSeenApprox = "within_month"
	default:
		v.LastSeenApprox = "long_ago"
	}
	return v
}

// handlePresence: POST /presence {"ids": [1,2,3]} — статусы для списка друзей.
func (s *Server) handlePresence(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.IDs) == 0 || len(req.IDs) > 100 {
		writeError(w, http.StatusBadRequest, "bad_request", "1-100 ids")
		return
	}
	me := userIDFrom(r.Context())
	rows, err := s.store.PresenceRows(r.Context(), me, req.IDs)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	out := make([]presenceView, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.presence(row.UserID, row.LastSeen, row.Visibility, row.UserID == me, row.Rel))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// ---------- настройки уведомлений ----------

var notificationTypes = []string{
	"friend_request", "friend_accepted", "like", "comment", "mention", "message",
	"gift", "transfer", "donation", "video_ready",
}

func (s *Server) handleGetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.NotificationSettings(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": n, "available_types": notificationTypes})
}

func (s *Server) handlePutNotificationSettings(w http.ResponseWriter, r *http.Request) {
	var n store.NotificationSettings
	if !decodeJSON(w, r, &n) {
		return
	}
	errs := map[string]string{}
	if n.MutedTypes == nil {
		n.MutedTypes = []string{}
	}
	for _, t := range n.MutedTypes {
		if !slices.Contains(notificationTypes, t) {
			errs["muted_types"] = "unknown type: " + t
		}
	}
	n.MutedTypes = slices.Compact(slices.Sorted(slices.Values(n.MutedTypes)))
	if (n.QuietStart == nil) != (n.QuietEnd == nil) {
		errs["quiet_start"] = "set both quiet_start and quiet_end, or neither"
	}
	for name, v := range map[string]*int{"quiet_start": n.QuietStart, "quiet_end": n.QuietEnd} {
		if v != nil && (*v < 0 || *v > 1439) {
			errs[name] = "minutes from midnight, 0-1439"
		}
	}
	if n.Timezone == "" {
		n.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(n.Timezone); err != nil || len(n.Timezone) > 64 {
		errs["timezone"] = "unknown timezone, e.g. Asia/Yekaterinburg"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if err := s.store.SaveNotificationSettings(r.Context(), userIDFrom(r.Context()), n); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": n})
}

// inQuietHours: тихий режим, в том числе «через полночь» (23:00–08:00).
func inQuietHours(n store.NotificationSettings, now time.Time) bool {
	if n.QuietStart == nil || n.QuietEnd == nil {
		return false
	}
	loc, err := time.LoadLocation(n.Timezone)
	if err != nil {
		loc = time.UTC
	}
	t := now.In(loc)
	m := t.Hour()*60 + t.Minute()
	start, end := *n.QuietStart, *n.QuietEnd
	if start <= end {
		return m >= start && m < end
	}
	return m >= start || m < end
}

// ---------- Web Push ----------

func (s *Server) handleVAPIDKey(w http.ResponseWriter, r *http.Request) {
	if s.vapid == nil {
		writeError(w, http.StatusServiceUnavailable, "push_disabled", "push notifications are not configured on this server")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"public_key": s.vapid.Public})
}

// Формат — как отдаёт браузерный PushSubscription.toJSON().
type pushSubRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	if s.vapid == nil {
		writeError(w, http.StatusServiceUnavailable, "push_disabled", "push notifications are not configured on this server")
		return
	}
	var req pushSubRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	if len(req.Endpoint) > 2048 || webpush.ValidateEndpoint(req.Endpoint, s.pushHosts) != nil {
		errs["endpoint"] = "must be an https endpoint of a known browser push service"
	}
	p256dh, err := base64.RawURLEncoding.DecodeString(req.Keys.P256dh)
	if err == nil {
		_, err = ecdh.P256().NewPublicKey(p256dh) // настоящая точка на кривой P-256
	}
	if err != nil {
		errs["keys.p256dh"] = "invalid P-256 public key"
	}
	auth, err := base64.RawURLEncoding.DecodeString(req.Keys.Auth)
	if err != nil || len(auth) != 16 {
		errs["keys.auth"] = "must be 16 bytes base64url"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	err = s.store.SavePushSub(r.Context(), userIDFrom(r.Context()), req.Endpoint, p256dh, auth)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "conflict", "subscription belongs to another account")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.store.DeletePushSub(r.Context(), userIDFrom(r.Context()), req.Endpoint); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type pushPayload struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

// push отправляет уведомление на все устройства пользователя в фоне,
// с учётом настроек и тихого режима. Протухшие подписки удаляются.
func (s *Server) push(userID int64, p pushPayload) {
	if s.vapid == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		settings, err := s.store.NotificationSettings(ctx, userID)
		if err != nil || !settings.PushEnabled || slices.Contains(settings.MutedTypes, p.Type) ||
			inQuietHours(settings, time.Now()) {
			return
		}
		subs, err := s.store.PushSubs(ctx, userID)
		if err != nil {
			s.log.Error("push subs", "err", err)
			return
		}
		body, _ := json.Marshal(p)
		for _, sub := range subs {
			err := s.vapid.Send(ctx, s.pushClient, webpush.Subscription{
				Endpoint: sub.Endpoint, P256dh: sub.P256dh, Auth: sub.Auth}, body, 24*time.Hour)
			switch {
			case errors.Is(err, webpush.ErrGone):
				_ = s.store.DeletePushSubByID(ctx, sub.ID)
			case err != nil:
				s.log.Warn("push send", "err", err)
			}
		}
	}()
}

func actorName(a *store.UserCard) string {
	if a == nil {
		return "Кто-то"
	}
	if a.FirstName != "" {
		return a.FirstName
	}
	return a.Username
}

// pushText — текст push по типу уведомления.
func pushText(n *store.Notification) pushPayload {
	name := actorName(n.Actor)
	p := pushPayload{Type: n.Type, Title: "Socialnet", URL: "/notifications"}
	switch n.Type {
	case "friend_request":
		p.Body, p.URL = name+" хочет добавить вас в друзья", "/friends/requests"
	case "friend_accepted":
		p.Body = name + " принял(а) вашу заявку в друзья"
	case "like":
		p.Body = name + " оценил(а) вашу запись"
	case "comment":
		p.Body = name + " прокомментировал(а) вашу запись"
	case "mention":
		p.Body = name + " упомянул(а) вас"
	case "gift":
		p.Body = name + " отправил(а) вам подарок"
	case "transfer":
		p.Body = name + " перевёл(а) вам монеты"
	case "donation":
		p.Body = name + " поддержал(а) ваше сообщество"
	case "video_ready":
		p.Body = "Ваше видео обработано и опубликовано"
	default:
		p.Body = "У вас новое уведомление"
	}
	if n.PostID != nil {
		p.URL = "/posts/" + strconv.FormatInt(*n.PostID, 10)
	}
	return p
}

// ---------- письма о непрочитанном ----------

// RunDigest отправляет письма тем, кто давно не заходил и пропустил уведомления.
// Вызывается по таймеру из main.
func (s *Server) RunDigest(ctx context.Context) (int, error) {
	list, err := s.store.DigestCandidates(ctx, 500)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, c := range list {
		claimed, err := s.store.ClaimDigest(ctx, c.UserID)
		if err != nil {
			return sent, err
		}
		if !claimed {
			continue
		}
		s.sendMail(c.Email, fmt.Sprintf("У вас %d непрочитанных уведомлений", c.Unread),
			"Здравствуйте, "+c.FirstName+"!\n\n"+
				fmt.Sprintf("Пока вас не было, накопилось %d уведомлений.\n", c.Unread)+
				"Посмотреть: "+s.cfg.AppURL+"/notifications\n\n"+
				"Отключить такие письма: "+s.cfg.AppURL+"/settings/notifications\n")
		sent++
	}
	return sent, nil
}
