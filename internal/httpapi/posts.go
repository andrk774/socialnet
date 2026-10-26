package httpapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/store"
	"socialnet/internal/textparse"
)

const maxPhotos = 10

type createPostRequest struct {
	Text        string         `json:"text"`
	PhotoIDs    []string       `json:"photo_ids"`
	TrackIDs    []string       `json:"track_ids"`
	VideoIDs    []string       `json:"video_ids"`
	DocumentIDs []string       `json:"document_ids"`
	CommunityID *int64         `json:"community_id"`
	Audience    string         `json:"audience,omitempty"`     // everyone | friends | only_me | list
	AudienceIDs []int64        `json:"audience_ids,omitempty"` // для list: друзья, которым виден пост
	Poll        *store.NewPoll `json:"poll,omitempty"`
	PublishAt   *time.Time     `json:"publish_at,omitempty"` // отложенная публикация
}

const (
	maxAudience     = 100
	maxDocsPerPost  = 10
	minScheduleLead = 30 * time.Second
	maxScheduleLead = 365 * 24 * time.Hour
)

func postCursor(p store.Post) int64 { return p.ID }

// validate нормализует запрос и возвращает ошибки по полям. at — момент
// публикации (сейчас или запланированный): от него считается срок опроса.
func (req *createPostRequest) validate(at time.Time) map[string]string {
	req.Text = strings.TrimSpace(req.Text)
	errs := map[string]string{}
	if utf8.RuneCountInString(req.Text) > 10000 {
		errs["text"] = "max 10000 characters"
	}
	var e string
	if req.PhotoIDs, e = dedupeUUIDs(req.PhotoIDs, maxPhotos); e != "" {
		errs["photo_ids"] = e
	}
	if req.TrackIDs, e = dedupeUUIDs(req.TrackIDs, 10); e != "" {
		errs["track_ids"] = e
	}
	if req.VideoIDs, e = dedupeUUIDs(req.VideoIDs, 5); e != "" {
		errs["video_ids"] = e
	}
	if req.DocumentIDs, e = dedupeUUIDs(req.DocumentIDs, maxDocsPerPost); e != "" {
		errs["document_ids"] = e
	}
	if req.Poll != nil {
		if pe := validatePoll(req.Poll, at); pe != "" {
			errs["poll"] = pe
		}
	}
	if req.Text == "" && len(req.PhotoIDs)+len(req.TrackIDs)+len(req.VideoIDs)+len(req.DocumentIDs) == 0 && req.Poll == nil {
		errs["text"] = "post must have text, attachments or a poll"
	}
	if req.Audience == "" {
		req.Audience = "everyone"
	}
	switch req.Audience {
	case "everyone", "friends", "only_me":
		if len(req.AudienceIDs) > 0 {
			errs["audience_ids"] = "only for audience=list"
		}
	case "list":
		ids, ae := dedupeIDs(req.AudienceIDs, maxAudience)
		if ae != "" || len(ids) == 0 {
			errs["audience_ids"] = "1-100 friends"
		}
		req.AudienceIDs = ids
	default:
		errs["audience"] = "one of: everyone, friends, only_me, list"
	}
	if req.CommunityID != nil && req.Audience != "everyone" {
		errs["audience"] = "community posts are visible to the community audience"
	}
	return errs
}

func validatePoll(p *store.NewPoll, at time.Time) string {
	p.Question = cleanText(p.Question, 255)
	if p.Question == "" {
		return "question is required (up to 255 characters)"
	}
	if len(p.Options) < 2 || len(p.Options) > 10 {
		return "2-10 options"
	}
	seen := map[string]bool{}
	for i, o := range p.Options {
		o = cleanText(o, 100)
		if o == "" || seen[strings.ToLower(o)] {
			return "options must be non-empty and different"
		}
		seen[strings.ToLower(o)] = true
		p.Options[i] = o
	}
	if p.ClosesAt != nil {
		if !p.ClosesAt.After(at.Add(time.Minute)) || p.ClosesAt.After(at.Add(maxScheduleLead)) {
			return "closes_at must be from 1 minute to 1 year after publication"
		}
		t := p.ClosesAt.UTC().Truncate(time.Second)
		p.ClosesAt = &t
	}
	return ""
}

