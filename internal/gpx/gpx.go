// Package gpx разбирает GPX-треки из походов и считает по ним статистику.
//
// Файл приходит от пользователя, поэтому разбор защищён:
//   - XML без DTD: объявления DOCTYPE и сущности отклоняются (нет XXE и «бомб»);
//   - лимиты на число точек, сегментов, путевых точек и длину строк;
//   - координаты и высоты проверяются на диапазон и NaN/Inf.
//
// Наружу отдаётся не исходный файл, а заново собранный GPX: без метаданных
// (автор, почта, устройство), без меток времени и, по умолчанию, без первых и
// последних сотен метров — чтобы по треку нельзя было найти дом.
package gpx

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Лимиты разбора.
const (
	MaxPoints    = 200_000
	MaxSegments  = 200
	MaxWaypoints = 500
	maxText      = 200 // символов в названии трека или точки
	maxDepth     = 32
)

var (
	ErrNotGPX   = errors.New("not a GPX file")
	ErrDTD      = errors.New("DTD and entities are not allowed")
	ErrTooBig   = errors.New("too many points")
	ErrEmpty    = errors.New("no track points")
	ErrBadPoint = errors.New("invalid coordinates")
	ErrEncoding = errors.New("only UTF-8 GPX is supported")
)

type Point struct {
	Lat, Lon float64
	Ele      *float64
	Time     *time.Time
}

type Waypoint struct {
	Lat, Lon float64
	Ele      *float64
	Name     string
}

// Track — разобранный файл: сегменты трека (trkseg и rte) и путевые точки.
type Track struct {
	Name      string
	Segments  [][]Point
	Waypoints []Waypoint
}

// Parse читает GPX. r должен быть уже ограничен по размеру.
func Parse(r io.Reader) (*Track, error) {
	d := xml.NewDecoder(r)
	d.Strict = true
	d.Entity = nil // только пять стандартных сущностей XML
	d.CharsetReader = func(charset string, in io.Reader) (io.Reader, error) {
		if c := strings.ToLower(charset); c == "utf-8" || c == "utf8" || c == "us-ascii" {
			return in, nil
		}
		return nil, ErrEncoding
	}

	t := &Track{}
	var (
		stack  []string
		seg    []Point
		cur    *Point // текущая trkpt/rtept
		wpt    *Waypoint
		text   strings.Builder
		points int
		rootOK bool
	)
	pushSeg := func() error {
		if len(seg) > 0 {
			if len(t.Segments) >= MaxSegments {
				return ErrTooBig
			}
			t.Segments = append(t.Segments, seg)
		}
		seg = nil
		return nil
	}

	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if errors.Is(err, ErrEncoding) {
				return nil, ErrEncoding
			}
			if strings.Contains(err.Error(), "entity") {
				return nil, ErrDTD
			}
			return nil, fmt.Errorf("%w: %w", ErrNotGPX, err)
		}
		switch el := tok.(type) {
		case xml.Directive: // <!DOCTYPE ...>, <!ENTITY ...>
			return nil, ErrDTD
		case xml.StartElement:
			name := el.Name.Local
			if len(stack) == 0 {
				if name != "gpx" {
					return nil, ErrNotGPX
				}
				rootOK = true
			}
			stack = append(stack, name)
			if len(stack) > maxDepth {
				return nil, ErrNotGPX
			}
			text.Reset()
			parent := ""
			if len(stack) > 1 {
				parent = stack[len(stack)-2]
			}
			switch {
			case name == "trkseg":
				if err := pushSeg(); err != nil {
					return nil, err
				}
			case (name == "trkpt" && parent == "trkseg") || (name == "rtept" && parent == "rte"):
				lat, lon, err := latLon(el.Attr)
				if err != nil {
					return nil, err
				}
				points++
				if points > MaxPoints {
					return nil, ErrTooBig
				}
				cur = &Point{Lat: lat, Lon: lon}
			case name == "wpt" && parent == "gpx":
				lat, lon, err := latLon(el.Attr)
				if err != nil {
					return nil, err
				}
				if len(t.Waypoints) >= MaxWaypoints {
					return nil, ErrTooBig
				}
				wpt = &Waypoint{Lat: lat, Lon: lon}
			}
		case xml.CharData:
			if text.Len() < 4*maxText {
				text.Write(el)
			}
		case xml.EndElement:
			name := el.Name.Local
			parent := ""
			if len(stack) > 1 {
				parent = stack[len(stack)-2]
			}
			s := strings.TrimSpace(text.String())
			switch {
			case name == "ele" && (cur != nil || wpt != nil):
				v, err := strconv.ParseFloat(s, 64)
				if err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v > -500 && v < 9000 {
					if cur != nil {
						cur.Ele = &v
					} else {
						wpt.Ele = &v
					}
				}
			case name == "time" && cur != nil:
				if tm, err := time.Parse(time.RFC3339, s); err == nil && tm.Year() > 1990 && tm.Year() < 2200 {
					cur.Time = &tm
				}
			case name == "name" && wpt != nil:
				wpt.Name = cleanText(s)
			case name == "name" && (parent == "trk" || parent == "rte") && t.Name == "":
				t.Name = cleanText(s)
			case name == "trkpt" || name == "rtept":
				if cur != nil {
					seg = append(seg, *cur)
					cur = nil
				}
			case name == "trkseg" || name == "rte":
				if err := pushSeg(); err != nil {
					return nil, err
				}
			case name == "wpt" && wpt != nil:
				t.Waypoints = append(t.Waypoints, *wpt)
				wpt = nil
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			text.Reset()
		}
	}
	if !rootOK {
		return nil, ErrNotGPX
	}
	if err := pushSeg(); err != nil {
		return nil, err
	}
	if len(t.Segments) == 0 {
		return nil, ErrEmpty
	}
	return t, nil
}

