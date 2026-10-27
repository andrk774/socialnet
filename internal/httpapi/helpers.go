package httpapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/store"
)

// pathID разбирает числовой {name} из пути. При ошибке сам отвечает 404.
func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return 0, false
	}
	return id, true
}

// page: ?limit=20&before=<id> (новые сверху) или ?after=<id> (по возрастанию).
type page struct {
	Limit  int
	Before int64
	After  int64
}

func pageParams(r *http.Request) page {
	q := r.URL.Query()
	p := page{Limit: 20, Before: math.MaxInt64}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		p.Limit = min(n, 50)
	}
	if n, err := strconv.ParseInt(q.Get("before"), 10, 64); err == nil && n > 0 {
		p.Before = n
	}
	if n, err := strconv.ParseInt(q.Get("after"), 10, 64); err == nil && n > 0 {
		p.After = n
	}
	return p
}

type listResponse[T any] struct {
	Items      []T    `json:"items"`
	NextCursor *int64 `json:"next_cursor"` // передайте в before/after для следующей страницы
}

func list[T any](items []T, limit int, cursor func(T) int64) listResponse[T] {
	if items == nil {
		items = []T{}
	}
	resp := listResponse[T]{Items: items}
	if len(items) == limit {
		c := cursor(items[len(items)-1])
		resp.NextCursor = &c
	}
	return resp
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validUUID(s string) bool { return uuidRe.MatchString(s) }

// ---------- представления пользователя ----------

// selfView — свой профиль, со всеми настройками.
type selfView struct {
	ID               int64       `json:"id"`
	ConsentRequired  bool        `json:"consent_required"` // принять новую редакцию документов
	Plus             bool        `json:"plus"`
	Email            string      `json:"email"`
	EmailVerified    bool        `json:"email_verified"`
	Username         string      `json:"username"`
	FirstName        string      `json:"first_name"`
	LastName         string      `json:"last_name"`
	StatusText       string      `json:"status_text"`
	City             string      `json:"city"`
	About            string      `json:"about"`
	AvatarURL        *string     `json:"avatar_url"`
	Role             string      `json:"role"`
	TwoFactorEnabled bool        `json:"two_factor_enabled"`
	Privacy          privacyView `json:"privacy"`
	CreatedAt        time.Time   `json:"created_at"`
}

type privacyView struct {
	OnlineVisibility  string `json:"online_visibility"`
	ProfileVisibility string `json:"profile_visibility"`
	FriendsVisibility string `json:"friends_visibility"`
	MessagePrivacy    string `json:"message_privacy"`
	Searchable        bool   `json:"searchable"`
}

func toSelf(u *store.User) selfView {
	return selfView{
		ID: u.ID, Email: u.Email, EmailVerified: u.EmailVerifiedAt != nil,
		Username: u.Username, FirstName: u.FirstName, LastName: u.LastName,
		StatusText: u.StatusText, City: u.City, About: u.About,
		AvatarURL: store.MediaURL(u.AvatarID, "thumb"), Role: u.Role,
		TwoFactorEnabled: u.TOTPEnabledAt != nil,
		Privacy: privacyView{
			OnlineVisibility:  u.OnlineVisibility,
			ProfileVisibility: u.ProfileVisibility, FriendsVisibility: u.FriendsVisibility,
			MessagePrivacy: u.MessagePrivacy, Searchable: u.Searchable,
		},
		CreatedAt: u.CreatedAt,
	}
}

// profileView — чужой профиль. Закрытый профиль показывает только имя и аватар,
// как во ВКонтакте.
type profileView struct {
	ID           int64        `json:"id"`
	Username     string       `json:"username"`
	FirstName    string       `json:"first_name"`
	LastName     string       `json:"last_name"`
	AvatarURL    *string      `json:"avatar_url"`
	Closed       bool         `json:"closed"`
	StatusText   string       `json:"status_text,omitempty"`
	City         string       `json:"city,omitempty"`
	About        string       `json:"about,omitempty"`
	CreatedAt    *time.Time   `json:"created_at,omitempty"`
	Plus         bool         `json:"plus"` // значок подписки «Плюс»
	Relationship relView      `json:"relationship"`
	Presence     presenceView `json:"presence"`
}

type relView struct {
	Status   string `json:"status"` // self | friends | request_sent | request_received | none
	IBlocked bool   `json:"i_blocked"`
	CanWrite bool   `json:"can_write"`
}

func relStatus(self bool, r store.Relationship) string {
	switch {
	case self:
		return "self"
	case r.Friends():
		return "friends"
	case r.IFollow:
		return "request_sent"
	case r.FollowsMe:
		return "request_received"
	}
	return "none"
}

// canSee применяет настройку видимости (everyone | friends | only_me).
func canSee(setting string, self bool, r store.Relationship) bool {
	if self {
		return true
	}
	if r.Blocked() {
		return false
	}
	switch setting {
	case "everyone":
		return true
	case "friends":
		return r.Friends()
	}
	return false
}

func canMessage(setting string, r store.Relationship) bool {
	if r.Blocked() {
		return false
	}
	switch setting {
	case "everyone":
		return true
	case "friends":
		return r.Friends()
	}
	return false
}

func toProfile(u *store.User, self bool, r store.Relationship) profileView {
	v := profileView{
		ID: u.ID, Username: u.Username, FirstName: u.FirstName, LastName: u.LastName,
		AvatarURL: store.MediaURL(u.AvatarID, "thumb"),
		Relationship: relView{
			Status:   relStatus(self, r),
			IBlocked: r.IBlocked,
			CanWrite: !self && canMessage(u.MessagePrivacy, r),
		},
	}
	if canSee(u.ProfileVisibility, self, r) {
		v.StatusText, v.City, v.About = u.StatusText, u.City, u.About
		v.CreatedAt = &u.CreatedAt
	} else {
		v.Closed = true
	}
	return v
}

// notify сохраняет уведомление и сразу отправляет его по WebSocket.
// Ошибки только логируются: из-за уведомления действие не должно падать.
func (s *Server) notify(ctx context.Context, n store.NewNotification) {
	created, err := s.store.CreateNotification(ctx, n)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.log.Error("notification", "err", err)
		}
		return
	}
	// Если пользователь не в приложении — push на телефон или компьютер.
	if !s.hub.Publish(n.UserID, "notification", created) {
		s.push(n.UserID, pushText(created))
	}
}
