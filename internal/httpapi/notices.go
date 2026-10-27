package httpapi

import (
	"errors"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"socialnet/internal/store"
)

// Предупреждения на территории ведут модераторы: противопожарный режим,
// закрытые тропы нацпарков, паводок. Показываются на маршрутах и походах,
// которые пересекают их прямоугольник в дни действия.

type noticeRequest struct {
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	SourceURL string    `json:"source_url"`
	BBox      []float64 `json:"bbox"` // min_lat, min_lon, max_lat, max_lon
	StartsOn  string    `json:"starts_on"`
	EndsOn    *string   `json:"ends_on"`
}

func (req *noticeRequest) fields() (store.AreaNoticeFields, map[string]string) {
	errs := map[string]string{}
	f := store.AreaNoticeFields{Kind: req.Kind, Title: cleanText(req.Title, 200), Body: strings.TrimSpace(req.Body), SourceURL: strings.TrimSpace(req.SourceURL)}
	oneOf(errs, "kind", &f.Kind, "fire", "closure", "flood", "other")
	if f.Title == "" {
		errs["title"] = "1-200 characters"
	}
	if utf8.RuneCountInString(f.Body) > 2000 {
		errs["body"] = "up to 2000 characters"
	}
	if f.SourceURL != "" {
		if u, err := url.Parse(f.SourceURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(f.SourceURL) > 500 {
			errs["source_url"] = "http(s) link up to 500 characters"
		}
	}
	if len(req.BBox) != 4 {
		errs["bbox"] = "[min_lat, min_lon, max_lat, max_lon]"
	} else {
		copy(f.BBox[:], req.BBox)
		b := f.BBox
		for _, v := range b {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				errs["bbox"] = "numbers only"
			}
		}
		if errs["bbox"] == "" && (b[0] < -90 || b[2] > 90 || b[1] < -180 || b[3] > 180 || b[0] > b[2] || b[1] > b[3]) {
			errs["bbox"] = "min must not exceed max; latitude ±90, longitude ±180"
		}
		// не больше области: предупреждение на всю страну — уже не о территории
		if errs["bbox"] == "" && (b[2]-b[0] > 5 || b[3]-b[1] > 8) {
			errs["bbox"] = "too large: at most 5° of latitude and 8° of longitude"
		}
	}
	var err error
	if f.StartsOn, err = time.Parse(time.DateOnly, req.StartsOn); err != nil {
		errs["starts_on"] = "YYYY-MM-DD"
	}
	if req.EndsOn != nil && *req.EndsOn != "" {
		end, err := time.Parse(time.DateOnly, *req.EndsOn)
		if err != nil {
			errs["ends_on"] = "YYYY-MM-DD"
		} else if errs["starts_on"] == "" && end.Before(f.StartsOn) {
			errs["ends_on"] = "must not be before starts_on"
		} else {
			f.EndsOn = &end
		}
	}
	return f, errs
}

func (s *Server) handleCreateNotice(w http.ResponseWriter, r *http.Request) {
	var req noticeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	f, errs := req.fields()
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	id, err := s.store.CreateAreaNotice(r.Context(), userIDFrom(r.Context()), f)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.modLog(r, "notice_create", "notice", id, nil, f.Title)
	n, err := s.store.AreaNotice(r.Context(), id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) handleUpdateNotice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req noticeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	f, errs := req.fields()
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if err := s.store.UpdateAreaNotice(r.Context(), id, f); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "notice not found")
		return
	} else if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.modLog(r, "notice_update", "notice", id, nil, f.Title)
	n, err := s.store.AreaNotice(r.Context(), id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleDeleteNotice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteAreaNotice(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "notice not found")
		return
	} else if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.modLog(r, "notice_delete", "notice", id, nil, "")
	w.WriteHeader(http.StatusNoContent)
}

// handleNotices: GET /notices — действующие; ?all=1 — и прошедшие (модераторам в админке).
func (s *Server) handleNotices(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.AreaNotices(r.Context(), r.URL.Query().Get("all") == "1")
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

var noticeKindText = map[string]string{"fire": "Противопожарный режим", "closure": "Тропа закрыта", "flood": "Паводок", "other": "Внимание"}

func noticePeriod(n store.AreaNotice) string {
	day := func(s string) string {
		t, err := time.Parse(time.DateOnly, s)
		if err != nil {
			return s
		}
		return t.Format("02.01.2006")
	}
	if n.EndsOn == nil {
		return "с " + day(n.StartsOn) + ", до отмены"
	}
	return day(n.StartsOn) + " — " + day(*n.EndsOn)
}
