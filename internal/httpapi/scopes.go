package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/auth"
	"socialnet/internal/store"
)

// Токены сторонних приложений и ботов начинаются с префикса — так их
// отличает middleware, а сканеры секретов (GitHub и т. п.) находят утёкшие.
const apiTokenPrefix = "snt_"

// Права (scopes), которые приложение может запросить у пользователя.
var scopeInfo = map[string]string{
	"profile":       "Просмотр профиля, друзей и поиск",
	"friends":       "Заявки в друзья",
	"wall":          "Публикации, лента, комментарии и реакции",
	"photos":        "Загрузка фото",
	"messages":      "Личные сообщения и беседы",
	"notifications": "Уведомления",
	"music":         "Музыка и плейлисты",
	"video":         "Видео",
	"communities":   "Сообщества: просмотр, вступление",
	"stories":       "Истории",
	"events":        "Мероприятия",
	"docs":          "Документы: загрузка и скачивание",
	"bookmarks":     "Закладки",
	"routes":        "Маршруты: загрузка GPX и скачивание",
	"offline":       "Доступ в любое время (токен без срока действия)",
}

// Боту сообщества доступны только эти права.
var botScopes = []string{"messages", "wall", "photos", "docs"}

// scopeRoutes: какой scope открывает какой маршрут. Всё, чего здесь нет —
// безопасность аккаунта, сессии, пароль, 2FA, кошелёк, ключи E2E, настройки,
// модерация, админка, управление приложениями — токенам API недоступно вообще.
var scopeRoutes = map[string][]string{
	"profile": {
		"GET /api/v1/users/me", "GET /api/v1/users/{id}", "GET /api/v1/users/by-username/{username}",
		"GET /api/v1/users/{id}/friends", "GET /api/v1/users/{id}/gifts", "GET /api/v1/search",
	},
	"friends": {
		"POST /api/v1/users/{id}/friend", "DELETE /api/v1/users/{id}/friend",
		"POST /api/v1/users/{id}/friend/reject", "GET /api/v1/friends/requests",
	},
	"wall": {
		"GET /api/v1/antennas", "POST /api/v1/antennas", "PUT /api/v1/antennas/{id}", "DELETE /api/v1/antennas/{id}",
		"GET /api/v1/antennas/{id}/posts",
		"POST /api/v1/articles", "GET /api/v1/articles", "GET /api/v1/articles/mine", "POST /api/v1/articles/preview",
		"GET /api/v1/articles/{id}", "PATCH /api/v1/articles/{id}", "DELETE /api/v1/articles/{id}",
		"POST /api/v1/posts", "GET /api/v1/posts/{id}", "PATCH /api/v1/posts/{id}", "DELETE /api/v1/posts/{id}",
		"POST /api/v1/posts/{id}/like", "DELETE /api/v1/posts/{id}/like",
		"POST /api/v1/posts/{id}/reactions", "DELETE /api/v1/posts/{id}/reactions",
		"GET /api/v1/posts/{id}/comments", "POST /api/v1/posts/{id}/comments",
		"DELETE /api/v1/comments/{id}", "GET /api/v1/comments/{id}/replies",
		"GET /api/v1/users/{id}/posts", "GET /api/v1/users/{id}/reviews", "GET /api/v1/feed", "GET /api/v1/hashtags/{tag}/posts",
		"GET /api/v1/communities/{id}/posts",
		"GET /api/v1/posts/{id}/audience", "POST /api/v1/posts/{id}/poll/votes", "DELETE /api/v1/posts/{id}/poll/votes",
		"GET /api/v1/posts/{id}/poll/options/{oid}/voters",
		"GET /api/v1/scheduled-posts", "PATCH /api/v1/scheduled-posts/{sid}", "DELETE /api/v1/scheduled-posts/{sid}",
	},
	"docs": {
		"POST /api/v1/documents", "GET /api/v1/documents", "PATCH /api/v1/documents/{did}",
		"DELETE /api/v1/documents/{did}", "GET /api/v1/documents/{did}/download",
	},
	"routes": {
		"POST /api/v1/routes", "GET /api/v1/routes", "GET /api/v1/routes/{rid}", "PATCH /api/v1/routes/{rid}",
		"DELETE /api/v1/routes/{rid}", "GET /api/v1/routes/{rid}/gpx",
		"POST /api/v1/routes/{rid}/marks", "POST /api/v1/routes/{rid}/marks/{pid}/check", "DELETE /api/v1/routes/{rid}/marks/{pid}",
		"GET /api/v1/routes/{rid}/reviews", "POST /api/v1/routes/{rid}/reviews", "DELETE /api/v1/routes/{rid}/reviews",
	},
	"bookmarks": {
		"GET /api/v1/bookmarks", "PUT /api/v1/bookmarks/{kind}/{target}", "DELETE /api/v1/bookmarks/{kind}/{target}",
	},
	"photos": {"POST /api/v1/media", "DELETE /api/v1/media/{id}"},
	"messages": {
		"POST /api/v1/users/{id}/messages", "GET /api/v1/conversations", "POST /api/v1/companions/{id}/reply",
		"GET /api/v1/conversations/{id}/messages", "POST /api/v1/conversations/{id}/messages",
		"POST /api/v1/conversations/{id}/read", "POST /api/v1/conversations/{id}/typing",
		"POST /api/v1/chats", "PATCH /api/v1/conversations/{id}",
		"GET /api/v1/conversations/{id}/members", "POST /api/v1/conversations/{id}/members",
		"DELETE /api/v1/conversations/{id}/members/{uid}", "PUT /api/v1/conversations/{id}/members/{uid}/role",
		"GET /api/v1/voice/{vid}", "POST /api/v1/voice", "POST /api/v1/ws/ticket", "GET /api/v1/users/{id}/keys",
	},
	"notifications": {"GET /api/v1/notifications", "POST /api/v1/notifications/read"},
	"music": {
		"POST /api/v1/tracks", "GET /api/v1/tracks/{tid}", "DELETE /api/v1/tracks/{tid}",
		"POST /api/v1/tracks/{tid}/my-music", "DELETE /api/v1/tracks/{tid}/my-music",
		"GET /api/v1/users/{id}/music", "GET /api/v1/users/{id}/playlists",
		"POST /api/v1/playlists", "GET /api/v1/playlists/{id}", "PATCH /api/v1/playlists/{id}",
		"DELETE /api/v1/playlists/{id}", "POST /api/v1/playlists/{id}/tracks",
		"DELETE /api/v1/playlists/{id}/tracks/{tid}",
	},
	"video": {
		"POST /api/v1/videos", "GET /api/v1/videos/{vid}", "PATCH /api/v1/videos/{vid}",
		"DELETE /api/v1/videos/{vid}", "GET /api/v1/users/{id}/videos",
	},
	"communities": {
		"GET /api/v1/communities", "GET /api/v1/communities/by-slug/{slug}", "GET /api/v1/communities/{id}",
		"POST /api/v1/communities/{id}/join", "POST /api/v1/communities/{id}/leave",
		"GET /api/v1/communities/{id}/members", "GET /api/v1/communities/{id}/events",
		"GET /api/v1/communities/{id}/fundraisers", "GET /api/v1/fundraisers", "GET /api/v1/fundraisers/{id}",
		"GET /api/v1/communities/{id}/emoji", "POST /api/v1/communities/{id}/emoji", "DELETE /api/v1/communities/{id}/emoji/{eid}",
	},
	"stories": {
		"POST /api/v1/stories", "GET /api/v1/stories/feed", "GET /api/v1/users/{id}/stories",
		"POST /api/v1/stories/{id}/view", "GET /api/v1/stories/{id}/viewers", "DELETE /api/v1/stories/{id}",
	},
	"events": {
		"POST /api/v1/events", "GET /api/v1/events", "GET /api/v1/events/{id}", "PATCH /api/v1/events/{id}",
		"DELETE /api/v1/events/{id}", "POST /api/v1/events/{id}/rsvp", "DELETE /api/v1/events/{id}/rsvp",
		"GET /api/v1/events/{id}/attendees", "DELETE /api/v1/events/{id}/attendees/{uid}",
		"GET /api/v1/events/{id}/applications", "POST /api/v1/events/{id}/applications/{uid}", "GET /api/v1/hikes",
		"GET /api/v1/events/{id}/contacts", "POST /api/v1/events/{id}/contacts", "DELETE /api/v1/events/{id}/contacts/{cid}",
		"POST /api/v1/events/{id}/returned", "GET /api/v1/weather", "GET /api/v1/events/{id}/participants.csv",
		"GET /api/v1/events/{id}/calendar.ics", "POST /api/v1/events/{id}/review", "DELETE /api/v1/events/{id}/review",
		"POST /api/v1/rides", "GET /api/v1/rides", "GET /api/v1/rides/{id}", "DELETE /api/v1/rides/{id}",
		"POST /api/v1/rides/{id}/request", "DELETE /api/v1/rides/{id}/request", "GET /api/v1/rides/{id}/requests",
		"POST /api/v1/rides/{id}/requests/{uid}",
		"POST /api/v1/companions", "GET /api/v1/companions", "GET /api/v1/companions/mine", "GET /api/v1/companions/{id}",
		"PATCH /api/v1/companions/{id}", "DELETE /api/v1/companions/{id}",
	},
}