func dedupeIDs(ids []int64, max int) ([]int64, string) {
	seen := map[int64]bool{}
	out := []int64{}
	for _, id := range ids {
		if id <= 0 {
			return nil, "invalid id"
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) > max {
		return nil, "too many"
	}
	return out, ""
}

// postPermission: может ли пользователь (или бот) публиковать туда, куда просит.
func (s *Server) postPermission(ctx context.Context, me, bot int64, communityID *int64) (int, string, string, error) {
	// Бот публикует только на стене своего сообщества.
	if bot != 0 && (communityID == nil || *communityID != bot) {
		return http.StatusForbidden, "forbidden", "bots can only post to their own community", nil
	}
	// Публиковать от имени сообщества могут редакторы и выше.
	if communityID != nil {
		role, err := s.store.CommunityRole(ctx, *communityID, me)
		if err != nil {
			return 0, "", "", err
		}
		if !store.RoleAtLeast(role, "editor") {
			return http.StatusForbidden, "forbidden", "only editors can post to this community", nil
		}
	}
	return 0, "", "", nil
}

// publishPost создаёт пост и рассылает всё, что положено при публикации.
func (s *Server) publishPost(ctx context.Context, me int64, req createPostRequest, scheduledID *int64) (*store.Post, error) {
	id, err := s.store.CreatePost(ctx, store.NewPost{
		AuthorID: me, CommunityID: req.CommunityID, Text: req.Text,
		MediaIDs: req.PhotoIDs, TrackIDs: req.TrackIDs, VideoIDs: req.VideoIDs, DocumentIDs: req.DocumentIDs,
		Hashtags: textparse.Hashtags(req.Text), Audience: req.Audience, AudienceIDs: req.AudienceIDs,
		Poll: req.Poll, ScheduledID: scheduledID,
	})
	if err != nil {
		return nil, err
	}
	s.notifyMentions(ctx, req.Text, me, id, nil)
	s.m.business.Inc("post_created")
	post, err := s.store.PostForViewer(ctx, me, id)
	if err != nil {
		return nil, err
	}
	if req.CommunityID != nil {
		s.emit(ctx, *req.CommunityID, "wall_post_new", post)
	}
	return post, nil
}

// writePostError — ответ на ошибки вложений и аудитории.
func writePostError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, store.ErrBadMedia):
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"attachments": "unknown attachment, or photo/document is not yours, or video is not ready"})
	case errors.Is(err, store.ErrNotFriends):
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"audience_ids": "you can choose only your friends"})
	default:
		return false
	}
	return true
}

func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	var req createPostRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	me := userIDFrom(r.Context())
	now := time.Now()
	at := now
	if req.PublishAt != nil {
		at = *req.PublishAt
	}
	errs := req.validate(at)
	if req.PublishAt != nil && (req.PublishAt.Before(now.Add(minScheduleLead)) || req.PublishAt.After(now.Add(maxScheduleLead))) {
		errs["publish_at"] = "from 30 seconds to 1 year in the future"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if status, code, msg, err := s.postPermission(r.Context(), me, viewerFrom(r.Context()).Bot, req.CommunityID); err != nil {
		s.internalError(w, r, err)
		return
	} else if status != 0 {
		writeError(w, status, code, msg)
		return
	}

	if req.PublishAt != nil {
		s.schedulePost(w, r, me, req)
		return
	}
	post, err := s.publishPost(r.Context(), me, req, nil)
	if writePostError(w, err) {
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, post)
}

func dedupeUUIDs(ids []string, max int) ([]string, string) {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		id = strings.ToLower(id)
		if !validUUID(id) {
			return nil, "invalid id"
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) > max {
		return nil, "too many attachments"
	}
	return out, ""
}

// notifyMentions уведомляет упомянутых @username, если они могут видеть пост.
func (s *Server) notifyMentions(ctx context.Context, text string, actor, postID int64, commentID *int64) {
	users, err := s.store.UsersByUsernames(ctx, textparse.Mentions(text))
	if err != nil {
		s.log.Error("mentions", "err", err)
		return
	}
	for _, u := range users {
		if _, err := s.store.PostForViewer(ctx, u.ID, postID); err != nil {
			continue // упомянутый не видит пост — не сообщаем о его существовании
		}
		pid := postID
		s.notify(ctx, store.NewNotification{UserID: u.ID, Type: "mention", ActorID: actor, PostID: &pid, CommentID: commentID})
	}
}

