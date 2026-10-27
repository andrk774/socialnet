package store

import "context"

// Общий поиск по походной части: ближайшие походы, статьи, маршруты
// каталога. Права те же, что у списков: видно только то, что и так видно.

func (s *Store) SearchHikes(ctx context.Context, viewer int64, q string, limit int) ([]Event, error) {
	return s.queryEvents(ctx, eventSelect+`
		AND e.kind = 'hike' AND COALESCE(e.ends_at, e.starts_at + interval '6 hours') > now()
		AND lower(e.title || ' ' || e.place || ' ' || e.meeting_point || ' ' || e.description) LIKE $2
		AND NOT EXISTS (SELECT 1 FROM blocks b WHERE (b.blocker_id = $1 AND b.blocked_id = e.creator_id)
		                                         OR (b.blocker_id = e.creator_id AND b.blocked_id = $1))
		ORDER BY e.starts_at, e.id LIMIT $3`, viewer, likePattern(q), limit)
}

func (s *Store) SearchArticles(ctx context.Context, viewer int64, q string, limit int) ([]Article, error) {
	return s.queryArticles(ctx, false, ` AND a.status = 'published' AND lower(a.title || ' ' || a.body) LIKE $2
		ORDER BY similarity(lower(a.title), lower($3)) DESC, a.published_at DESC LIMIT $4`, viewer, likePattern(q), q, limit)
}

func (s *Store) SearchPublicRoutes(ctx context.Context, q string, limit int) ([]PublicRoute, error) {
	return s.scanPublic(ctx, false, ` AND lower(r.title || ' ' || r.public_note) LIKE $1
		ORDER BY similarity(lower(r.title), lower($2)) DESC, r.public_at DESC LIMIT $3`, likePattern(q), q, limit)
}