func latLon(attrs []xml.Attr) (float64, float64, error) {
	lat, lon := math.NaN(), math.NaN()
	for _, a := range attrs {
		v, err := strconv.ParseFloat(strings.TrimSpace(a.Value), 64)
		if err != nil {
			return 0, 0, ErrBadPoint
		}
		switch a.Name.Local {
		case "lat":
			lat = v
		case "lon":
			lon = v
		}
	}
	if math.IsNaN(lat) || math.IsNaN(lon) || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0, ErrBadPoint
	}
	return lat, lon, nil
}

// cleanText: без управляющих символов и символов смены направления текста.
func cleanText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) || r == '​' {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > maxText {
		s = string([]rune(s)[:maxText])
	}
	return s
}

// ---------- геометрия ----------

const earthR = 6371008.8 // средний радиус Земли, м

// Distance — расстояние по большому кругу (гаверсинус), м.
func Distance(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthR * math.Asin(math.Min(1, math.Sqrt(a)))
}

// Stats — сводка по маршруту.
type Stats struct {
	DistanceM   float64    `json:"distance_m"`
	GainM       float64    `json:"gain_m"`
	LossM       float64    `json:"loss_m"`
	MinEle      *float64   `json:"min_ele"`
	MaxEle      *float64   `json:"max_ele"`
	DurationSec int64      `json:"duration_sec"` // 0 — в треке нет времени
	MovingSec   int64      `json:"moving_sec"`   // без остановок дольше минуты
	StartedAt   *time.Time `json:"-"`            // наружу не отдаётся: дата похода — личное
	Points      int        `json:"points"`
}

// eleThreshold — шум высоты GPS: колебания меньше этого не считаются подъёмом.
const eleThreshold = 5.0

func (t *Track) Stats() Stats {
	var s Stats
	var first, last *time.Time
	for _, seg := range t.Segments {
		ref := math.NaN() // опорная высота для набора/сброса с гистерезисом
		for i, p := range seg {
			s.Points++
			if i > 0 {
				prev := seg[i-1]
				d := Distance(prev.Lat, prev.Lon, p.Lat, p.Lon)
				s.DistanceM += d
				if prev.Time != nil && p.Time != nil {
					dt := p.Time.Sub(*prev.Time).Seconds()
					if dt > 0 && dt < 60 && d/dt < 50 { // движение, не телепорт
						s.MovingSec += int64(dt)
					}
				}
			}
			if p.Ele != nil {
				e := *p.Ele
				if s.MinEle == nil || e < *s.MinEle {
					v := e
					s.MinEle = &v
				}
				if s.MaxEle == nil || e > *s.MaxEle {
					v := e
					s.MaxEle = &v
				}
				switch {
				case math.IsNaN(ref):
					ref = e
				case e-ref >= eleThreshold:
					s.GainM += e - ref
					ref = e
				case ref-e >= eleThreshold:
					s.LossM += ref - e
					ref = e
				}
			}
			if p.Time != nil {
				if first == nil || p.Time.Before(*first) {
					first = p.Time
				}
				if last == nil || p.Time.After(*last) {
					last = p.Time
				}
			}
		}
	}
	if first != nil && last != nil && last.After(*first) {
		s.DurationSec = int64(last.Sub(*first).Seconds())
		s.StartedAt = first
	}
	s.DistanceM = math.Round(s.DistanceM)
	s.GainM, s.LossM = math.Round(s.GainM), math.Round(s.LossM)
	return s
}

