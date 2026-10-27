-- Общий поиск по походам, статьям и маршрутам каталога: триграммные
-- индексы под те же выражения, что в запросах (store/search.go).
CREATE INDEX events_hike_search_idx ON events
    USING gin (lower(title || ' ' || place || ' ' || meeting_point || ' ' || description) gin_trgm_ops)
    WHERE kind = 'hike';
CREATE INDEX articles_search_idx ON articles
    USING gin (lower(title || ' ' || body) gin_trgm_ops) WHERE status = 'published';
CREATE INDEX routes_public_search_idx ON routes
    USING gin (lower(title || ' ' || public_note) gin_trgm_ops) WHERE public;
