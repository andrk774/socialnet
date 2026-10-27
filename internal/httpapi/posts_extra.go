package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/store"
)

// ---------- опросы ----------

// handleVote: POST /posts/{id}/poll/votes {"option_ids": [..]} — проголосовать.
func (s *Server) handleVote(w http.ResponseWriter, r *http.Request) {
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}
	var req struct {
		OptionIDs []int64 `json:"option_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	ids, e := dedupeIDs(req.OptionIDs, 10)
	if e != "" || len(ids) == 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"option_ids": "1-10 option ids"})
		return
	}
	s.pollResult(w, r, post.ID, s.store.Vote(r.Context(), post.ID, userIDFrom(r.Context()), ids))
}

// handleRetractVote: DELETE /posts/{id}/poll/votes — отозвать голос.
func (s *Server) handleRetractVote(w http.ResponseWriter, r *http.Request) {
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}
	s.pollResult(w, r, post.ID, s.store.Retract(r.Context(), post.ID, userIDFrom(r.Context())))
}

func (s *Server) pollResult(w http.ResponseWriter, r *http.Request, postID int64, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "this post has no poll")
	case errors.Is(err, store.ErrPollClosed):
		writeError(w, http.StatusConflict, "poll_closed", "the poll is closed")
	case errors.Is(err, store.ErrAlreadyVoted):
		writeError(w, http.StatusConflict, "already_voted", "you have already voted; retract the vote to change it")
	case errors.Is(err, store.ErrNotVoted):
		writeError(w, http.StatusConflict, "not_voted", "you have not voted")
	case errors.Is(err, store.ErrBadOption):
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"option_ids": "unknown option, or several options in a single-choice poll"})
	case err != nil:
		s.internalError(w, r, err)
	default:
		post, err := s.store.PostForViewer(r.Context(), userIDFrom(r.Context()), postID)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, post.Poll)
	}
}

// handlePollVoters: GET /posts/{id}/poll/options/{oid}/voters — кто выбрал вариант.
// В анонимных опросах — 403. До своего голоса — тоже нельзя (как и счётчики).
func (s *Server) handlePollVoters(w http.ResponseWriter, r *http.Request) {
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}
	oid, ok := pathID(w, r, "oid")
	if !ok {
		return
	}
	if post.Poll == nil {
		writeError(w, http.StatusNotFound, "not_found", "this post has no poll")
		return
	}
	if post.Poll.ResultsHidden {
		writeError(w, http.StatusForbidden, "results_hidden", "vote first to see the results")
		return
	}
	p := pageParams(r)
	items, err := s.store.PollVoters(r.Context(), post.ID, oid, p.After, p.Limit)
	switch {
	case errors.Is(err, store.ErrBadOption):
		writeError(w, http.StatusNotFound, "not_found", "option not found")
	case errors.Is(err, store.ErrAnonymous):
		writeError(w, http.StatusForbidden, "anonymous", "the poll is anonymous")
	case err != nil:
		s.internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, list(items, p.Limit, func(u store.UserCard) int64 { return u.ID }))
	}
}

// ---------- закладки ----------

const maxBookmarks = 5000

// bookmarkTarget проверяет вид и id закладки и то, что объект виден.
func (s *Server) bookmarkTarget(w http.ResponseWriter, r *http.Request, checkVisible bool) (string, string, bool) {
	kind, target := chi.URLParam(r, "kind"), strings.ToLower(chi.URLParam(r, "target"))
	notFound := func() (string, string, bool) {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return "", "", false
	}
	me := userIDFrom(r.Context())
	switch kind {
	case "post":
		id, err := strconv.ParseInt(target, 10, 64)
		if err != nil || id <= 0 {
			return notFound()
		}
		target = strconv.FormatInt(id, 10)
		if checkVisible {
			if _, err := s.store.PostForViewer(r.Context(), me, id); errors.Is(err, store.ErrNotFound) {
				return notFound()
			} else if err != nil {
				s.internalError(w, r, err)
				return "", "", false
			}
		}
	case "video":
		if !validUUID(target) {
			return notFound()
		}
		if checkVisible {
			v, err := s.store.Video(r.Context(), target)
			if errors.Is(err, store.ErrNotFound) {
				return notFound()
			}
			if err != nil {
				s.internalError(w, r, err)
				return "", "", false
			}
			ok, err := s.videoVisible(r.Context(), me, v)
			if err != nil {
				s.internalError(w, r, err)
				return "", "", false
			}
			if !ok {
				return notFound()
			}
		}
	case "track":
		if !validUUID(target) {
			return notFound()
		}
		if checkVisible {
			t, err := s.store.Track(r.Context(), me, target)
			if errors.Is(err, store.ErrNotFound) || (err == nil && t.Removed) {
				return notFound()
			}
			if err != nil {
				s.internalError(w, r, err)
				return "", "", false
			}
		}
	default:
		return notFound()
	}
	return kind, target, true
}

func (s *Server) handleAddBookmark(w http.ResponseWriter, r *http.Request) {
	kind, target, ok := s.bookmarkTarget(w, r, true)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	n, err := s.store.CountBookmarks(r.Context(), me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if n >= maxBookmarks {
		writeError(w, http.StatusConflict, "limit_reached", "too many bookmarks, remove some first")
		return
	}
	if err := s.store.AddBookmark(r.Context(), me, kind, target); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveBookmark(w http.ResponseWriter, r *http.Request) {
	kind, target, ok := s.bookmarkTarget(w, r, false) // убрать можно и то, что уже не видно
	if !ok {
		return
	}
	if err := s.store.RemoveBookmark(r.Context(), userIDFrom(r.Context()), kind, target); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleBookmarks: GET /bookmarks?kind=post|video|track&before=<cursor>
func (s *Server) handleBookmarks(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "post"
	}
	if kind != "post" && kind != "video" && kind != "track" {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"kind": "one of: post, video, track"})
		return
	}
	me := userIDFrom(r.Context())
	p := pageParams(r)
	items, next, err := s.store.Bookmarks(r.Context(), me, kind, p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if kind == "video" { // видео подчиняются приватности профиля владельца
		kept := items[:0]
		for _, b := range items {
			ok, err := s.videoVisible(r.Context(), me, b.Video)
			if err != nil {
				s.internalError(w, r, err)
				return
			}
			if ok {
				kept = append(kept, b)
			}
		}
		items = kept
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": next})
}

// ---------- отложенные публикации ----------

func (s *Server) schedulePost(w http.ResponseWriter, r *http.Request, me int64, req createPostRequest) {
	err := s.store.CheckPostAttachments(r.Context(), me, store.NewPost{
		MediaIDs: req.PhotoIDs, TrackIDs: req.TrackIDs, VideoIDs: req.VideoIDs, DocumentIDs: req.DocumentIDs,
		RouteIDs: req.RouteIDs, Audience: req.Audience, AudienceIDs: req.AudienceIDs,
	})
	if writePostError(w, err) {
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	at := req.PublishAt.UTC().Truncate(time.Second)
	req.PublishAt = nil
	payload, err := json.Marshal(req)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	sp, err := s.store.CreateScheduled(r.Context(), me, req.CommunityID, at, payload)
	if errors.Is(err, store.ErrTooManyScheduled) {
		writeError(w, http.StatusConflict, "limit_reached", "too many scheduled posts")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, sp)
}

// handleScheduledList: GET /scheduled-posts[?community_id=] — свои или сообщества (редакторам).
func (s *Server) handleScheduledList(w http.ResponseWriter, r *http.Request) {
	me := userIDFrom(r.Context())
	var community *int64
	if v := r.URL.Query().Get("community_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusNotFound, "not_found", "community not found")
			return
		}
		role, err := s.store.CommunityRole(r.Context(), id, me)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if !store.RoleAtLeast(role, "editor") {
			writeError(w, http.StatusForbidden, "forbidden", "only editors see scheduled posts")
			return
		}
		community = &id
	}
	items, err := s.store.ScheduledList(r.Context(), me, community)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// scheduledParam: отложенный пост, которым управляет автор или админ сообщества.
func (s *Server) scheduledParam(w http.ResponseWriter, r *http.Request) (*store.ScheduledPost, bool) {
	id, ok := pathID(w, r, "sid")
	if !ok {
		return nil, false
	}
	sp, err := s.store.Scheduled(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "scheduled post not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	me := userIDFrom(r.Context())
	allowed := sp.AuthorID == me
	if !allowed && sp.CommunityID != nil {
		role, err := s.store.CommunityRole(r.Context(), *sp.CommunityID, me)
		if err != nil {
			s.internalError(w, r, err)
			return nil, false
		}
		allowed = store.RoleAtLeast(role, "admin")
	}
	if !allowed {
		writeError(w, http.StatusNotFound, "not_found", "scheduled post not found")
		return nil, false
	}
	return sp, true
}

func (s *Server) handleReschedule(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.scheduledParam(w, r)
	if !ok {
		return
	}
	var req struct {
		PublishAt time.Time `json:"publish_at"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	now := time.Now()
	if req.PublishAt.Before(now.Add(minScheduleLead)) || req.PublishAt.After(now.Add(maxScheduleLead)) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"publish_at": "from 30 seconds to 1 year in the future"})
		return
	}
	var payload createPostRequest
	if err := json.Unmarshal(sp.Payload, &payload); err == nil && payload.Poll != nil && payload.Poll.ClosesAt != nil &&
		!payload.Poll.ClosesAt.After(req.PublishAt.Add(time.Minute)) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"publish_at": "the poll would already be closed"})
		return
	}
	err := s.store.Reschedule(r.Context(), sp.ID, req.PublishAt.UTC().Truncate(time.Second))
	if errors.Is(err, store.ErrNotPending) {
		writeError(w, http.StatusConflict, "not_pending", "already published or canceled")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	updated, err := s.store.Scheduled(r.Context(), sp.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleCancelScheduled(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.scheduledParam(w, r)
	if !ok {
		return
	}
	err := s.store.CancelScheduled(r.Context(), sp.ID)
	if errors.Is(err, store.ErrNotPending) {
		writeError(w, http.StatusConflict, "not_pending", "already published or canceled")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PublishDue публикует отложенные посты, время которых пришло. Права
// проверяются заново: автора могли забанить или снять с редакторов.
func (s *Server) PublishDue(ctx context.Context) (int, error) {
	due, err := s.store.DueScheduled(ctx, 50)
	if err != nil {
		return 0, err
	}
	published := 0
	for _, sp := range due {
		ok, err := s.publishScheduled(ctx, sp)
		if err != nil {
			if ctx.Err() != nil {
				return published, ctx.Err()
			}
			// Сбой одной записи не должен останавливать очередь: она первая по
			// времени и иначе блокировала бы все следующие. Запись остаётся
			// в ожидании и будет повторена на следующем проходе.
			s.log.Error("scheduled post", "err", err, "id", sp.ID)
			continue
		}
		if ok {
			published++
		}
	}
	return published, nil
}

// publishScheduled публикует одну запись. ok=false без ошибки — запись
// отмечена неудавшейся (права, вложения) или её уже опубликовали.
func (s *Server) publishScheduled(ctx context.Context, sp store.ScheduledPost) (bool, error) {
	fail := func(msg string) (bool, error) {
		return false, s.store.FailScheduled(ctx, sp.ID, msg)
	}
	var req createPostRequest
	if err := json.Unmarshal(sp.Payload, &req); err != nil {
		return fail("broken payload")
	}
	author, err := s.store.UserByID(ctx, sp.AuthorID)
	if err != nil {
		return false, err
	}
	if author.BannedAt != nil {
		return fail("author is banned")
	}
	var bot int64
	if c, err := s.store.BotCommunity(ctx, sp.AuthorID); err != nil {
		return false, err
	} else if c != nil {
		bot = *c
	}
	status, _, msg, err := s.postPermission(ctx, sp.AuthorID, bot, req.CommunityID)
	if err != nil {
		return false, err
	}
	if status != 0 {
		return fail(msg)
	}
	if errs := req.validate(sp.PublishAt); len(errs) > 0 {
		return fail("invalid post")
	}
	id := sp.ID
	_, err = s.publishPost(ctx, sp.AuthorID, req, &id)
	switch {
	case errors.Is(err, store.ErrNotPending):
		return false, nil // опубликовал другой экземпляр API или запись отменили
	case errors.Is(err, store.ErrBadMedia):
		return fail("an attachment was deleted")
	case errors.Is(err, store.ErrNotFriends):
		return fail("someone in the audience is no longer a friend")
	case err != nil:
		return false, err
	}
	return true, nil
}

func (s *Server) runScheduler(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := s.PublishDue(ctx); err != nil && ctx.Err() == nil {
				s.log.Error("scheduled posts", "err", err)
			} else if n > 0 {
				s.log.Info("scheduled posts published", "count", n)
			}
			if err := s.RunControl(ctx); err != nil && ctx.Err() == nil {
				s.log.Error("hike control deadlines", "err", err)
			}
			if err := s.RunPlusRenewals(ctx); err != nil && ctx.Err() == nil {
				s.log.Error("plus renewals", "err", err)
			}
		}
	}
}

// videoVisible: готовое видео видно, если виден профиль владельца.
func (s *Server) videoVisible(ctx context.Context, me int64, v *store.Video) (bool, error) {
	if v.OwnerID == me {
		return true, nil
	}
	if v.Status != "ready" {
		return false, nil
	}
	owner, err := s.store.UserByID(ctx, v.OwnerID)
	if err != nil {
		return false, err
	}
	rel, err := s.store.Relationship(ctx, me, v.OwnerID)
	if err != nil {
		return false, err
	}
	return owner.BannedAt == nil && canSee(owner.ProfileVisibility, false, rel), nil
}