// Даже с нужным scope бот не создаёт беседы и не добавляет в них людей — это спам.
var botDenied = map[string]bool{
	"POST /api/v1/chats":                                true,
	"POST /api/v1/conversations/{id}/members":           true,
	"PUT /api/v1/conversations/{id}/members/{uid}/role": true,
	"GET /api/v1/feed":                                  true,
	"POST /api/v1/posts/{id}/poll/votes":                true,
}

var routeScope = func() map[string]string {
	m := map[string]string{}
	for scope, routes := range scopeRoutes {
		for _, rt := range routes {
			if _, dup := m[rt]; dup {
				panic("route in two scopes: " + rt)
			}
			m[rt] = scope
		}
	}
	return m
}()

// parseScopes: "wall messages" или "wall,messages" → отсортированный список без повторов.
func parseScopes(s string, allowed []string) ([]string, string) {
	var out []string
	for _, sc := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' }) {
		if _, ok := scopeInfo[sc]; !ok || (allowed != nil && !slices.Contains(allowed, sc)) {
			return nil, sc
		}
		if !slices.Contains(out, sc) {
			out = append(out, sc)
		}
	}
	slices.Sort(out)
	return out, ""
}

// routePattern находит шаблон маршрута, в который попадёт запрос.
// middleware срабатывает до маршрутизации вложенных групп, поэтому ищем сами.
func (s *Server) routePattern(r *http.Request) string {
	if s.root == nil {
		return ""
	}
	return s.root.Find(chi.NewRouteContext(), r.Method, r.URL.Path)
}