// Trim убирает точки ближе radius метров к началу и к концу всего маршрута,
// а также путевые точки в этих зонах. Сегменты, ставшие пустыми, удаляются.
func (t *Track) Trim(radius float64) {
	if radius <= 0 || len(t.Segments) == 0 {
		return
	}
	firstSeg, lastSeg := t.Segments[0], t.Segments[len(t.Segments)-1]
	start, end := firstSeg[0], lastSeg[len(lastSeg)-1]
	near := func(p Point) bool {
		return Distance(p.Lat, p.Lon, start.Lat, start.Lon) < radius ||
			Distance(p.Lat, p.Lon, end.Lat, end.Lon) < radius
	}
	// начало: пропускаем точки, пока не вышли из зоны старта (дальше — всё, даже если трек вернётся)
	var out [][]Point
	leaving := true
	for _, seg := range t.Segments {
		var keep []Point
		for _, p := range seg {
			if leaving && Distance(p.Lat, p.Lon, start.Lat, start.Lon) < radius {
				continue
			}
			leaving = false
			keep = append(keep, p)
		}
		if len(keep) > 0 {
			out = append(out, keep)
		}
	}
	// конец: с хвоста убираем точки в зоне финиша
	for len(out) > 0 {
		seg := out[len(out)-1]
		i := len(seg)
		for i > 0 && Distance(seg[i-1].Lat, seg[i-1].Lon, end.Lat, end.Lon) < radius {
			i--
		}
		if i > 0 {
			out[len(out)-1] = seg[:i]
			break
		}
		out = out[:len(out)-1]
	}
	t.Segments = out
	var wp []Waypoint
	for _, w := range t.Waypoints {
		if !near(Point{Lat: w.Lat, Lon: w.Lon}) {
			wp = append(wp, w)
		}
	}
	t.Waypoints = wp
}

// StripTimes удаляет метки времени (по ним видно, когда человека не было дома).
func (t *Track) StripTimes() {
	for _, seg := range t.Segments {
		for i := range seg {
			seg[i].Time = nil
		}
	}
}

// ---------- упрощение для отображения ----------

// Simplify возвращает сегменты, упрощённые алгоритмом Дугласа — Пекера так,
// чтобы точек было не больше max (приблизительно; минимум 2 на сегмент).
func (t *Track) Simplify(max int) [][]Point {
	total := 0
	for _, s := range t.Segments {
		total += len(s)
	}
	if total <= max {
		return t.Segments
	}
	// подбираем допуск двоичным поиском: от 0,5 м до 50 км
	lo, hi := 0.5, 50_000.0
	var best [][]Point
	for range 30 {
		mid := math.Sqrt(lo * hi)
		res, n := simplifyAll(t.Segments, mid)
		if n <= max {
			best, hi = res, mid
		} else {
			lo = mid
		}
		if hi/lo < 1.05 {
			break
		}
	}
	if best == nil {
		best, _ = simplifyAll(t.Segments, hi)
	}
	return best
}

func simplifyAll(segs [][]Point, tol float64) ([][]Point, int) {
	out := make([][]Point, 0, len(segs))
	n := 0
	for _, s := range segs {
		r := dp(s, tol)
		out = append(out, r)
		n += len(r)
	}
	return out, n
}

// dp — итеративный Дуглас — Пекер в локальной проекции (метры).
func dp(pts []Point, tol float64) []Point {
	if len(pts) <= 2 {
		return pts
	}
	lat0 := pts[0].Lat * math.Pi / 180
	kx, ky := earthR*math.Cos(lat0)*math.Pi/180, earthR*math.Pi/180
	xy := func(p Point) (float64, float64) { return p.Lon * kx, p.Lat * ky }
	keep := make([]bool, len(pts))
	keep[0], keep[len(pts)-1] = true, true
	type span struct{ a, b int }
	stack := []span{{0, len(pts) - 1}}
	for len(stack) > 0 {
		sp := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		ax, ay := xy(pts[sp.a])
		bx, by := xy(pts[sp.b])
		dx, dy := bx-ax, by-ay
		l2 := dx*dx + dy*dy
		far, idx := -1.0, -1
		for i := sp.a + 1; i < sp.b; i++ {
			px, py := xy(pts[i])
			var d float64
			if l2 == 0 {
				d = math.Hypot(px-ax, py-ay)
			} else {
				u := math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/l2))
				d = math.Hypot(px-(ax+u*dx), py-(ay+u*dy))
			}
			if d > far {
				far, idx = d, i
			}
		}
		if far > tol && idx > 0 {
			keep[idx] = true
			stack = append(stack, span{sp.a, idx}, span{idx, sp.b})
		}
	}
	out := make([]Point, 0, 64)
	for i, k := range keep {
		if k {
			out = append(out, pts[i])
		}
	}
	return out
}

