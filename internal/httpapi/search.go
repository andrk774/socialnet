package httpapi

import (
	"net/http"

	"socialnet/internal/store"
)

// searchRoute — маршрут каталога в результатах поиска.
type searchRoute struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	DistanceM int            `json:"distance_m"`
	GainM     int            `json:"gain_m"`
	Author    store.UserCard `json:"author"`
	URL       string         `json:"url"`
}

// searchHiking: type=hikes|articles|routes — один список; type=all — по
// несколько из каждого раздела, включая людей и сообщества.
func (s *Server) searchHiking(w http.ResponseWriter, r *http.Request, typ, q string, limit int) {
	ctx, me := r.Context(), userIDFrom(r.Context())
	if typ == "all" {
		limit = 5
	}
	out := map[string]any{}
	var err error
	if typ == "hikes" || typ == "all" {
		var v []store.Event
		v, err = s.store.SearchHikes(ctx, me, q, limit)
		out["hikes"] = nonNil(v)
	}
	if err == nil && (typ == "articles" || typ == "all") {
		var v []store.Article
		v, err = s.store.SearchArticles(ctx, me, q, limit)
		out["articles"] = nonNil(v)
	}
	if err == nil && (typ == "routes" || typ == "all") {
		var v []store.PublicRoute
		v, err = s.store.SearchPublicRoutes(ctx, q, limit)
		list := make([]searchRoute, 0, len(v))
		for _, pr := range v {
			list = append(list, searchRoute{ID: pr.ID, Title: pr.Title, DistanceM: pr.DistanceM, GainM: pr.GainM, Author: pr.Author, URL: "/trails/" + pr.ID})
		}
		out["routes"] = list
	}
	if err == nil && typ == "all" {
		var users []store.UserCard
		if users, err = s.store.SearchUsers(ctx, me, q, limit); err == nil {
			out["users"] = nonNil(users)
			var cs []store.Community
			cs, err = s.store.SearchCommunities(ctx, me, q, limit)
			out["communities"] = nonNil(cs)
		}
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if typ != "all" {
		writeJSON(w, http.StatusOK, map[string]any{"items": out[typ]})
		return
	}
	writeJSON(w, http.StatusOK, out)
}
