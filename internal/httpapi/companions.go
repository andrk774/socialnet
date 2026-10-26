package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"socialnet/internal/store"
)

// «Ищу попутчиков». Объявление видят все, кроме заблокированных; отклик
// уходит автору личным сообщением — разместив объявление, автор согласился
// принимать отклики, даже если пишет ему не друг.

const dateLayout = "2006-01-02"

func parseDate(v string) (*time.Time, bool) {
	if v == "" {
		return nil, true
	}
	t, err := time.Parse(dateLayout, v)
	if err != nil {
		return nil, false
	}
	return &t, true
}

func (s *Server) handleCreateCompanionAd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Region     string  `json:"region"`
		Title      string  `json:"title"`
		Body       string  `json:"body"`
		DateFrom   string  `json:"date_from"`
		DateTo     string  `json:"date_to"`
		Difficulty *int    `json:"difficulty"`
		Seats      *int    `json:"seats"`
		RouteID    *string `json:"route_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	me := userIDFrom(r.Context())
	errs := map[string]string{}
	a := store.NewCompanionAd{AuthorID: me, Region: cleanText(req.Region, 80), Title: cleanText(req.Title, 120),
		Body: strings.TrimSpace(req.Body), Difficulty: req.Difficulty, Seats: 1}
	if a.Region == "" {
		errs["region"] = "1-80 characters"
	}
	if a.Title == "" {
		errs["title"] = "1-120 characters"
	}
	if utf8.RuneCountInString(a.Body) > 2000 {
		errs["body"] = "max 2000 characters"
	}
	from, ok1 := parseDate(req.DateFrom)
	to, ok2 := parseDate(req.DateTo)
	today := time.Now().UTC().Truncate(24 * time.Hour).Add(-24 * time.Hour) // запас на часовые пояса
	switch {
	case !ok1 || from == nil:
		errs["date_from"] = "required, YYYY-MM-DD"
	case from.Before(today):
		errs["date_from"] = "must not be in the past"
	}
	if !ok2 {
		errs["date_to"] = "YYYY-MM-DD"
	} else if from != nil {
		if to == nil {
			to = from
		}
		if to.Before(*from) || to.Sub(*from) > 60*24*time.Hour {
			errs["date_to"] = "on or after date_from, at most 60 days later"
		}
	}
	if a.Difficulty != nil && (*a.Difficulty < 1 || *a.Difficulty > 5) {
		errs["difficulty"] = "1-5 or null"
	}
	if req.Seats != nil {
		a.Seats = *req.Seats
		if a.Seats < 1 || a.Seats > 20 {
			errs["seats"] = "1-20"
		}
	}
	if req.RouteID != nil {
		id := strings.ToLower(*req.RouteID)
		if !validUUID(id) {
			errs["route_id"] = "route not found"
		} else if err := s.checkRoute(r.Context(), me, id, errs); err != nil {
			s.internalError(w, r, err)
			return
		}
		a.RouteID = &id
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	a.DateFrom, a.DateTo = *from, *to
	id, err := s.store.CreateCompanionAd(r.Context(), a)
	if errors.Is(err, store.ErrTooMany) {
		writeError(w, http.StatusUnprocessableEntity, "too_many_ads",
			fmt.Sprintf("at most %d open ads; close one first", store.MaxOpenCompanionAds))
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ad, err := s.store.CompanionAd(r.Context(), me, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.m.business.Inc("companion_ad_created")
	writeJSON(w, http.StatusCreated, ad)
}

// handleCompanionAds: GET /companions?region=&from=&to=&max_difficulty=
func (s *Server) handleCompanionAds(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.CompanionFilter{Region: cleanText(q.Get("region"), 80)}
	var ok1, ok2 bool
	f.From, ok1 = parseDate(q.Get("from"))
	f.To, ok2 = parseDate(q.Get("to"))
	if !ok1 || !ok2 {
		writeError(w, http.StatusBadRequest, "bad_request", "from and to are YYYY-MM-DD")
		return
	}
	if v := q.Get("max_difficulty"); v != "" {
		d, err := strconv.Atoi(v)
		if err != nil || d < 1 || d > 5 {
			writeError(w, http.StatusBadRequest, "bad_request", "max_difficulty must be 1-5")
			return
		}
		f.MaxDifficulty = d
	}
	items, err := s.store.CompanionAds(r.Context(), userIDFrom(r.Context()), f, 100)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleMyCompanionAds(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.MyCompanionAds(r.Context(), userIDFrom(r.Context()), 100)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) companionAd(w http.ResponseWriter, r *http.Request) (*store.CompanionAd, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	ad, err := s.store.CompanionAd(r.Context(), userIDFrom(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "ad not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	return ad, true
}

func (s *Server) handleGetCompanionAd(w http.ResponseWriter, r *http.Request) {
	if ad, ok := s.companionAd(w, r); ok {
		writeJSON(w, http.StatusOK, ad)
	}
}

// handleCloseCompanionAd: PATCH /companions/{id} {"closed": bool} — только автор.
func (s *Server) handleCloseCompanionAd(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Closed *bool `json:"closed"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Closed == nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"closed": "required"})
		return
	}
	me := userIDFrom(r.Context())
	if err := s.store.CloseCompanionAd(r.Context(), id, me, *req.Closed); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "ad not found")
		return
	} else if err != nil {
		s.internalError(w, r, err)
		return
	}
	ad, err := s.store.CompanionAd(r.Context(), me, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ad)
}

func (s *Server) handleDeleteCompanionAd(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteCompanionAd(r.Context(), id, userIDFrom(r.Context())); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "ad not found")
		return
	} else if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleReplyCompanionAd: POST /companions/{id}/reply {"text"} — личное
// сообщение автору с пометкой, на какое объявление это отклик.
func (s *Server) handleReplyCompanionAd(w http.ResponseWriter, r *http.Request) {
	ad, ok := s.companionAd(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	if viewerFrom(r.Context()).Bot != 0 {
		writeError(w, http.StatusForbidden, "bot_cannot_initiate", "bots cannot reply to ads")
		return
	}
	if ad.Author.ID == me {
		writeError(w, http.StatusBadRequest, "bad_request", "this is your own ad")
		return
	}
	if ad.Closed {
		writeError(w, http.StatusConflict, "ad_closed", "the author has already found companions")
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	text := strings.TrimSpace(req.Text)
	if n := utf8.RuneCountInString(text); n < 1 || n > 1000 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"text": "1-1000 characters"})
		return
	}
	// блокировки уже отсечены: заблокированный не видит объявления (404)
	convID, err := s.store.Conversation(r.Context(), me, ad.Author.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	conv, err := s.store.ConvAccess(r.Context(), convID, me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	body := fmt.Sprintf("Отклик на объявление «%s» (%s, %s):\n\n%s", ad.Title, ad.Region, ad.DateFrom, text)
	s.deliver(w, r, conv, sendMessageRequest{Body: body})
}
