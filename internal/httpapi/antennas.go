package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"socialnet/internal/store"
)

// «Антенны» — свои ленты по ключевым словам. Поиск полнотекстовый по-русски
// («Таганай» найдёт и «Таганае»), по всем записям, которые человеку видны.

var antennaWordRe = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} -]{0,48}[\p{L}\p{N}]$`)

func cleanWords(in []string, field string, min int, errs map[string]string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, w := range in {
		w = strings.Join(strings.Fields(strings.ToLower(strings.TrimPrefix(strings.TrimSpace(w), "#"))), " ")
		if w == "" || seen[w] {
			continue
		}
		if !antennaWordRe.MatchString(w) {
			errs[field] = "words: 2-50 letters, digits, spaces or hyphens"
			return nil
		}
		seen[w] = true
		out = append(out, w)
	}
	if len(out) < min || len(out) > 10 {
		errs[field] = fmt.Sprintf("%d-10 words", min)
	}
	return out
}

func (s *Server) handleAntennas(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Antennas(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) saveAntenna(w http.ResponseWriter, r *http.Request, id int64) {
	var req struct {
		Name     string   `json:"name"`
		Keywords []string `json:"keywords"`
		Excludes []string `json:"excludes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	name := cleanText(req.Name, 50)
	if name == "" {
		errs["name"] = "1-50 characters"
	}
	kw := cleanWords(req.Keywords, "keywords", 1, errs)
	ex := cleanWords(req.Excludes, "excludes", 0, errs)
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := userIDFrom(r.Context())
	max := s.plusLimit(r, me, store.MaxAntennas, store.MaxAntennasPlus)
	id, err := s.store.SaveAntenna(r.Context(), me, id, name, kw, ex, max)
	switch {
	case errors.Is(err, store.ErrTooMany):
		writeError(w, http.StatusUnprocessableEntity, "too_many_antennas", fmt.Sprintf("at most %d feeds", max))
		return
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "feed not found")
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	a, err := s.store.Antenna(r.Context(), me, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	code := http.StatusOK
	if r.Method == http.MethodPost {
		code = http.StatusCreated
	}
	writeJSON(w, code, a)
}

func (s *Server) handleCreateAntenna(w http.ResponseWriter, r *http.Request) { s.saveAntenna(w, r, 0) }

func (s *Server) handleUpdateAntenna(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		s.saveAntenna(w, r, id)
	}
}

func (s *Server) handleDeleteAntenna(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteAntenna(r.Context(), userIDFrom(r.Context()), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "feed not found")
		return
	} else if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAntennaPosts(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	a, err := s.store.Antenna(r.Context(), me, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "feed not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	p := pageParams(r)
	items, err := s.store.AntennaPosts(r.Context(), me, a, p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, func(x store.Post) int64 { return x.ID }))
}
