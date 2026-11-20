package httpapi

import (
	"errors"
	"github.com/go-chi/chi/v5"
	"net/http"
	"socialnet/internal/store"
	"strings"
	"unicode/utf8"
)

func (s *Server) handleDescribePhoto(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		writeError(w, 404, "not_found", "photo not found")
		return
	}
	var req struct {
		Description string `json:"description"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if utf8.RuneCountInString(req.Description) > 2000 {
		writeError(w, 422, "validation_failed", "description too long")
		return
	}
	err := s.store.DescribePhoto(r.Context(), userIDFrom(r.Context()), id, req.Description)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "not_found", "photo not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) handlePopularCommunities(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.PopularCommunities(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) handleLibraryPhotos(w http.ResponseWriter, r *http.Request) {
	after := r.URL.Query().Get("after")
	if after != "" && !validUUID(after) {
		writeError(w, 400, "bad_request", "invalid cursor")
		return
	}
	items, err := s.store.LibraryPhotos(r.Context(), userIDFrom(r.Context()), after, 200)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	var next *string
	if len(items) == 200 {
		next = &items[len(items)-1].ID
	}
	writeJSON(w, 200, map[string]any{"items": items, "next_cursor": next})
}
func (s *Server) handlePhotoAlbums(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.PhotoAlbums(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) handleCreatePhotoAlbum(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if n := utf8.RuneCountInString(req.Title); n < 1 || n > 100 {
		writeError(w, 422, "validation_failed", "title must be 1-100 characters")
		return
	}
	id, err := s.store.CreatePhotoAlbum(r.Context(), userIDFrom(r.Context()), req.Title)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "title": req.Title})
}
func (s *Server) handleOrganizePhotos(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs      []string `json:"ids"`
		AlbumID  *int64   `json:"album_id"`
		Archived *bool    `json:"archived"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.IDs) < 1 || len(req.IDs) > 200 || (req.AlbumID == nil && req.Archived == nil) {
		writeError(w, 422, "validation_failed", "select 1-200 photos and an action")
		return
	}
	seen := map[string]bool{}
	for i, id := range req.IDs {
		id = strings.ToLower(id)
		if !validUUID(id) || seen[id] {
			writeError(w, 422, "validation_failed", "invalid or duplicate photo")
			return
		}
		seen[id] = true
		req.IDs[i] = id
	}
	err := s.store.OrganizePhotos(r.Context(), userIDFrom(r.Context()), req.IDs, req.AlbumID, req.Archived)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "not_found", "photo or album not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(204)
}
