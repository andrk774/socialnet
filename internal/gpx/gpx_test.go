package gpx

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// track строит GPX из точек (lat, lon, ele) с шагом времени 10 с.
func track(pts [][3]float64, extra string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><gpx version="1.1" creator="Garmin" xmlns="http://www.topografix.com/GPX/1/1">`)
	b.WriteString(`<metadata><author><name>Иван</name><email id="ivan" domain="mail.ru"/></author></metadata>`)
	b.WriteString(extra)
	b.WriteString(`<trk><name>Таганай, Откликной гребень</name><trkseg>`)
	t0 := time.Date(2026, 7, 4, 6, 0, 0, 0, time.UTC)
	for i, p := range pts {
		fmt.Fprintf(&b, `<trkpt lat="%f" lon="%f"><ele>%f</ele><time>%s</time></trkpt>`,
			p[0], p[1], p[2], t0.Add(time.Duration(i)*10*time.Second).Format(time.RFC3339))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	return b.String()
}

// line — прямая на север от Миасса: n точек через step метров, высота по функции.
func line(n int, step float64, ele func(i int) float64) [][3]float64 {
	out := make([][3]float64, n)
	for i := range out {
		out[i] = [3]float64{55.05 + float64(i)*step/111195.0, 60.1, ele(i)}
	}
	return out
}

func TestDistance(t *testing.T) {
	// Миасс — Челябинск по прямой ≈ 90 км
	d := Distance(55.045, 60.108, 55.160, 61.403)
	if d < 82_000 || d > 84_500 {
		t.Fatalf("Miass–Chelyabinsk: %.0f m", d)
	}
	if Distance(10, 20, 10, 20) != 0 {
		t.Fatal("zero distance")
	}
	// градус широты ≈ 111,2 км
	if d := Distance(0, 0, 1, 0); math.Abs(d-111195) > 50 {
		t.Fatalf("1° lat = %.0f", d)
	}
}

func TestParseAndStats(t *testing.T) {
	// 101 точка через 10 м = 1000 м; подъём на 200 м с дрожанием GPS ±2 м
	pts := line(101, 10, func(i int) float64 { return 400 + float64(i)*2 + float64(i%2)*2 - 1 })
	tr, err := Parse(strings.NewReader(track(pts, `<wpt lat="55.051" lon="60.1"><ele>410</ele><name>Родник&#x202E;</name></wpt>`)))
	if err != nil {
		t.Fatal(err)
	}
	if tr.Name != "Таганай, Откликной гребень" || len(tr.Segments) != 1 || len(tr.Segments[0]) != 101 {
		t.Fatalf("parsed: %q %d", tr.Name, len(tr.Segments))
	}
	if len(tr.Waypoints) != 1 || tr.Waypoints[0].Name != "Родник" {
		t.Fatalf("waypoint (bidi control must be stripped): %+v", tr.Waypoints)
	}
	s := tr.Stats()
	if math.Abs(s.DistanceM-1000) > 2 {
		t.Errorf("distance %.0f", s.DistanceM)
	}
	// дрожание ±2 м не должно накручивать набор: честный подъём ≈ 200 м
	if s.GainM < 190 || s.GainM > 210 || s.LossM > 10 {
		t.Errorf("gain %.0f loss %.0f", s.GainM, s.LossM)
	}
	if s.DurationSec != 1000 || s.MovingSec != 1000 || s.Points != 101 {
		t.Errorf("time: %+v", s)
	}
	if *s.MinEle != 399 || *s.MaxEle != 599 {
		t.Errorf("ele: %v %v", *s.MinEle, *s.MaxEle)
	}
}

func TestRoute(t *testing.T) {
	g := `<gpx version="1.1"><rte><name>План</name><rtept lat="55" lon="60"/><rtept lat="55.01" lon="60"/></rte></gpx>`
	tr, err := Parse(strings.NewReader(g))
	if err != nil || len(tr.Segments) != 1 || len(tr.Segments[0]) != 2 || tr.Name != "План" {
		t.Fatalf("rte: %v %+v", err, tr)
	}
	if s := tr.Stats(); s.DurationSec != 0 || s.MinEle != nil || tr.Profile(10) != nil {
		t.Fatalf("no time/ele: %+v", s)
	}
}

