package httpapi

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/store"
)

// Открытый каталог маршрутов: /trails и /trails/{id} — обычные HTML-страницы
// без входа и без скриптов (их читают поисковики и те, у кого ещё нет
// аккаунта). В каталог попадает только то, что владелец открыл сам.

const trailsPerPage = 30

var nf = func(v float64) string { return strconv.FormatFloat(math.Round(v), 'f', 0, 64) }

func kmText(m int) string {
	if m < 1000 {
		return fmt.Sprintf("%d м", m)
	}
	return strings.Replace(strconv.FormatFloat(math.Round(float64(m)/100)/10, 'f', -1, 64), ".", ",", 1) + " км"
}

// sketchPath — линия трека в квадрате w×h (равнопромежуточная проекция).
func sketchPath(pts []store.LatLon, w, h, pad float64) (path string, sx, sy, fx, fy float64) {
	if len(pts) < 2 {
		return "", 0, 0, 0, 0
	}
	var lat0 float64
	for _, p := range pts {
		lat0 += p[0]
	}
	lat0 = lat0 / float64(len(pts)) * math.Pi / 180
	minX, maxX, minY, maxY := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, p := range pts {
		x, y := p[1]*math.Cos(lat0), p[0]
		minX, maxX, minY, maxY = math.Min(minX, x), math.Max(maxX, x), math.Min(minY, y), math.Max(maxY, y)
	}
	scale := math.Min((w-2*pad)/math.Max(maxX-minX, 1e-9), (h-2*pad)/math.Max(maxY-minY, 1e-9))
	ox, oy := (w-(maxX-minX)*scale)/2, (h-(maxY-minY)*scale)/2
	X := func(p store.LatLon) float64 { return ox + (p[1]*math.Cos(lat0)-minX)*scale }
	Y := func(p store.LatLon) float64 { return h - oy - (p[0]-minY)*scale }
	var b strings.Builder
	for i, p := range pts {
		if i == 0 {
			b.WriteString("M")
		} else {
			b.WriteString("L")
		}
		fmt.Fprintf(&b, "%.1f %.1f", X(p), Y(p))
	}
	last := pts[len(pts)-1]
	return b.String(), X(pts[0]), Y(pts[0]), X(last), Y(last)
}

func profilePath(prof []store.ProfilePoint, w, h float64) (line, area string, lo, hi float64) {
	if len(prof) < 2 {
		return "", "", 0, 0
	}
	total := math.Max(prof[len(prof)-1].D, 1)
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, p := range prof {
		lo, hi = math.Min(lo, p.E), math.Max(hi, p.E)
	}
	if hi-lo < 10 {
		hi = lo + 10
	}
	var b strings.Builder
	for i, p := range prof {
		if i == 0 {
			b.WriteString("M")
		} else {
			b.WriteString("L")
		}
		fmt.Fprintf(&b, "%.1f %.1f", p.D/total*w, h-(p.E-lo)/(hi-lo)*(h-8)-4)
	}
	line = b.String()
	area = line + fmt.Sprintf("L%.1f %.1fL0 %.1fZ", w, h, h)
	return line, area, lo, hi
}

var markKindText = map[string]string{"spring": "Родник", "camp": "Стоянка", "ford": "Брод", "danger": "Опасно", "view": "Вид", "other": "Другое"}

const trailsHead = `<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<meta name="description" content="{{.Description}}">
<link rel="canonical" href="{{.Canonical}}">
<meta property="og:type" content="website"><meta property="og:title" content="{{.Title}}">
<meta property="og:description" content="{{.Description}}"><meta property="og:url" content="{{.Canonical}}">
<link rel="icon" href="/static/icon.svg" type="image/svg+xml">
<link rel="stylesheet" href="/static/styles.css">
</head><body class="legal-page"><main class="legal trails">
<p class="trails-top"><a href="/trails">Маршруты Южного Урала</a> · <a href="/">Войти в socialnet</a></p>`

const trailsFoot = `<nav class="legal-nav"><a href="/trails">Все маршруты</a> <a href="/legal/terms">Соглашение</a> <a href="/legal/privacy">Персональные данные</a></nav>
</main></body></html>`

var trailsListTpl = template.Must(template.New("list").Parse(trailsHead + `
<h1>Маршруты Южного Урала</h1>
<p class="muted">Треки, которые авторы открыли для всех: расстояние, набор высоты, профиль и точки — родники, стоянки, броды.</p>
{{if not .Items}}<p>Пока здесь пусто.</p>{{end}}
<ul class="trail-list">{{range .Items}}
<li><a href="/trails/{{.ID}}"><svg class="route-sketch" viewBox="0 0 120 90" width="120" height="90" role="img" aria-label="Схема трека"><path d="{{.Path}}" fill="none" class="route-line" stroke-width="2"/></svg></a>
<div><a class="trail-name" href="/trails/{{.ID}}">{{.Title}}</a><br><span class="muted">{{.Distance}}{{if .Gain}} · ↑ {{.Gain}} м{{end}} · автор {{.Author}}</span></div></li>{{end}}
</ul>
<p class="pager">{{if .Prev}}<a href="/trails?page={{.Prev}}" rel="prev">← Новее</a>{{end}} {{if .Next}}<a href="/trails?page={{.Next}}" rel="next">Старше →</a>{{end}}</p>
` + trailsFoot))

