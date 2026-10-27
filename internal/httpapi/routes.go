package httpapi

import (
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/av"
	"socialnet/internal/gpx"
	"socialnet/internal/store"
)

// Маршруты из GPX. Исходный файл не хранится и не отдаётся: сервер собирает
// чистый GPX заново — без автора, устройства и времени, а по умолчанию и без
// первых и последних 200 м, чтобы по треку нельзя было найти дом.

const (
	maxGPXBytes      = 15 << 20
	maxRoutesPerPost = 3
	routeDetailPts   = 3000 // для карты
	routePreviewPts  = 150  // схема в ленте
	routeProfilePts  = 120  // профиль высот
	maxRouteWaypts   = 200
)

var hiddenEndsChoices = map[string]int{"0": 0, "200": 200, "500": 500, "1000": 1000}

func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }

func toLatLon(seg []gpx.Point) []store.LatLon {
	out := make([]store.LatLon, len(seg))
	for i, p := range seg {
		out[i] = store.LatLon{round6(p.Lat), round6(p.Lon)}
	}
	return out
}

func (s *Server) handleUploadRoute(w http.ResponseWriter, r *http.Request) {
	me := userIDFrom(r.Context())
	if !s.uploadLimiter.allow(strconv.FormatInt(me, 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many uploads, slow down")
		return
	}
	up, ok := s.receiveUpload(w, r, maxGPXBytes)
	if !ok {
		return
	}
	defer up.cleanup()

	hide := 200
	if v, set := up.Fields["hidden_ends_m"]; set {
		h, ok := hiddenEndsChoices[strings.TrimSpace(v)]
		if !ok {
			writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
				map[string]string{"hidden_ends_m": "one of: 0, 200, 500, 1000"})
			return
		}
		hide = h
	}

	f, err := os.Open(up.Path)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	tr, err := gpx.Parse(io.LimitReader(f, maxGPXBytes))
	_ = f.Close()
	if err != nil {
		msg := "not a valid GPX track"
		switch {
		case errors.Is(err, gpx.ErrTooBig):
			msg = "too many points (max 200 000) or segments"
		case errors.Is(err, gpx.ErrEmpty):
			msg = "the file has no track points"
		case errors.Is(err, gpx.ErrEncoding):
			msg = "save the GPX in UTF-8"
		case errors.Is(err, gpx.ErrBadPoint):
			msg = "the file has invalid coordinates"
		}
		writeError(w, http.StatusUnprocessableEntity, "invalid_gpx", msg)
		return
	}
	if blocked, err := s.store.IsBlocked(r.Context(), up.SHA); err != nil {
		s.internalError(w, r, err)
		return
	} else if blocked {
		writeError(w, http.StatusUnprocessableEntity, "blocked_content", "this file was removed by moderators")
		return
	}

	// Статистика — по полному треку: это честные километры и набор.
	stats := tr.Stats()
	profile := tr.Profile(routeProfilePts)
	// Всё, что видно на карте и попадает в файл, — уже без концов и без времени.
	tr.StripTimes()
	tr.Trim(float64(hide))
	if len(tr.Segments) == 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"hidden_ends_m": "the route is shorter than the hidden start and finish; choose less"})
		return
	}

	detailSegs := tr.Simplify(routeDetailPts)
	detail := make([][]store.LatLon, len(detailSegs))
	bbox := [4]float64{90, 180, -90, -180}
	for i, seg := range detailSegs {
		detail[i] = toLatLon(seg)
	}
	for _, seg := range tr.Segments {
		for _, p := range seg {
			bbox[0], bbox[1] = math.Min(bbox[0], p.Lat), math.Min(bbox[1], p.Lon)
			bbox[2], bbox[3] = math.Max(bbox[2], p.Lat), math.Max(bbox[3], p.Lon)
		}
	}
	for i := range bbox {
		bbox[i] = round6(bbox[i])
	}
	var preview []store.LatLon
	for _, seg := range tr.Simplify(routePreviewPts) {
		preview = append(preview, toLatLon(seg)...)
	}
	var prof []store.ProfilePoint
	for _, p := range profile {
		prof = append(prof, store.ProfilePoint{D: p.D, E: p.E})
	}
	var wps []store.RouteWaypoint
	for _, wp := range tr.Waypoints {
		if len(wps) == maxRouteWaypts {
			break
		}
		wps = append(wps, store.RouteWaypoint{Lat: round6(wp.Lat), Lon: round6(wp.Lon), Ele: wp.Ele, Name: wp.Name})
	}

	title := cleanText(up.Fields["title"], 120)
	if title == "" {
		title = cleanText(tr.Name, 120)
	}
	if title == "" {
		base := filepath.Base(strings.ReplaceAll(up.Filename, `\`, "/"))
		title = cleanText(strings.TrimSuffix(base, filepath.Ext(base)), 120)
	}
	if title == "" {
		title = "Маршрут"
	}

	id := av.NewID()
	dir, _ := s.avStore.Dir("routes", id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		s.internalError(w, r, err)
		return
	}
	dst, _ := s.avStore.Path("routes", id, "route.gpx")
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- путь из проверенного UUID и фиксированного имени
	if err == nil {
		err = tr.WriteGPX(out)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}
	if err == nil {
		err = s.avStore.Commit(r.Context(), "routes", id)
	}
	if err != nil {
		_ = s.avStore.Delete("routes", id)
		s.internalError(w, r, err)
		return
	}

	route, err := s.store.CreateRoute(r.Context(), store.NewRoute{
		ID: id, Owner: me, Title: title,
		DistanceM: int(stats.DistanceM), GainM: int(stats.GainM), LossM: int(stats.LossM),
		MinEle: stats.MinEle, MaxEle: stats.MaxEle,
		DurationSec: int(stats.DurationSec), MovingSec: int(stats.MovingSec), Points: stats.Points,
		HiddenEndsM: hide, BBox: bbox, Preview: preview, Detail: detail, Profile: prof, Waypoints: wps,
		SHA256: up.SHA,
	})
	if err != nil {
		_ = s.avStore.Delete("routes", id)
		s.internalError(w, r, err)
		return
	}
	s.m.business.Inc("route_uploaded")
	writeJSON(w, http.StatusCreated, route)
}

func routeID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.ToLower(chi.URLParam(r, "rid"))
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
		return "", false
	}
	return id, true
}

// visibleRoute: маршрут, который можно смотреть (владелец или зритель записи).
func (s *Server) visibleRoute(w http.ResponseWriter, r *http.Request, withDetail bool) (*store.Route, bool) {
	id, ok := routeID(w, r)
	if !ok {
		return nil, false
	}
	route, err := s.store.Route(r.Context(), id, withDetail)
	if err == nil {
		var allowed bool
		if allowed, err = s.store.RouteAccess(r.Context(), userIDFrom(r.Context()), id); err == nil && !allowed {
			err = store.ErrNotFound
		}
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	return route, true
}

func (s *Server) handleGetRoute(w http.ResponseWriter, r *http.Request) {
	route, ok := s.visibleRoute(w, r, true)
	if !ok {
		return
	}
	marks, err := s.store.RoutePoints(r.Context(), userIDFrom(r.Context()), route.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	route.Marks = marks
	writeJSON(w, http.StatusOK, route)
}

func (s *Server) handleMyRoutes(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r)
	items, err := s.store.MyRoutes(r.Context(), userIDFrom(r.Context()), p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, store.Route.Cursor))
}

func (s *Server) handleRenameRoute(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r)
	if !ok {
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	title := cleanText(req.Title, 120)
	if title == "" {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"title": "1-120 characters"})
		return
	}
	err := s.store.RenameRoute(r.Context(), id, userIDFrom(r.Context()), title)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	route, err := s.store.Route(r.Context(), id, false)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, route)
}

func (s *Server) handleDeleteRoute(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r)
	if !ok {
		return
	}
	err := s.store.DeleteRoute(r.Context(), id, userIDFrom(r.Context()))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.avStore.Delete("routes", id); err != nil {
		s.log.Warn("delete route file", "err", err, "id", id)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRouteGPX: GET /routes/{rid}/gpx — очищенный GPX, только как вложение.
func (s *Server) handleRouteGPX(w http.ResponseWriter, r *http.Request) {
	route, ok := s.visibleRoute(w, r, false)
	if !ok {
		return
	}
	h := w.Header()
	h.Set("Content-Disposition", contentDisposition(route.Title+".gpx"))
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, max-age=3600")
	s.serveAVFile(w, r, "routes", route.ID, "route.gpx", "application/gpx+xml")
}
