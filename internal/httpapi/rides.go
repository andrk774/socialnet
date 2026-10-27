package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"socialnet/internal/store"
)

// Подвоз к началу маршрута. Водитель предлагает места (можно привязать к
// походу), попутчики просятся с комментарием, водитель принимает — места
// считаются под блокировкой поездки. Деньги приложение не принимает: о
// складчине на бензин договариваются в личных сообщениях.

func (s *Server) notifyRide(r *http.Request, users []int64, typ string, actor, ride int64) {
	for _, u := range users {
		s.notify(r.Context(), store.NewNotification{UserID: u, Type: typ, ActorID: actor, RideID: &ride})
	}
}

func (s *Server) handleCreateRide(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventID   *int64     `json:"event_id"`
		From      string     `json:"from"`
		To        string     `json:"to"`
		DepartsAt *time.Time `json:"departs_at"`
		Seats     int        `json:"seats"`
		Note      string     `json:"note"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	me := userIDFrom(r.Context())
	f := store.RideFields{EventID: req.EventID, From: cleanText(req.From, 150), To: cleanText(req.To, 150),
		Seats: req.Seats, Note: cleanText(req.Note, 500)}
	errs := map[string]string{}
	if f.From == "" {
		errs["from"] = "1-150 characters"
	}
	if f.To == "" {
		errs["to"] = "1-150 characters"
	}
	if f.Seats < 1 || f.Seats > 8 {
		errs["seats"] = "1-8"
	}
	switch {
	case req.DepartsAt == nil:
		errs["departs_at"] = "required (RFC 3339)"
	case req.DepartsAt.Before(time.Now()) || req.DepartsAt.After(time.Now().Add(60*24*time.Hour)):
		errs["departs_at"] = "within the next 60 days"
	default:
		f.DepartsAt = *req.DepartsAt
	}
	if req.EventID != nil {
		if _, err := s.store.Event(r.Context(), me, *req.EventID); errors.Is(err, store.ErrNotFound) {
			errs["event_id"] = "event not found"
		} else if err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	id, err := s.store.CreateRide(r.Context(), me, f)
	if errors.Is(err, store.ErrRateLimited) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many rides today")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ride, err := s.store.Ride(r.Context(), me, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, ride)
}

// handleRides: GET /rides?event_id=&to= — предстоящие поездки.
func (s *Server) handleRides(w http.ResponseWriter, r *http.Request) {
	var event int64
	if v := r.URL.Query().Get("event_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "bad_request", "bad event_id")
			return
		}
		event = n
	}
	items, err := s.store.Rides(r.Context(), userIDFrom(r.Context()), event, cleanText(r.URL.Query().Get("to"), 150), 100)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) rideParam(w http.ResponseWriter, r *http.Request) (*store.Ride, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	ride, err := s.store.Ride(r.Context(), userIDFrom(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "ride not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	return ride, true
}

func (s *Server) handleGetRide(w http.ResponseWriter, r *http.Request) {
	if ride, ok := s.rideParam(w, r); ok {
		writeJSON(w, http.StatusOK, ride)
	}
}

func (s *Server) handleDeleteRide(w http.ResponseWriter, r *http.Request) {
	ride, ok := s.rideParam(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	riders, err := s.store.DeleteRide(r.Context(), ride.ID, me)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusForbidden, "forbidden", "only the driver can cancel the ride")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	for _, u := range riders { // поездки больше нет — ссылки в уведомлении не будет
		s.notify(r.Context(), store.NewNotification{UserID: u, Type: "ride_canceled", ActorID: me})
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRequestSeat: POST /rides/{id}/request {"note"} — попроситься.
func (s *Server) handleRequestSeat(w http.ResponseWriter, r *http.Request) {
	ride, ok := s.rideParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	me := userIDFrom(r.Context())
	if ride.Driver.ID == me {
		writeError(w, http.StatusBadRequest, "bad_request", "this is your own ride")
		return
	}
	if ride.DepartsAt.Before(time.Now()) {
		writeError(w, http.StatusConflict, "ride_departed", "the ride has already departed")
		return
	}
	created, err := s.store.RequestSeat(r.Context(), ride.ID, me, cleanText(req.Note, 300))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if created {
		s.notifyRide(r, []int64{ride.Driver.ID}, "ride_request", me, ride.ID)
	}
	ride, err = s.store.Ride(r.Context(), me, ride.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ride)
}

func (s *Server) handleLeaveRide(w http.ResponseWriter, r *http.Request) {
	ride, ok := s.rideParam(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	accepted, err := s.store.LeaveRide(r.Context(), ride.ID, me)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no request")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if accepted {
		s.notifyRide(r, []int64{ride.Driver.ID}, "ride_left", me, ride.ID)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRideRequests(w http.ResponseWriter, r *http.Request) {
	ride, ok := s.rideParam(w, r)
	if !ok {
		return
	}
	if ride.Driver.ID != userIDFrom(r.Context()) {
		writeError(w, http.StatusForbidden, "forbidden", "only the driver sees requests")
		return
	}
	items, err := s.store.RideRequests(r.Context(), ride.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleDecideSeat: POST /rides/{id}/requests/{uid} {"accept": bool} —
// принять или отказать (принятого — высадить).
func (s *Server) handleDecideSeat(w http.ResponseWriter, r *http.Request) {
	ride, ok := s.rideParam(w, r)
	if !ok {
		return
	}
	uid, ok := pathID(w, r, "uid")
	if !ok {
		return
	}
	var req struct {
		Accept *bool `json:"accept"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Accept == nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"accept": "required"})
		return
	}
	me := userIDFrom(r.Context())
	if ride.Driver.ID != me {
		writeError(w, http.StatusForbidden, "forbidden", "only the driver decides")
		return
	}
	err := s.store.DecideSeat(r.Context(), ride.ID, me, uid, *req.Accept)
	switch {
	case errors.Is(err, store.ErrRideFull):
		writeError(w, http.StatusConflict, "ride_full", "no free seats")
		return
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "request not found")
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	typ := "ride_accepted"
	if !*req.Accept {
		typ = "ride_declined"
	}
	s.notifyRide(r, []int64{uid}, typ, me, ride.ID)
	ride, err = s.store.Ride(r.Context(), me, ride.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ride)
}