// visiblePost загружает пост с проверкой видимости. 404, если не видно.
func (s *Server) visiblePost(w http.ResponseWriter, r *http.Request) (*store.Post, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	post, err := s.store.PostForViewer(r.Context(), userIDFrom(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "post not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	return post, true
}

func (s *Server) handleGetPost(w http.ResponseWriter, r *http.Request) {
	if post, ok := s.visiblePost(w, r); ok {
		writeJSON(w, http.StatusOK, post)
	}
}

// canManagePost: автор, а для постов сообщества ещё его админы и владелец.
func (s *Server) canManagePost(ctx context.Context, me int64, post *store.Post) (bool, error) {
	if post.Author.ID == me {
		return true, nil
	}
	if post.Community != nil {
		role, err := s.store.CommunityRole(ctx, post.Community.ID, me)
		return store.RoleAtLeast(role, "admin"), err
	}
	return false, nil
}

func (s *Server) handleUpdatePost(w http.ResponseWriter, r *http.Request) {
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}
	var req struct {
		Text        *string `json:"text"`
		Audience    *string `json:"audience"`
		AudienceIDs []int64 `json:"audience_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	me := userIDFrom(r.Context())
	if post.Author.ID != me { // редактировать может только автор
		writeError(w, http.StatusForbidden, "forbidden", "only the author can edit")
		return
	}
	errs := map[string]string{}
	if req.Text == nil && req.Audience == nil {
		errs["text"] = "nothing to change: send text and/or audience"
	}
	hasContent := len(post.Photos)+len(post.Tracks)+len(post.Videos)+len(post.Documents) > 0 || post.Poll != nil
	if req.Text != nil {
		t := strings.TrimSpace(*req.Text)
		req.Text = &t
		if utf8.RuneCountInString(t) > 10000 || (t == "" && !hasContent) {
			errs["text"] = "1-10000 characters"
		}
	}
	if req.Audience != nil {
		probe := createPostRequest{Text: "x", Audience: *req.Audience, AudienceIDs: req.AudienceIDs}
		if post.Community != nil {
			probe.CommunityID = &post.Community.ID
		}
		for k, v := range probe.validate(time.Now()) {
			errs[k] = v
		}
		req.Audience, req.AudienceIDs = &probe.Audience, probe.AudienceIDs
	} else if len(req.AudienceIDs) > 0 {
		errs["audience_ids"] = "send together with audience=list"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if req.Text != nil {
		if err := s.store.UpdatePostText(r.Context(), post.ID, *req.Text, textparse.Hashtags(*req.Text)); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	if req.Audience != nil {
		err := s.store.SetPostAudience(r.Context(), post.ID, me, *req.Audience, req.AudienceIDs)
		if writePostError(w, err) {
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	updated, err := s.store.PostForViewer(r.Context(), me, post.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handlePostAudience: GET /posts/{id}/audience — кому виден пост (только автору).
func (s *Server) handlePostAudience(w http.ResponseWriter, r *http.Request) {
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}
	if post.Author.ID != userIDFrom(r.Context()) {
		writeError(w, http.StatusForbidden, "forbidden", "only the author can see the audience")
		return
	}
	ids, err := s.store.PostAudience(r.Context(), post.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"audience": post.Audience, "user_ids": ids})
}

func (s *Server) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}
	allowed, err := s.canManagePost(r.Context(), userIDFrom(r.Context()), post)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden", "not allowed")
		return
	}
	if err := s.store.DeletePost(r.Context(), post.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWall(w http.ResponseWriter, r *http.Request) {
	target, rel, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	if !canSee(target.ProfileVisibility, target.ID == me, rel) {
		writeError(w, http.StatusForbidden, "private", "profile is closed")
		return
	}
	p := pageParams(r)
	posts, err := s.store.Wall(r.Context(), me, target.ID, p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(posts, p.Limit, postCursor))
}

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r)
	posts, err := s.store.Feed(r.Context(), userIDFrom(r.Context()), p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(posts, p.Limit, postCursor))
}

var tagRe = regexp.MustCompile(`^[\p{L}\p{N}_]{1,64}$`)

func (s *Server) handleHashtag(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	if !tagRe.MatchString(tag) {
		writeError(w, http.StatusNotFound, "not_found", "unknown hashtag")
		return
	}
	p := pageParams(r)
	posts, err := s.store.HashtagPosts(r.Context(), userIDFrom(r.Context()), strings.ToLower(tag), p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(posts, p.Limit, postCursor))
}

func (s *Server) handleLike(w http.ResponseWriter, r *http.Request) {
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	count, created, err := s.store.Like(r.Context(), post.ID, me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if created {
		pid := post.ID
		s.notify(r.Context(), store.NewNotification{UserID: post.Author.ID, Type: "like", ActorID: me, PostID: &pid})
	}
	writeJSON(w, http.StatusOK, map[string]any{"likes_count": count, "liked_by_me": true})
}

func (s *Server) handleUnlike(w http.ResponseWriter, r *http.Request) {
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}
	count, err := s.store.Unlike(r.Context(), post.ID, userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"likes_count": count, "liked_by_me": false})
}

func commentCursor(c store.Comment) int64 { return c.ID }

func (s *Server) handleComments(w http.ResponseWriter, r *http.Request) {
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}
	p := pageParams(r)
	items, err := s.store.Comments(r.Context(), userIDFrom(r.Context()), post.ID, p.After, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, commentCursor))
}

func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}
	var req struct {
		Text      string `json:"text"`
		ReplyToID *int64 `json:"reply_to_id"` // ответ на комментарий
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if n := utf8.RuneCountInString(req.Text); n < 1 || n > 2000 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"text": "1-2000 characters"})
		return
	}
	me := userIDFrom(r.Context())
	if bot := viewerFrom(r.Context()).Bot; bot != 0 && (post.Community == nil || post.Community.ID != bot) {
		writeError(w, http.StatusForbidden, "forbidden", "bots can only comment in their own community")
		return
	}

	// Ответ: комментарий должен быть из этого же поста (иначе можно было бы
	// «привязать» ответ к чужой закрытой ветке). Ветка — одноуровневая.
	var parentID *int64
	var replyAuthor int64
	if req.ReplyToID != nil {
		meta, err := s.store.CommentMeta(r.Context(), *req.ReplyToID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && meta.PostID != post.ID) {
			writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
				map[string]string{"reply_to_id": "comment not found in this post"})
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		root := *req.ReplyToID
		if meta.ParentID != nil {
			root = *meta.ParentID
		}
		parentID, replyAuthor = &root, meta.AuthorID
	}

	id, err := s.store.AddComment(r.Context(), post.ID, me, req.Text, parentID, req.ReplyToID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	pid, cid := post.ID, id
	s.notify(r.Context(), store.NewNotification{UserID: post.Author.ID, Type: "comment", ActorID: me, PostID: &pid, CommentID: &cid})
	if replyAuthor != 0 && replyAuthor != post.Author.ID {
		s.notify(r.Context(), store.NewNotification{UserID: replyAuthor, Type: "reply", ActorID: me, PostID: &pid, CommentID: &cid})
	}
	s.notifyMentions(r.Context(), req.Text, me, post.ID, &cid)

	out := map[string]any{"id": id, "post_id": post.ID, "parent_id": parentID, "reply_to_id": req.ReplyToID,
		"text": req.Text, "author_id": me}
	if post.Community != nil {
		s.emit(r.Context(), post.Community.ID, "wall_reply_new", out)
	}
	writeJSON(w, http.StatusCreated, out)
}

// Удалить комментарий может его автор, автор поста или админ сообщества.
func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	meta, err := s.store.CommentMeta(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "comment not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	post, err := s.store.PostForViewer(r.Context(), me, meta.PostID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "comment not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	allowed := meta.AuthorID == me
	if !allowed {
		if allowed, err = s.canManagePost(r.Context(), me, post); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden", "not allowed")
		return
	}
	if err := s.store.DeleteComment(r.Context(), id); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleReplies: GET /comments/{id}/replies — ответы в ветке.
func (s *Server) handleReplies(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	meta, err := s.store.CommentMeta(r.Context(), id)
	if err == nil {
		_, err = s.store.PostForViewer(r.Context(), me, meta.PostID) // ветка видна, только если виден пост
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "comment not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	p := pageParams(r)
	items, err := s.store.Replies(r.Context(), me, id, p.After, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, commentCursor))
}

// handleReact: POST /posts/{id}/reactions {"reaction": "love"} — поставить или сменить.
func (s *Server) handleReact(w http.ResponseWriter, r *http.Request) {
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}
	var req struct {
		Reaction string `json:"reaction"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	oneOf(errs, "reaction", &req.Reaction, store.Reactions...)
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := userIDFrom(r.Context())
	count, created, err := s.store.React(r.Context(), post.ID, me, req.Reaction)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if created {
		pid := post.ID
		s.notify(r.Context(), store.NewNotification{UserID: post.Author.ID, Type: "like", ActorID: me, PostID: &pid})
	}
	writeJSON(w, http.StatusOK, map[string]any{"likes_count": count, "my_reaction": req.Reaction})
}