var trailTpl = template.Must(template.New("trail").Parse(trailsHead + `
<h1>{{.R.Title}}</h1>
<p class="muted">Автор {{.Author}} · открыт {{.Since}}</p>
<dl class="facts route-stats">
<div><dt>Расстояние</dt><dd>{{.Distance}}</dd></div>
{{if .HasEle}}<div><dt>Набор / сброс</dt><dd>↑ {{.Gain}} м / ↓ {{.Loss}} м</dd></div>
<div><dt>Высоты</dt><dd>{{.MinEle}}–{{.MaxEle}} м</dd></div>{{end}}
{{if .Moving}}<div><dt>В движении (по треку автора)</dt><dd>{{.Moving}}</dd></div>{{end}}
</dl>
{{if .R.PublicNote}}<div class="event-text">{{range .Note}}<p>{{.}}</p>{{end}}</div>{{end}}
<div class="route-graphics">
<svg class="route-sketch" viewBox="0 0 240 180" role="img" aria-label="Схема трека"><path d="{{.Sketch}}" fill="none" class="route-line" stroke-width="2.5" stroke-linejoin="round" stroke-linecap="round"/>
<circle cx="{{.SX}}" cy="{{.SY}}" r="4.5" class="route-start"/><circle cx="{{.FX}}" cy="{{.FY}}" r="4.5" class="route-finish"/>
<text x="234" y="14" text-anchor="end" class="route-north">С ↑</text></svg>
{{if .ProfLine}}<svg class="route-profile" viewBox="0 0 360 160" role="img" aria-label="Профиль высот: от {{.Lo}} до {{.Hi}} м">
<path d="{{.ProfArea}}" class="route-area"/><path d="{{.ProfLine}}" fill="none" class="route-line" stroke-width="2"/>
<text x="4" y="14" class="route-axis">{{.Hi}} м</text><text x="4" y="154" class="route-axis">{{.Lo}} м</text></svg>{{end}}
</div>
{{if .R.HiddenEndsM}}<p class="muted">Первые и последние {{.R.HiddenEndsM}} м трека автор скрыл.</p>{{end}}
{{if .Marks}}<h2>Точки на маршруте</h2>
<ul class="trail-marks">{{range .Marks}}<li><strong>{{.Kind}}:</strong> {{.Title}}{{if .Note}} — {{.Note}}{{end}}<br><span class="muted">{{.Coords}} · проверено {{.Verified}}</span></li>{{end}}</ul>{{end}}
<p><a class="primary button-link" href="/trails/{{.R.ID}}.gpx">Скачать GPX</a> <a class="ghost button-link" href="/">Карта, погода и походы — в socialnet</a></p>
<p class="muted">Маршрут опубликован пользователем и может быть неточным. Перед выходом проверьте погоду и ограничения на территории; экстренная связь — 112.</p>
` + trailsFoot))

type trailCard struct {
	ID, Title, Path, Distance, Gain, Author string
}

func authorName(u store.UserCard) string {
	if n := strings.TrimSpace(u.FirstName + " " + u.LastName); n != "" {
		return n
	}
	return u.Username
}

func (s *Server) renderHTML(w http.ResponseWriter, r *http.Request, tpl *template.Template, data any, cache string) {
	var b bytes.Buffer
	if err := tpl.Execute(&b, data); err != nil {
		s.internalError(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", cache)
	_, _ = w.Write(b.Bytes())
}

func (s *Server) publicAllowed(w http.ResponseWriter, r *http.Request) bool {
	if !s.publicLimiter.allow(clientIP(r)) {
		http.Error(w, "Слишком много запросов, попробуйте позже", http.StatusTooManyRequests)
		return false
	}
	return true
}

func (s *Server) handleTrails(w http.ResponseWriter, r *http.Request) {
	if !s.publicAllowed(w, r) {
		return
	}
	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			http.NotFound(w, r)
			return
		}
		page = n
	}
	items, err := s.store.PublicRoutes(r.Context(), trailsPerPage+1, (page-1)*trailsPerPage)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	next := 0
	if len(items) > trailsPerPage {
		items, next = items[:trailsPerPage], page+1
	}
	cards := make([]trailCard, 0, len(items))
	for _, it := range items {
		path, _, _, _, _ := sketchPath(it.Preview, 120, 90, 6)
		c := trailCard{ID: it.ID, Title: it.Title, Path: path, Distance: kmText(it.DistanceM), Author: authorName(it.Author)}
		if it.MaxEle != nil {
			c.Gain = nf(float64(it.GainM))
		}
		cards = append(cards, c)
	}
	canon := s.cfg.AppURL + "/trails"
	if page > 1 {
		canon += "?page=" + strconv.Itoa(page)
	}
	s.renderHTML(w, r, trailsListTpl, map[string]any{
		"Title": "Маршруты Южного Урала — socialnet", "Canonical": canon,
		"Description": "Треки походов по Южному Уралу: Таганай, Иремель, Зюраткуль и другие — расстояние, набор высоты, родники и стоянки.",
		"Items":       cards, "Next": next, "Prev": page - 1,
	}, "public, max-age=300")
}