// apiTokenAuth — ветка requireAuth для токенов приложений и ботов.
func (s *Server) apiTokenAuth(w http.ResponseWriter, r *http.Request, next http.Handler, token string) {
	info, err := s.store.APITokenAuth(r.Context(), auth.HashRefreshToken(token))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			writeError(w, http.StatusUnauthorized, "invalid_token", "token is invalid, expired or revoked")
			return
		}
		s.internalError(w, r, err)
		return
	}
	if info.Banned {
		writeError(w, http.StatusForbidden, "banned", "account is banned")
		return
	}
	route := r.Method + " " + s.routePattern(r)
	need, ok := routeScope[route]
	if !ok || (info.Kind == "bot" && botDenied[route]) {
		writeError(w, http.StatusForbidden, "not_available", "this method is not available with an API token")
		return
	}
	if !slices.Contains(info.Scopes, need) {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="insufficient_scope", scope=%q`, need))
		writeError(w, http.StatusForbidden, "insufficient_scope", "token lacks scope "+need)
		return
	}
	lim := s.appLimiter
	if info.Kind == "bot" {
		lim = s.botLimiter
	}
	if !lim.allow(fmt.Sprint("t", info.ID)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests for this token")
		return
	}
	// Роль всегда user: токен приложения не даёт прав модератора или админа,
	// даже если их выдал админ. Онлайн-статус приложения не трогают.
	v := viewer{ID: info.UserID, Role: "user", API: true, Scopes: info.Scopes}
	if info.BotCommunity != nil {
		v.Bot = *info.BotCommunity
	}
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), viewerKey, v)))
}
