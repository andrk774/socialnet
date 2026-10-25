package httpapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/store"
	"socialnet/internal/textparse"
)

const maxPhotos = 10

type createPostRequest struct {
	Text        string   `json:"text"`
	PhotoIDs    []string `json:"photo_ids"`
	TrackIDs    []string `json:"track_ids"`
	VideoIDs    []string `json:"video_ids"`
	CommunityID *int64   `json:"community_id"`
}

func postCursor(p store.Post) int64 { return p.ID }

func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	var req createPostRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	me := userIDFrom(r.Context())

	req.Text = strings.TrimSpace(req.Text)
	errs := map[string]string{}
	if utf8.RuneCountInString(req.Text) > 10000 {
		errs["text"] = "max 10000 characters"
	}
	photos, perr := dedupeUUIDs(req.PhotoIDs, maxPhotos)
	if perr != "" {
		errs["photo_ids"] = perr
	}
	tracks, terr := dedupeUUIDs(req.TrackIDs, 10)
	if terr != "" {
		errs["track_ids"] = terr
	}
	videos, verr := dedupeUUIDs(req.VideoIDs, 5)
	if verr != "" {
		errs["video_ids"] = verr
	}
	if req.Text == "" && len(photos)+len(tracks)+len(videos) == 0 {
		errs["text"] = "post must have text or attachments"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}

	// Бот публикует только на стене своего сообщества.
	if bot := viewerFrom(r.Context()).Bot; bot != 0 && (req.CommunityID == nil || *req.CommunityID != bot) {
		writeError(w, http.StatusForbidden, "forbidden", "bots can only post to their own community")
		return
	}
	// Публиковать от имени сообщества могут редакторы и выше.
	if req.CommunityID != nil {
		role, err := s.store.CommunityRole(r.Context(), *req.CommunityID, me)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if !store.RoleAtLeast(role, "editor") {
			writeError(w, http.StatusForbidden, "forbidden", "only editors can post to this community")
			return
		}
	}

	id, err := s.store.CreatePost(r.Context(), store.NewPost{
		AuthorID: me, CommunityID: req.CommunityID, Text: req.Text,
		MediaIDs: photos, TrackIDs: tracks, VideoIDs: videos, Hashtags: textparse.Hashtags(req.Text),
	})
	if errors.Is(err, store.ErrBadMedia) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"attachments": "unknown attachment, or photo is not yours, or video is not ready"})
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	s.notifyMentions(r.Context(), req.Text, me, id, nil)
	s.m.business.Inc("post_created")

	post, err := s.store.PostForViewer(r.Context(), me, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if req.CommunityID != nil {
		s.emit(r.Context(), *req.CommunityID, "wall_post_new", post)
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
		Text string `json:"text"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if post.Author.ID != userIDFrom(r.Context()) { // редактировать может только автор
		writeError(w, http.StatusForbidden, "forbidden", "only the author can edit")
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if utf8.RuneCountInString(req.Text) > 10000 || (req.Text == "" && len(post.Photos) == 0) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"text": "1-10000 characters"})
		return
	}
	if err := s.store.UpdatePostText(r.Context(), post.ID, req.Text, textparse.Hashtags(req.Text)); err != nil {
		s.internalError(w, r, err)
		return
	}
	updated, err := s.store.PostForViewer(r.Context(), userIDFrom(r.Context()), post.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
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