func (s *Server) publicRoute(w http.ResponseWriter, r *http.Request, id string) (*store.PublicRoute, bool) {
	id = strings.ToLower(id)
	if !validUUID(id) {
		http.NotFound(w, r)
		return nil, false
	}
	pr, err := s.store.PublicRouteByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	return pr, true
}

func (s *Server) handleTrail(w http.ResponseWriter, r *http.Request) {
	if !s.publicAllowed(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	if gpxID, ok := strings.CutSuffix(id, ".gpx"); ok {
		s.handleTrailGPX(w, r, gpxID)
		return
	}
	pr, ok := s.publicRoute(w, r, id)
	if !ok {
		return
	}
	marks, err := s.store.RoutePoints(r.Context(), 0, pr.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	type markView struct{ Kind, Title, Note, Coords, Verified string }
	var mv []markView
	for _, m := range marks {
		mv = append(mv, markView{Kind: markKindText[m.Kind], Title: m.Title, Note: m.Note,
			Coords: fmt.Sprintf("%.5f, %.5f", m.Lat, m.Lon), Verified: m.VerifiedOn})
	}
	var pts []store.LatLon
	for _, seg := range pr.Detail {
		pts = append(pts, seg...)
	}
	if len(pts) < 2 {
		pts = pr.Preview
	}
	sk, sx, sy, fx, fy := sketchPath(pts, 240, 180, 12)
	line, area, lo, hi := profilePath(pr.Profile, 360, 160)
	data := map[string]any{
		"R": pr, "Author": authorName(pr.Author), "Since": pr.PublicAt.In(uralTime).Format("02.01.2006"),
		"Distance": kmText(pr.DistanceM), "HasEle": pr.MaxEle != nil, "Gain": nf(float64(pr.GainM)), "Loss": nf(float64(pr.LossM)),
		"Sketch": sk, "SX": sx, "SY": sy, "FX": fx, "FY": fy, "ProfLine": line, "ProfArea": area, "Lo": nf(lo), "Hi": nf(hi),
		"Marks": mv, "Note": strings.Split(pr.PublicNote, "\n"),
		"Title": pr.Title + " — маршрут на socialnet", "Canonical": s.cfg.AppURL + "/trails/" + pr.ID,
	}
	desc := pr.Title + ": " + kmText(pr.DistanceM)
	if pr.MaxEle != nil {
		data["MinEle"], data["MaxEle"] = nf(*pr.MinEle), nf(*pr.MaxEle)
		desc += fmt.Sprintf(", набор %d м, высоты %s–%s м", pr.GainM, nf(*pr.MinEle), nf(*pr.MaxEle))
	}
	if pr.MovingSec > 0 {
		data["Moving"] = fmt.Sprintf("%d ч %d мин", pr.MovingSec/3600, pr.MovingSec%3600/60)
	}
	data["Description"] = desc + "."
	s.renderHTML(w, r, trailTpl, data, "public, max-age=300")
}

func (s *Server) handleTrailGPX(w http.ResponseWriter, r *http.Request, id string) {
	pr, ok := s.publicRoute(w, r, id)
	if !ok {
		return
	}
	h := w.Header()
	h.Set("Content-Disposition", contentDisposition(pr.Title+".gpx"))
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "public, max-age=3600")
	s.serveAVFile(w, r, "routes", pr.ID, "route.gpx", "application/gpx+xml")
}

// handleRobots: в поиск — каталог и документы; приложение (за входом) — нет.
func (s *Server) handleRobots(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = fmt.Fprintf(w, "User-agent: *\nAllow: /trails\nAllow: /legal/\nAllow: /static/\nDisallow: /\n\nSitemap: %s/sitemap.xml\n", s.cfg.AppURL)
}

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	if !s.publicAllowed(w, r) {
		return
	}
	items, err := s.store.PublicRouteIDs(r.Context(), 45000)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	type u struct {
		Loc     string `xml:"loc"`
		LastMod string `xml:"lastmod,omitempty"`
	}
	set := struct {
		XMLName xml.Name `xml:"urlset"`
		NS      string   `xml:"xmlns,attr"`
		URLs    []u      `xml:"url"`
	}{NS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	set.URLs = append(set.URLs, u{Loc: s.cfg.AppURL + "/trails"})
	for _, it := range items {
		set.URLs = append(set.URLs, u{Loc: s.cfg.AppURL + "/trails/" + it.ID, LastMod: it.PublicAt.UTC().Format(time.DateOnly)})
	}
	for _, d := range []string{"terms", "privacy", "consent"} {
		set.URLs = append(set.URLs, u{Loc: s.cfg.AppURL + "/legal/" + d})
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(set)
}
