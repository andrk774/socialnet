package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"socialnet/internal/markup"
	"socialnet/internal/store"
)

// Статьи — длинные отчёты о походах. Текст хранится как есть и при выдаче
// разбирается в дерево блоков (internal/markup): HTML из текста не
// получается никогда. Фото в тексте и обложка — только свои, маршрут — свой.

type articleView struct {
	*store.Article
	Blocks  []markup.Block `json:"blocks,omitempty"`
	Excerpt string         `json:"excerpt"`
}

type articleRequest struct {
	Title   *string     `json:"title"`
	Body    *string     `json:"body"`
	CoverID opt[string] `json:"cover_media_id"`
	RouteID opt[string] `json:"route_id"`
	Status  *string     `json:"status"` // draft | published
}

func articleErr(err error) string {
	switch {
	case errors.Is(err, markup.ErrTooLong):
		return "max 100000 characters"
	case errors.Is(err, markup.ErrTooMany):
		return "too many paragraphs"
	case errors.Is(err, markup.ErrTooManyI):
		return "at most 50 photos"
	}
	return "invalid text"
}

// applyArticle проверяет поля и права на фото и маршрут.
func (s *Server) applyArticle(r *http.Request, req articleRequest, f *store.ArticleFields) (map[string]string, error) {
	me := userIDFrom(r.Context())
	errs := map[string]string{}
	if req.Title != nil {
		f.Title = cleanText(*req.Title, 150)
	}
	if f.Title == "" {
		errs["title"] = "1-150 characters"
	}
	if req.Body != nil {
		f.Body = strings.TrimSpace(*req.Body)
	}
	blocks, err := markup.Parse(f.Body)
	if err != nil {
		errs["body"] = articleErr(err)
	}
	if req.Status != nil {
		f.Status = *req.Status
	}
	if f.Status != "draft" && f.Status != "published" {
		errs["status"] = "draft or published"
	}
	if f.Status == "published" && len(blocks) == 0 && errs["body"] == "" {
		errs["body"] = "an empty article cannot be published"
	}
	if req.CoverID.Set {
		f.CoverID = nil
		if req.CoverID.V != nil {
			id := strings.ToLower(*req.CoverID.V)
			f.CoverID = &id
		}
	}
	if req.RouteID.Set {
		f.RouteID = nil
		if req.RouteID.V != nil {
			id := strings.ToLower(*req.RouteID.V)
			f.RouteID = &id
		}
	}
	ids := markup.MediaIDs(blocks)
	if f.CoverID != nil {
		if !validUUID(*f.CoverID) {
			errs["cover_media_id"] = "photo not found"
		} else {
			ids = append(ids, *f.CoverID)
		}
	}
	owned, err := s.store.OwnedMedia(r.Context(), me, ids)
	if err != nil {
		return nil, err
	}
	if f.CoverID != nil && validUUID(*f.CoverID) && !owned[*f.CoverID] {
		errs["cover_media_id"] = "photo not found"
	}
	for _, id := range markup.MediaIDs(blocks) {
		if !owned[id] {
			errs["body"] = "the text uses a photo that is not yours: " + id
			break
		}
	}
	if f.RouteID != nil {
		if !validUUID(*f.RouteID) {
			errs["route_id"] = "route not found"
		} else if err := s.checkRoute(r.Context(), me, *f.RouteID, errs); err != nil {
			return nil, err
		}
	}
	return errs, nil
}

func (s *Server) articleView(a *store.Article, full bool) articleView {
	v := articleView{Article: a}
	blocks, err := markup.Parse(a.Body)
	if err == nil {
		v.Excerpt = markup.PlainText(blocks, 240)
		if full {
			v.Blocks = blocks
		}
	}
	if !full {
		a.Body = ""
	}
	return v
}

func (s *Server) handleCreateArticle(w http.ResponseWriter, r *http.Request) {
	var req articleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	f := store.ArticleFields{Status: "draft"}
	errs, err := s.applyArticle(r, req, &f)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := userIDFrom(r.Context())
	id, err := s.store.CreateArticle(r.Context(), me, f)
	if errors.Is(err, store.ErrRateLimited) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many articles today")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	a, err := s.store.Article(r.Context(), me, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.articleView(a, true))
}

func (s *Server) articleParam(w http.ResponseWriter, r *http.Request) (*store.Article, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	a, err := s.store.Article(r.Context(), userIDFrom(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "article not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	return a, true
}

func (s *Server) handleGetArticle(w http.ResponseWriter, r *http.Request) {
	if a, ok := s.articleParam(w, r); ok {
		writeJSON(w, http.StatusOK, s.articleView(a, true))
	}
}

func (s *Server) handleUpdateArticle(w http.ResponseWriter, r *http.Request) {
	a, ok := s.articleParam(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	if a.Author.ID != me {
		writeError(w, http.StatusForbidden, "forbidden", "only the author can edit")
		return
	}
	var req articleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	f := store.ArticleFields{Title: a.Title, Body: a.Body, CoverID: a.CoverID, RouteID: a.RouteID, Status: a.Status}
	keepRoute := req.RouteID.Set && req.RouteID.V != nil && a.RouteID != nil && strings.EqualFold(*req.RouteID.V, *a.RouteID)
	if keepRoute {
		req.RouteID = opt[string]{} // прежний маршрут перепроверять не нужно
	}
	errs, err := s.applyArticle(r, req, &f)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if err := s.store.UpdateArticle(r.Context(), a.ID, me, f); err != nil {
		s.internalError(w, r, err)
		return
	}
	a, err = s.store.Article(r.Context(), me, a.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.articleView(a, true))
}

func (s *Server) handleDeleteArticle(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteArticle(r.Context(), id, userIDFrom(r.Context())); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "article not found")
		return
	} else if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleArticles(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r)
	items, err := s.store.Articles(r.Context(), userIDFrom(r.Context()), p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	views := make([]articleView, len(items))
	for i := range items {
		views[i] = s.articleView(&items[i], false)
	}
	writeJSON(w, http.StatusOK, list(views, p.Limit, func(v articleView) int64 { return v.ID }))
}

func (s *Server) handleMyArticles(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.MyArticles(r.Context(), userIDFrom(r.Context()), 100)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	views := make([]articleView, len(items))
	for i := range items {
		views[i] = s.articleView(&items[i], false)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": views})
}

// handlePreviewArticle: POST /articles/preview {"body"} — как будет выглядеть.
func (s *Server) handlePreviewArticle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Body string `json:"body"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	blocks, err := markup.Parse(req.Body)
	if err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"body": articleErr(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"blocks": blocks})
}
