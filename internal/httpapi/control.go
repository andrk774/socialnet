package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"socialnet/internal/store"
)

// Контрольный срок похода. Участник оставляет до трёх доверенных контактов
// (родные, друзья). Если к сроку группа не отметила «вернулись», контактам
// уходит письмо с тем, что нужно сообщить спасателям. Контакты видит только
// тот, кто их добавил, и они удаляются после возвращения или через 7 дней.

// Южный Урал живёт по екатеринбургскому времени (UTC+5, без перехода).
var uralTime = time.FixedZone("UTC+5", 5*60*60)

func uralFmt(t time.Time) string {
	return t.In(uralTime).Format("02.01.2006 15:04") + " (UTC+5, Екатеринбург)"
}

func (s *Server) handleHikeContacts(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	items, err := s.store.HikeContacts(r.Context(), e.ID, userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleAddHikeContact(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Consent bool   `json:"consent"` // контакт знает и согласен получить письмо
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	name := cleanText(req.Name, 80)
	email := normalizeEmail(req.Email)
	if name == "" {
		errs["name"] = "1-80 characters"
	}
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || len(email) > 254 {
		errs["email"] = "invalid email address"
	}
	if !req.Consent {
		errs["consent"] = "confirm that this person agreed to receive the alert"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := userIDFrom(r.Context())
	user, err := s.store.UserByID(r.Context(), me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if user.EmailVerifiedAt == nil {
		writeError(w, http.StatusForbidden, "email_not_verified", "verify your email first")
		return
	}
	c, err := s.store.AddHikeContact(r.Context(), e.ID, me, name, email)
	switch {
	case errors.Is(err, store.ErrNoControl):
		writeError(w, http.StatusConflict, "no_control_deadline", "the hike has no active control deadline")
	case errors.Is(err, store.ErrNotParticipant):
		writeError(w, http.StatusForbidden, "not_participant", "only participants who are going can add contacts")
	case errors.Is(err, store.ErrTooMany):
		writeError(w, http.StatusUnprocessableEntity, "too_many_contacts", fmt.Sprintf("at most %d contacts per hike", store.MaxContactsPerHike))
	case errors.Is(err, store.ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many contacts added this week")
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "event not found")
	case err != nil:
		s.internalError(w, r, err)
	default:
		writeJSON(w, http.StatusCreated, c)
	}
}

func (s *Server) handleDeleteHikeContact(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	if err := s.store.DeleteHikeContact(r.Context(), cid, e.ID, userIDFrom(r.Context())); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "contact not found")
		return
	} else if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleReturned: POST /events/{id}/returned — группа вернулась. Отметить
// может организатор или любой идущий участник.
func (s *Server) handleReturned(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	allowed := e.MyStatus != nil && *e.MyStatus == "going"
	if !allowed {
		var err error
		if allowed, err = s.canManageEvent(r, e); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden", "only the organizer or a participant can mark the return")
		return
	}
	if e.Kind != "hike" {
		writeError(w, http.StatusBadRequest, "bad_request", "only hikes have a control deadline")
		return
	}
	alerted, err := s.store.MarkReturned(r.Context(), e.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusConflict, "already_returned", "already marked as returned")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if alerted != nil {
		for _, c := range alerted.Contacts {
			s.sendMail(c.Email, "Отбой: группа «"+alerted.Title+"» вернулась",
				"Здравствуйте, "+c.Name+"!\n\n"+
					"Группа похода «"+alerted.Title+"», о которой мы писали вам, отметила, что вернулась ("+uralFmt(time.Now())+").\n"+
					"Если вы обращались в МЧС, сообщите им, что помощь не нужна.\n\n"+
					"Ваш адрес удалён с socialnet.\n")
		}
	}
	s.notifyGoing(r.Context(), e.ID, me, "hike_returned")
	s.log.Info("hike returned", "event_id", e.ID, "after_alert", alerted != nil)
	w.WriteHeader(http.StatusNoContent)
}

// notifyGoing — уведомление всем идущим, кроме actor.
func (s *Server) notifyGoing(ctx context.Context, event, actor int64, typ string) {
	ids, err := s.store.GoingIDs(ctx, event)
	if err != nil {
		s.log.Error("going ids", "err", err)
		return
	}
	s.notifyEvent(ctx, ids, typ, actor, event)
}

// RunControl — напоминания, тревоги и очистка контактов (вызывается планировщиком).
func (s *Server) RunControl(ctx context.Context) error {
	reminders, err := s.store.DueReminders(ctx)
	if err != nil {
		return err
	}
	for _, e := range reminders {
		s.notifyEvent(ctx, e.Participants, "hike_control_soon", 0, e.ID)
	}
	alerts, err := s.store.DueAlerts(ctx)
	if err != nil {
		return err
	}
	for _, e := range alerts {
		s.sendAlerts(e)
		s.notifyEvent(ctx, e.Participants, "hike_alert_sent", 0, e.ID)
		s.log.Warn("hike control deadline missed, alerts sent", "event_id", e.ID, "contacts", len(e.Contacts))
		s.m.business.Inc("hike_alert")
	}
	if n, err := s.store.CleanupHikeContacts(ctx); err != nil {
		return err
	} else if n > 0 {
		s.log.Info("hike contacts deleted", "count", n)
	}
	return nil
}

func (s *Server) sendAlerts(e *store.ControlEvent) {
	where := e.RouteTitle
	if e.Place != "" {
		if where != "" {
			where += ", "
		}
		where += e.Place
	}
	if where == "" {
		where = "не указан"
	}
	meeting := e.MeetingPoint
	if meeting == "" {
		meeting = "не указано"
	}
	for _, c := range e.Contacts {
		who := strings.TrimSpace(c.Participant.FirstName + " " + c.Participant.LastName)
		if who == "" {
			who = c.Participant.Username
		}
		org := strings.TrimSpace(e.Organizer.FirstName + " " + e.Organizer.LastName)
		body := "Здравствуйте, " + c.Name + "!\n\n" +
			who + " указал(а) вас доверенным контактом в походе «" + e.Title + "» на socialnet.\n" +
			"Контрольный срок возвращения — " + uralFmt(e.ControlAt) + ". К этому сроку группа не отметила, что вернулась.\n\n" +
			"Часто это значит лишь, что в горах нет связи. Попробуйте дозвониться до " + who + " и других участников.\n" +
			"Если связаться не удаётся и есть основания беспокоиться, звоните 112 и сообщите:\n" +
			"  — маршрут: " + where + "\n" +
			"  — место сбора: " + meeting + "\n" +
			"  — начало: " + uralFmt(e.StartsAt) + "\n" +
			fmt.Sprintf("  — в группе: %d чел., организатор %s (@%s)\n", e.Going, org, e.Organizer.Username) +
			"  — страница похода: " + s.cfg.AppURL + "/events/" + fmt.Sprint(e.ID) + "\n\n" +
			"Когда группа отметит возвращение, мы напишем вам ещё раз.\n\n" +
			"Если вы не знаете этого человека, просто проигнорируйте письмо: ваш адрес будет удалён в течение 7 дней.\n"
		s.sendMail(c.Email, "Контрольный срок: группа «"+e.Title+"» не отметилась", body)
	}
}
