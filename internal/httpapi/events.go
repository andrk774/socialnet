package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"socialnet/internal/store"
)

// Мероприятия и походы. У похода есть сложность, число мест, снаряжение,
// маршрут, место сбора и, по желанию, заявки с одобрением организатора.
// Места распределяются под блокировкой строки мероприятия: когда кто-то
// уходит, освободившееся место сразу получает первый из листа ожидания.

// opt различает «поле не прислали» и «прислали null» (сбросить значение).
type opt[T any] struct {
	Set bool
	V   *T
}

func (o *opt[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.V = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.V = &v
	return nil
}

type eventRequest struct {
	Kind         *string        `json:"kind"` // event | hike, только при создании
	Title        *string        `json:"title"`
	Description  *string        `json:"description"`
	Place        *string        `json:"place"`
	StartsAt     *time.Time     `json:"starts_at"` // RFC 3339, с часовым поясом
	EndsAt       opt[time.Time] `json:"ends_at"`
	CommunityID  *int64         `json:"community_id"`
	Difficulty   opt[int]       `json:"difficulty"`
	Capacity     opt[int]       `json:"capacity"`
	Gear         *string        `json:"gear"`
	RouteID      opt[string]    `json:"route_id"`
	MeetingPoint *string        `json:"meeting_point"`
	Approval     *bool          `json:"approval"`
	ControlAt    opt[time.Time] `json:"control_at"` // контрольный срок возвращения (поход)
}

// apply накладывает запрос на поля (при создании — на пустые) и проверяет итог.
func (req *eventRequest) apply(f *store.EventFields, creating bool) map[string]string {
	errs := map[string]string{}
	if req.Title != nil {
		f.Title = cleanText(*req.Title, 200)
	}
	if f.Title == "" {
		errs["title"] = "1-200 characters"
	}
	if req.Place != nil {
		f.Place = cleanText(*req.Place, 200)
	}
	if req.MeetingPoint != nil {
		f.MeetingPoint = cleanText(*req.MeetingPoint, 300)
	}
	if req.Description != nil {
		f.Description = strings.TrimSpace(*req.Description)
		if utf8.RuneCountInString(f.Description) > 5000 {
			errs["description"] = "max 5000 characters"
		}
	}
	if req.Gear != nil {
		f.Gear = strings.TrimSpace(*req.Gear)
		if utf8.RuneCountInString(f.Gear) > 2000 {
			errs["gear"] = "max 2000 characters"
		}
	}
	if req.StartsAt != nil {
		if creating && req.StartsAt.Before(time.Now().Add(-time.Hour)) {
			errs["starts_at"] = "must be in the future"
		}
		f.StartsAt = *req.StartsAt
	} else if creating {
		errs["starts_at"] = "required (RFC 3339)"
	}
	if req.EndsAt.Set {
		f.EndsAt = req.EndsAt.V
	}
	if f.EndsAt != nil && !f.EndsAt.After(f.StartsAt) {
		errs["ends_at"] = "must be after starts_at"
	}
	if req.Difficulty.Set {
		f.Difficulty = req.Difficulty.V
	}
	if f.Difficulty != nil && (*f.Difficulty < 1 || *f.Difficulty > 5) {
		errs["difficulty"] = "1-5 or null"
	}
	if req.Capacity.Set {
		f.Capacity = req.Capacity.V
	}
	if f.Capacity != nil && (*f.Capacity < 1 || *f.Capacity > 500) {
		errs["capacity"] = "1-500 or null"
	}
	if req.RouteID.Set {
		f.RouteID = req.RouteID.V
		if f.RouteID != nil {
			id := strings.ToLower(*f.RouteID)
			f.RouteID = &id
			if !validUUID(id) {
				errs["route_id"] = "invalid route id"
			}
		}
	}
	if req.Approval != nil {
		f.Approval = *req.Approval
	}
	if req.ControlAt.Set {
		f.ControlAt = req.ControlAt.V
	}
	if f.ControlAt != nil {
		switch {
		case !f.ControlAt.After(f.StartsAt):
			errs["control_at"] = "must be after starts_at"
		case f.ControlAt.Sub(f.StartsAt) > 30*24*time.Hour:
			errs["control_at"] = "at most 30 days after starts_at"
		}
	}
	if creating {
		f.Kind = "event"
		defer func() {
			if f.ControlAt != nil && f.Kind != "hike" {
				errs["control_at"] = "only for hikes"
			}
		}()
		if req.Kind != nil {
			f.Kind = *req.Kind
		}
		if f.Kind != "event" && f.Kind != "hike" {
			errs["kind"] = "event or hike"
		}
	} else if req.Kind != nil && *req.Kind != f.Kind {
		errs["kind"] = "cannot be changed"
	}
	return errs
}

// checkRoute: к походу можно приложить только свой маршрут.
func (s *Server) checkRoute(ctx context.Context, me int64, id string, errs map[string]string) error {
	route, err := s.store.Route(ctx, id, false)
	if errors.Is(err, store.ErrNotFound) || err == nil && route.OwnerID != me {
		errs["route_id"] = "route not found"
		return nil
	}
	return err
}

func (s *Server) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	var req eventRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	var f store.EventFields
	errs := req.apply(&f, true)
	me := userIDFrom(r.Context())
	if f.RouteID != nil && errs["route_id"] == "" {
		if err := s.checkRoute(r.Context(), me, *f.RouteID, errs); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if req.CommunityID != nil {
		role, err := s.store.CommunityRole(r.Context(), *req.CommunityID, me)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if !store.RoleAtLeast(role, "editor") {
			writeError(w, http.StatusForbidden, "forbidden", "only editors can create community events")
			return
		}
	}
	id, err := s.store.CreateEvent(r.Context(), store.NewEvent{EventFields: f, CreatorID: me, CommunityID: req.CommunityID})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	e, err := s.store.Event(r.Context(), me, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if f.Kind == "hike" {
		s.m.business.Inc("hike_created")
		s.syncHikeChat(r.Context(), id)
		e.ChatID, _ = s.store.HikeChatFor(r.Context(), id, me)
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) eventParam(w http.ResponseWriter, r *http.Request) (*store.Event, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	e, err := s.store.Event(r.Context(), userIDFrom(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	return e, true
}

// canManageEvent: организатор или админ сообщества.
func (s *Server) canManageEvent(r *http.Request, e *store.Event) (bool, error) {
	me := userIDFrom(r.Context())
	if e.CreatorID == me {
		return true, nil
	}
	if e.Community != nil {
		role, err := s.store.CommunityRole(r.Context(), e.Community.ID, me)
		return store.RoleAtLeast(role, "admin"), err
	}
	return false, nil
}

// managedEvent: мероприятие, которым управляет текущий пользователь.
func (s *Server) managedEvent(w http.ResponseWriter, r *http.Request) (*store.Event, bool) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return nil, false
	}
	allowed, err := s.canManageEvent(r, e)
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden", "only the organizer can do this")
		return nil, false
	}
	return e, true
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	if e.Kind == "hike" {
		me := userIDFrom(r.Context())
		chat, err := s.store.HikeChatFor(r.Context(), e.ID, me)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		// походы, созданные до появления чатов, получают чат при первом заходе участника
		if chat == nil && (e.CreatorID == me || (e.MyStatus != nil && *e.MyStatus == "going")) {
			s.syncHikeChat(r.Context(), e.ID)
			chat, _ = s.store.HikeChatFor(r.Context(), e.ID, me)
		}
		e.ChatID = chat
		s.reviewState(r, e)
		if e.RouteID != nil {
			if rt, err := s.store.RouteBBox(r.Context(), *e.RouteID); err == nil {
				end := e.StartsAt
				if e.EndsAt != nil {
					end = *e.EndsAt
				}
				if e.Notices, err = s.store.NoticesFor(r.Context(), rt, e.StartsAt.In(uralTime), end.In(uralTime)); err != nil {
					s.internalError(w, r, err)
					return
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleUpdateEvent(w http.ResponseWriter, r *http.Request) {
	e, ok := s.managedEvent(w, r)
	if !ok {
		return
	}
	var req eventRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.CommunityID != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "community cannot be changed")
		return
	}
	f := store.EventFields{Kind: e.Kind, Title: e.Title, Description: e.Description, Place: e.Place,
		StartsAt: e.StartsAt, EndsAt: e.EndsAt, Difficulty: e.Difficulty, Capacity: e.Capacity, Gear: e.Gear,
		RouteID: e.RouteID, MeetingPoint: e.MeetingPoint, Approval: e.Approval, ControlAt: e.ControlAt}
	errs := req.apply(&f, false)
	if req.ControlAt.Set && f.ControlAt != nil && e.Kind != "hike" {
		errs["control_at"] = "only for hikes"
	}
	// тревога уже ушла: срок не двигаем, чтобы не рассылать письма повторно
	if req.ControlAt.Set && e.AlertSent && !sameTime(e.ControlAt, f.ControlAt) {
		errs["control_at"] = "the alert has already been sent; mark the group as returned instead"
	}
	me := userIDFrom(r.Context())
	// новый маршрут — только свой; прежний можно оставить, кто бы ни правил
	if f.RouteID != nil && errs["route_id"] == "" && (e.RouteID == nil || *e.RouteID != *f.RouteID) {
		if err := s.checkRoute(r.Context(), me, *f.RouteID, errs); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	promoted, err := s.store.UpdateEvent(r.Context(), e.ID, f)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.notifyEvent(r.Context(), promoted, "event_seat", 0, e.ID)
	s.syncHikeChat(r.Context(), e.ID)
	updated, err := s.store.Event(r.Context(), me, e.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	e, ok := s.managedEvent(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteEvent(r.Context(), e.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) notifyEvent(ctx context.Context, users []int64, typ string, actor, event int64) {
	for _, u := range users {
		s.notify(ctx, store.NewNotification{UserID: u, Type: typ, ActorID: actor, EventID: &event})
	}
}

// handleRSVP: POST /events/{id}/rsvp {"status": "going" | "maybe", "note"?}.
// Ответ — итоговый статус: going, maybe, pending (ждёт организатора) или
// waitlist (мест нет, в очереди).
func (s *Server) handleRSVP(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
		Note   string `json:"note"`
		Source string `json:"source"` // "qr" — пришёл по QR-коду похода
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	oneOf(errs, "status", &req.Status, "going", "maybe")
	note := cleanText(req.Note, 500)
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := userIDFrom(r.Context())
	if rel, err := s.store.Relationship(r.Context(), me, e.CreatorID); err != nil {
		s.internalError(w, r, err)
		return
	} else if rel.Blocked() {
		writeError(w, http.StatusForbidden, "forbidden", "you cannot join this event")
		return
	}
	status, promoted, err := s.store.Attend(r.Context(), e.ID, me, req.Status, note)
	if errors.Is(err, store.ErrNoMaybe) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"status": "a hike has no 'maybe': join or not"})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if req.Source == "qr" && e.MyStatus == nil && status != "maybe" {
		if err := s.store.RecordQRJoin(r.Context(), e.ID); err != nil {
			s.log.Error("qr join", "err", err)
		} else {
			s.m.business.Inc("qr_join")
		}
	}
	if status == "pending" && (e.MyStatus == nil || *e.MyStatus != "pending") {
		s.notifyEvent(r.Context(), []int64{e.CreatorID}, "hike_application", me, e.ID)
	}
	s.notifyEvent(r.Context(), promoted, "event_seat", 0, e.ID)
	s.syncHikeChat(r.Context(), e.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (s *Server) handleCancelRSVP(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	if e.CreatorID == me {
		writeError(w, http.StatusUnprocessableEntity, "organizer_cannot_leave", "the organizer cannot leave; delete the event instead")
		return
	}
	promoted, err := s.store.Leave(r.Context(), e.ID, me)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.internalError(w, r, err)
		return
	}
	s.notifyEvent(r.Context(), promoted, "event_seat", 0, e.ID)
	s.syncHikeChat(r.Context(), e.ID)
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveAttendee: DELETE /events/{id}/attendees/{uid} — организатор
// убирает участника (или человека из очереди); место — следующему.
func (s *Server) handleRemoveAttendee(w http.ResponseWriter, r *http.Request) {
	e, ok := s.managedEvent(w, r)
	if !ok {
		return
	}
	uid, ok := pathID(w, r, "uid")
	if !ok {
		return
	}
	if uid == e.CreatorID {
		writeError(w, http.StatusUnprocessableEntity, "organizer_cannot_leave", "the organizer cannot be removed")
		return
	}
	promoted, err := s.store.Leave(r.Context(), e.ID, uid)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "not a participant")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.notifyEvent(r.Context(), []int64{uid}, "hike_removed", userIDFrom(r.Context()), e.ID)
	s.notifyEvent(r.Context(), promoted, "event_seat", 0, e.ID)
	s.syncHikeChat(r.Context(), e.ID)
	w.WriteHeader(http.StatusNoContent)
}

// handleApplications: GET /events/{id}/applications — заявки (организатору).
func (s *Server) handleApplications(w http.ResponseWriter, r *http.Request) {
	e, ok := s.managedEvent(w, r)
	if !ok {
		return
	}
	items, err := s.store.Applications(r.Context(), e.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleDecideApplication: POST /events/{id}/applications/{uid} {"approve": bool}.
func (s *Server) handleDecideApplication(w http.ResponseWriter, r *http.Request) {
	e, ok := s.managedEvent(w, r)
	if !ok {
		return
	}
	uid, ok := pathID(w, r, "uid")
	if !ok {
		return
	}
	var req struct {
		Approve *bool `json:"approve"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Approve == nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"approve": "required"})
		return
	}
	status, err := s.store.DecideApplication(r.Context(), e.ID, uid, *req.Approve)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "application not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	typ := "hike_approved"
	if status == "rejected" {
		typ = "hike_rejected"
	}
	s.notifyEvent(r.Context(), []int64{uid}, typ, userIDFrom(r.Context()), e.ID)
	s.syncHikeChat(r.Context(), e.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (s *Server) handleAttendees(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "going"
	}
	if status != "going" && status != "maybe" && status != "waitlist" {
		writeError(w, http.StatusBadRequest, "bad_request", "status must be going, maybe or waitlist")
		return
	}
	p := pageParams(r)
	items, err := s.store.Attendees(r.Context(), e.ID, status, p.After, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, cardCursor))
}

func (s *Server) handleUpcomingEvents(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.UpcomingEvents(r.Context(), userIDFrom(r.Context()), 50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleHikes: GET /hikes?max_difficulty=1..5 — ближайшие походы.
func (s *Server) handleHikes(w http.ResponseWriter, r *http.Request) {
	maxDiff := 0
	if v := r.URL.Query().Get("max_difficulty"); v != "" {
		if len(v) != 1 || v[0] < '1' || v[0] > '5' {
			writeError(w, http.StatusBadRequest, "bad_request", "max_difficulty must be 1-5")
			return
		}
		maxDiff = int(v[0] - '0')
	}
	items, err := s.store.Hikes(r.Context(), userIDFrom(r.Context()), maxDiff, 100)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleCommunityEvents(w http.ResponseWriter, r *http.Request) {
	c, ok := s.community(w, r)
	if !ok {
		return
	}
	items, err := s.store.CommunityEvents(r.Context(), userIDFrom(r.Context()), c.ID, 50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func sameTime(a, b *time.Time) bool {
	return a == nil && b == nil || a != nil && b != nil && a.Equal(*b)
}
