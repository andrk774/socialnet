package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"socialnet/internal/store"
)

// Свои эмодзи сообщества: добавляют администраторы (картинка — их фото),
// реагировать ими можно на записи этого сообщества.

var shortcodeRe = regexp.MustCompile(`^[a-z0-9_]{2,32}$`)

func (s *Server) handleCommunityEmoji(w http.ResponseWriter, r *http.Request) {
	c, ok := s.community(w, r)
	if !ok {
		return
	}
	items, err := s.store.CommunityEmoji(r.Context(), c.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleAddEmoji(w http.ResponseWriter, r *http.Request) {
	c, ok := s.communityAdmin(w, r)
	if !ok {
		return
	}
	var req struct {
		Shortcode string `json:"shortcode"`
		MediaID   string `json:"media_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	code := strings.Trim(strings.ToLower(strings.TrimSpace(req.Shortcode)), ":")
	media := strings.ToLower(req.MediaID)
	errs := map[string]string{}
	if !shortcodeRe.MatchString(code) {
		errs["shortcode"] = "2-32 characters: a-z, 0-9, _"
	}
	if !validUUID(media) {
		errs["media_id"] = "photo not found"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	id, err := s.store.AddEmoji(r.Context(), c.ID, userIDFrom(r.Context()), code, media)
	switch {
	case errors.Is(err, store.ErrBadPhoto):
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"media_id": "photo not found"})
		return
	case errors.Is(err, store.ErrDuplicateEmoji):
		writeFieldErrors(w, http.StatusConflict, "conflict", "already exists", map[string]string{"shortcode": "already used in this community"})
		return
	case errors.Is(err, store.ErrTooMany):
		writeError(w, http.StatusUnprocessableEntity, "too_many_emoji", fmt.Sprintf("at most %d emoji", store.MaxEmojiPerCommunity))
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	items, err := s.store.CommunityEmoji(r.Context(), c.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	for _, e := range items {
		if e.ID == id {
			writeJSON(w, http.StatusCreated, e)
			return
		}
	}
	s.internalError(w, r, errors.New("emoji vanished"))
}

func (s *Server) handleDeleteEmoji(w http.ResponseWriter, r *http.Request) {
	c, ok := s.communityAdmin(w, r)
	if !ok {
		return
	}
	eid, ok := pathID(w, r, "eid")
	if !ok {
		return
	}
	if err := s.store.DeleteEmoji(r.Context(), c.ID, eid); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "emoji not found")
		return
	} else if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// customReaction: «e:<id>» допустима только на записи сообщества этого эмодзи.
func (s *Server) customReaction(r *http.Request, post *store.Post, reaction string) (bool, error) {
	n, ok := strings.CutPrefix(reaction, "e:")
	if !ok {
		return false, nil
	}
	id, err := strconv.ParseInt(n, 10, 64)
	if err != nil || id < 1 || post.Community == nil {
		return false, nil
	}
	c, err := s.store.EmojiCommunity(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return err == nil && c == post.Community.ID, err
}
