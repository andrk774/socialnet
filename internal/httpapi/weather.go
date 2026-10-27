package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Прогноз погоды для маршрута через Open-Meteo. Запрос идёт с сервера:
// браузер не раскрывает сторонним сервисам ни IP, ни то, куда человек идёт.
// Координаты округляются до ~5 км, высота — до 50 м, ответ кешируется на
// час: для одного района все смотрят один прогноз, а к источнику уходит
// мало запросов (бесплатный тариф Open-Meteo — только некоммерческий,
// для коммерческого проекта нужен их платный ключ, см. WEATHER_URL).

const (
	weatherTTL      = time.Hour
	weatherMaxCache = 2000
)

type weatherDay struct {
	Date       string   `json:"date"`
	Code       int      `json:"code"` // WMO
	TMax       *float64 `json:"t_max"`
	TMin       *float64 `json:"t_min"`
	PrecipMM   *float64 `json:"precip_mm"`
	PrecipProb *float64 `json:"precip_prob"`
	WindMax    *float64 `json:"wind_max"` // м/с
	GustMax    *float64 `json:"gust_max"`
}

type weatherReport struct {
	Lat       float64      `json:"lat"`
	Lon       float64      `json:"lon"`
	Elevation *float64     `json:"elevation"`
	Days      []weatherDay `json:"days"`
	Source    string       `json:"source"`
	FetchedAt time.Time    `json:"fetched_at"`
}

type weatherEntry struct {
	rep  *weatherReport
	at   time.Time
	done chan struct{}
	err  error
}

type weatherCache struct {
	client *http.Client
	mu     sync.Mutex
	items  map[string]*weatherEntry
}

func newWeatherCache() *weatherCache {
	return &weatherCache{client: &http.Client{Timeout: 10 * time.Second}, items: map[string]*weatherEntry{}}
}

func roundTo(v, step float64) float64 { return math.Round(v/step) * step }