// ProfilePoint — точка профиля высот: пройдено метров, высота.
type ProfilePoint struct {
	D float64 `json:"d"`
	E float64 `json:"e"`
}

// Profile — профиль высот из n точек, равномерно по дистанции. Nil, если высот нет.
func (t *Track) Profile(n int) []ProfilePoint {
	var raw []ProfilePoint
	dist := 0.0
	for _, seg := range t.Segments {
		for i, p := range seg {
			if i > 0 {
				dist += Distance(seg[i-1].Lat, seg[i-1].Lon, p.Lat, p.Lon)
			}
			if p.Ele != nil {
				raw = append(raw, ProfilePoint{D: dist, E: *p.Ele})
			}
		}
	}
	if len(raw) < 2 || n < 2 {
		return nil
	}
	if len(raw) <= n {
		for i := range raw {
			raw[i].D, raw[i].E = math.Round(raw[i].D), math.Round(raw[i].E*10)/10
		}
		return raw
	}
	total := raw[len(raw)-1].D
	out := make([]ProfilePoint, 0, n)
	j := 0
	for k := range n {
		target := total * float64(k) / float64(n-1)
		for j < len(raw)-1 && raw[j+1].D < target {
			j++
		}
		// среднее по окну вместо одной точки — меньше шума GPS
		lo, hi := j, j
		for lo > 0 && raw[lo-1].D > target-total/float64(2*n) {
			lo--
		}
		for hi < len(raw)-1 && raw[hi+1].D < target+total/float64(2*n) {
			hi++
		}
		sum := 0.0
		for i := lo; i <= hi; i++ {
			sum += raw[i].E
		}
		out = append(out, ProfilePoint{D: math.Round(target), E: math.Round(sum/float64(hi-lo+1)*10) / 10})
	}
	return out
}

// ---------- вывод ----------

// WriteGPX пишет чистый GPX 1.1: только геометрия, высоты, время (если не
// удалено), название трека и путевых точек.
func (t *Track) WriteGPX(w io.Writer) error {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<gpx version="1.1" creator="socialnet" xmlns="http://www.topografix.com/GPX/1/1">` + "\n")
	esc := func(s string) string {
		var sb strings.Builder
		_ = xml.EscapeText(&sb, []byte(s))
		return sb.String()
	}
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 7, 64) }
	for _, wp := range t.Waypoints {
		fmt.Fprintf(&b, `<wpt lat="%s" lon="%s">`, f(wp.Lat), f(wp.Lon))
		if wp.Ele != nil {
			fmt.Fprintf(&b, "<ele>%s</ele>", strconv.FormatFloat(*wp.Ele, 'f', 1, 64))
		}
		if wp.Name != "" {
			fmt.Fprintf(&b, "<name>%s</name>", esc(wp.Name))
		}
		b.WriteString("</wpt>\n")
	}
	b.WriteString("<trk>")
	if t.Name != "" {
		fmt.Fprintf(&b, "<name>%s</name>", esc(t.Name))
	}
	b.WriteString("\n")
	for _, seg := range t.Segments {
		b.WriteString("<trkseg>\n")
		for _, p := range seg {
			fmt.Fprintf(&b, `<trkpt lat="%s" lon="%s">`, f(p.Lat), f(p.Lon))
			if p.Ele != nil {
				fmt.Fprintf(&b, "<ele>%s</ele>", strconv.FormatFloat(*p.Ele, 'f', 1, 64))
			}
			if p.Time != nil {
				fmt.Fprintf(&b, "<time>%s</time>", p.Time.UTC().Format(time.RFC3339))
			}
			b.WriteString("</trkpt>\n")
		}
		b.WriteString("</trkseg>\n")
	}
	b.WriteString("</trk>\n</gpx>\n")
	_, err := io.WriteString(w, b.String())
	return err
}