func TestMaliciousInput(t *testing.T) {
	cases := map[string]struct {
		in   string
		want error
	}{
		"xxe":               {`<?xml version="1.0"?><!DOCTYPE gpx [<!ENTITY x SYSTEM "file:///etc/passwd">]><gpx><trk><name>&x;</name></trk></gpx>`, ErrDTD},
		"billion laughs":    {`<?xml version="1.0"?><!DOCTYPE l [<!ENTITY a "aaaa"><!ENTITY b "&a;&a;&a;&a;">]><gpx>&b;</gpx>`, ErrDTD},
		"undeclared entity": {`<gpx><trk><name>&nbsp;</name><trkseg><trkpt lat="1" lon="1"/></trkseg></trk></gpx>`, ErrDTD},
		"not gpx":           {`<kml><Document/></kml>`, ErrNotGPX},
		"html":              {`<!doctype html><html><script>alert(1)</script></html>`, ErrDTD},
		"garbage":           {"\x00\x01PK\x03\x04", ErrNotGPX},
		"empty":             {`<gpx></gpx>`, ErrEmpty},
		"lat range":         {`<gpx><trk><trkseg><trkpt lat="91" lon="0"/></trkseg></trk></gpx>`, ErrBadPoint},
		"nan":               {`<gpx><trk><trkseg><trkpt lat="NaN" lon="0"/></trkseg></trk></gpx>`, ErrBadPoint},
		"inf":               {`<gpx><trk><trkseg><trkpt lat="1" lon="Inf"/></trkseg></trk></gpx>`, ErrBadPoint},
		"no lon":            {`<gpx><trk><trkseg><trkpt lat="1"/></trkseg></trk></gpx>`, ErrBadPoint},
		"cp1251":            {`<?xml version="1.0" encoding="windows-1251"?><gpx/>`, ErrEncoding},
		"deep":              {`<gpx>` + strings.Repeat("<a>", 100) + strings.Repeat("</a>", 100) + `</gpx>`, ErrNotGPX},
	}
	for name, c := range cases {
		_, err := Parse(strings.NewReader(c.in))
		if !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", name, err, c.want)
		}
	}
	// слишком много точек
	var b strings.Builder
	b.WriteString("<gpx><trk><trkseg>")
	for range MaxPoints + 1 {
		b.WriteString(`<trkpt lat="1" lon="1"/>`)
	}
	b.WriteString("</trkseg></trk></gpx>")
	if _, err := Parse(strings.NewReader(b.String())); !errors.Is(err, ErrTooBig) {
		t.Errorf("points limit: %v", err)
	}
	// слишком много сегментов
	b.Reset()
	b.WriteString("<gpx><trk>")
	for range MaxSegments + 1 {
		b.WriteString(`<trkseg><trkpt lat="1" lon="1"/></trkseg>`)
	}
	b.WriteString("</trk></gpx>")
	if _, err := Parse(strings.NewReader(b.String())); !errors.Is(err, ErrTooBig) {
		t.Errorf("segments limit: %v", err)
	}
	// абсурдная высота и время игнорируются, а не ломают статистику
	tr, err := Parse(strings.NewReader(`<gpx><trk><trkseg><trkpt lat="1" lon="1"><ele>1e308</ele><time>0001-01-01T00:00:00Z</time></trkpt><trkpt lat="1.001" lon="1"/></trkseg></trk></gpx>`))
	if err != nil || tr.Segments[0][0].Ele != nil || tr.Segments[0][0].Time != nil {
		t.Errorf("absurd values kept: %v", err)
	}
}

