package httpapi

import (
	"context"
	"net/http"

	"socialnet/internal/store"
)

// syncHikeChat сверяет чат похода с группой после любого изменения состава
// и пишет в чат, кто пришёл и кто ушёл. Ошибка не мешает основному
// действию: чат догонит при следующей сверке.
func (s *Server) syncHikeChat(ctx context.Context, eventID int64) {
	res, err := s.store.SyncHikeChat(ctx, eventID)
	if err != nil {
		s.log.Error("hike chat sync", "event", eventID, "err", err)
		return
	}
	if res.ChatID == 0 {
		return
	}
	owner := res.Owner
	c, err := s.store.ConvAccess(ctx, res.ChatID, owner)
	if err != nil {
		s.log.Error("hike chat", "event", eventID, "err", err)
		return
	}
	if res.Created {
		s.serviceMessage(ctx, c, owner, "чат похода «"+c.Title+"»: здесь все, кто идёт. Состав меняется вместе с группой")
	}
	for _, id := range res.Added {
		if id == owner && res.Created {
			continue
		}
		if u, err := s.store.UserByID(ctx, id); err == nil {
			s.serviceMessage(ctx, c, id, shortName(u.FirstName, u.Username)+" в группе похода")
		}
	}
	for _, id := range res.Removed {
		if u, err := s.store.UserByID(ctx, id); err == nil {
			s.serviceMessage(ctx, c, owner, shortName(u.FirstName, u.Username)+" больше не в группе похода")
		}
	}
}

// hikeChatLocked: состав, название и роли чата похода меняются только
// вместе с походом.
func hikeChatLocked(w http.ResponseWriter, conv *store.ConvInfo) bool {
	if conv.EventID == nil {
		return false
	}
	writeError(w, http.StatusConflict, "hike_chat", "the hike chat follows the hike: join or leave the hike instead")
	return true
}

func shortName(first, username string) string {
	if first != "" {
		return first
	}
	return username
}