func (c *weatherCache) get(ctx context.Context, base, ua string, lat, lon float64, ele *float64) (*weatherReport, error) {
	key := fmt.Sprintf("%.2f,%.2f", lat, lon)
	if ele != nil {
		key += fmt.Sprintf(",%.0f", *ele)
	}
	c.mu.Lock()
	e, ok := c.items[key]
	if ok && e.rep != nil && time.Since(e.at) < weatherTTL {
		c.mu.Unlock()
		return e.rep, nil
	}
	if !ok || e.rep != nil || e.err != nil { // нет, устарело или прошлый запрос не удался
		if len(c.items) >= weatherMaxCache {
			for k, v := range c.items { // грубая очистка: выкинуть устаревшие
				if v.rep != nil && time.Since(v.at) >= weatherTTL || v.err != nil {
					delete(c.items, k)
				}
			}
			if len(c.items) >= weatherMaxCache {
				c.items = map[string]*weatherEntry{}
			}
		}
		stale := e
		e = &weatherEntry{done: make(chan struct{})}
		c.items[key] = e
		c.mu.Unlock()
		go func() {
			fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			defer cancel()
			rep, err := c.fetch(fctx, base, ua, lat, lon, ele)
			c.mu.Lock()
			if err != nil && stale != nil && stale.rep != nil {
				e.rep, e.at = stale.rep, stale.at // источник лёг — прошлый прогноз лучше пустоты
			} else {
				e.rep, e.err, e.at = rep, err, time.Now()
			}
			c.mu.Unlock()
			close(e.done)
		}()
	} else {
		c.mu.Unlock()
	}
	select {
	case <-e.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return e.rep, e.err
}

func (c *weatherCache) fetch(ctx context.Context, base, ua string, lat, lon float64, ele *float64) (*weatherReport, error) {
	q := url.Values{
		"latitude":  {strconv.FormatFloat(lat, 'f', 2, 64)},
		"longitude": {strconv.FormatFloat(lon, 'f', 2, 64)},
		"daily": {"weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum," +
			"precipitation_probability_max,wind_speed_10m_max,wind_gusts_10m_max"},
		"wind_speed_unit": {"ms"},
		"timezone":        {"Asia/Yekaterinburg"},
		"forecast_days":   {"10"},
	}
	if ele != nil {
		q.Set("elevation", strconv.FormatFloat(*ele, 'f', 0, 64))
	}
	sep := "?"
	if strings.Contains(base, "?") { // платный адрес с ?apikey=…
		sep = "&"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+sep+q.Encode(), nil) // #nosec G704 -- адрес из WEATHER_URL администратора, параметры — числа
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	resp, err := c.client.Do(req) // #nosec G704 -- см. выше
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather source: HTTP %d", resp.StatusCode)
	}
	var raw struct {
		Elevation *float64 `json:"elevation"`
		Daily     struct {
			Time       []string   `json:"time"`
			Code       []*int     `json:"weather_code"`
			TMax       []*float64 `json:"temperature_2m_max"`
			TMin       []*float64 `json:"temperature_2m_min"`
			Precip     []*float64 `json:"precipitation_sum"`
			PrecipProb []*float64 `json:"precipitation_probability_max"`
			Wind       []*float64 `json:"wind_speed_10m_max"`
			Gust       []*float64 `json:"wind_gusts_10m_max"`
		} `json:"daily"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
		return nil, err
	}
	d := raw.Daily
	if len(d.Time) == 0 || len(d.Time) > 16 {
		return nil, errors.New("weather source: unexpected response")
	}
	at := func(v []*float64, i int) *float64 {
		if i < len(v) {
			return v[i]
		}
		return nil
	}
	rep := &weatherReport{Lat: lat, Lon: lon, Elevation: raw.Elevation, Source: "Open-Meteo.com", FetchedAt: time.Now().UTC()}
	for i, day := range d.Time {
		if _, err := time.Parse("2006-01-02", day); err != nil {
			return nil, errors.New("weather source: bad date")
		}
		wd := weatherDay{Date: day, TMax: at(d.TMax, i), TMin: at(d.TMin, i), PrecipMM: at(d.Precip, i),
			PrecipProb: at(d.PrecipProb, i), WindMax: at(d.Wind, i), GustMax: at(d.Gust, i)}
		if i < len(d.Code) && d.Code[i] != nil {
			wd.Code = *d.Code[i]
		}
		rep.Days = append(rep.Days, wd)
	}
	return rep, nil
}

// handleWeather: GET /api/v1/weather?lat=&lon=&elevation= — прогноз на 10 дней.
func (s *Server) handleWeather(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WeatherURL == "" || s.weather == nil {
		writeError(w, http.StatusNotFound, "weather_disabled", "weather is not configured")
		return
	}
	q := r.URL.Query()
	lat, err1 := strconv.ParseFloat(q.Get("lat"), 64)
	lon, err2 := strconv.ParseFloat(q.Get("lon"), 64)
	if err1 != nil || err2 != nil || math.IsNaN(lat) || math.IsNaN(lon) || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		writeError(w, http.StatusBadRequest, "bad_request", "lat and lon are required")
		return
	}
	var ele *float64
	if v := q.Get("elevation"); v != "" {
		e, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(e) || e < -500 || e > 9000 {
			writeError(w, http.StatusBadRequest, "bad_request", "elevation must be -500..9000 m")
			return
		}
		e = roundTo(e, 50)
		ele = &e
	}
	if !s.weatherLimiter.allow(strconv.FormatInt(userIDFrom(r.Context()), 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many weather requests")
		return
	}
	rep, err := s.weather.get(r.Context(), s.cfg.WeatherURL, s.tileUserAgent(), roundTo(lat, 0.05), roundTo(lon, 0.05), ele)
	if err != nil {
		if r.Context().Err() == nil {
			s.log.Warn("weather", "err", err)
		}
		writeError(w, http.StatusBadGateway, "weather_unavailable", "the weather service is unavailable, try later")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=600")
	writeJSON(w, http.StatusOK, rep)
}