func TestTrimPrivacy(t *testing.T) {
	// 2 км на север и обратно: старт и финиш у «дома»
	there := line(201, 10, func(int) float64 { return 300 })
	pts := append(there, reverse(there)[1:]...)
	tr, err := Parse(strings.NewReader(track(pts, `<wpt lat="55.0501" lon="60.1"><name>Дом</name></wpt><wpt lat="55.06" lon="60.1"><name>Вершина</name></wpt>`)))
	if err != nil {
		t.Fatal(err)
	}
	home := tr.Segments[0][0]
	tr.Trim(300)
	for _, seg := range tr.Segments {
		for _, p := range seg {
			if d := Distance(p.Lat, p.Lon, home.Lat, home.Lon); d < 300 {
				t.Fatalf("point %.0f m from home survived trim", d)
			}
		}
	}
	if len(tr.Waypoints) != 1 || tr.Waypoints[0].Name != "Вершина" {
		t.Fatalf("waypoints after trim: %+v", tr.Waypoints)
	}
	tr.StripTimes()
	var out bytes.Buffer
	if err := tr.WriteGPX(&out); err != nil {
		t.Fatal(err)
	}
	g := out.String()
	for _, leak := range []string{"<time>", "Иван", "mail.ru", "Garmin", "Дом"} {
		if strings.Contains(g, leak) {
			t.Errorf("output GPX leaks %q", leak)
		}
	}
	// результат снова читается и сохраняет геометрию
	again, err := Parse(strings.NewReader(g))
	if err != nil || again.Stats().Points != tr.Stats().Points || again.Name != tr.Name {
		t.Fatalf("round trip: %v", err)
	}
	// трек целиком в зоне — ничего не остаётся
	short := &Track{Segments: [][]Point{{{Lat: 1, Lon: 1}, {Lat: 1.0001, Lon: 1}}}}
	short.Trim(300)
	if len(short.Segments) != 0 {
		t.Fatalf("short track kept: %+v", short.Segments)
	}
}

func reverse(p [][3]float64) [][3]float64 {
	out := make([][3]float64, len(p))
	for i := range p {
		out[i] = p[len(p)-1-i]
	}
	return out
}

func TestSimplifyAndProfile(t *testing.T) {
	// зигзаг на 5000 точек
	pts := make([][3]float64, 5000)
	for i := range pts {
		pts[i] = [3]float64{55 + float64(i)*0.0001, 60 + math.Sin(float64(i)/40)*0.002, 300 + 100*math.Sin(float64(i)/500)}
	}
	tr, err := Parse(strings.NewReader(track(pts, "")))
	if err != nil {
		t.Fatal(err)
	}
	for _, max := range []int{2000, 300, 50} {
		s := tr.Simplify(max)
		n := 0
		for _, seg := range s {
			n += len(seg)
		}
		if n > max || n < max/4 {
			t.Errorf("simplify to %d: got %d points", max, n)
		}
		if s[0][0] != tr.Segments[0][0] || s[0][len(s[0])-1] != tr.Segments[0][len(tr.Segments[0])-1] {
			t.Error("simplify must keep start and end")
		}
	}
	prof := tr.Profile(200)
	if len(prof) != 200 || prof[0].D != 0 || math.Abs(prof[199].D-tr.Stats().DistanceM) > 1 {
		t.Fatalf("profile: len=%d first=%v last=%v dist=%v", len(prof), prof[0], prof[len(prof)-1], tr.Stats().DistanceM)
	}
	for i := 1; i < len(prof); i++ {
		if prof[i].D < prof[i-1].D {
			t.Fatal("profile distance must grow")
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add(track(line(5, 10, func(i int) float64 { return float64(i) }), ""))
	f.Add(`<gpx><rte><rtept lat="1" lon="2"><ele>3</ele></rtept></rte><wpt lat="0" lon="0"><name>x</name></wpt></gpx>`)
	f.Add(`<?xml version="1.0"?><!DOCTYPE x [<!ENTITY a "b">]><gpx>&a;</gpx>`)
	f.Fuzz(func(t *testing.T, s string) {
		tr, err := Parse(strings.NewReader(s))
		if err != nil {
			return
		}
		st := tr.Stats()
		if math.IsNaN(st.DistanceM) || st.DistanceM < 0 || st.GainM < 0 || st.LossM < 0 {
			t.Fatalf("bad stats %+v", st)
		}
		tr.Trim(200)
		_ = tr.Simplify(100)
		_ = tr.Profile(50)
		var b bytes.Buffer
		if err := tr.WriteGPX(&b); err != nil {
			t.Fatal(err)
		}
		if len(tr.Segments) > 0 {
			if _, err := Parse(&b); err != nil {
				t.Fatalf("own output does not parse: %v\n%s", err, b.String())
			}
		}
	})
}
