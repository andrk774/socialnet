package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const meteoJSON = `{"latitude":55.15,"longitude":59.7,"elevation":1150,"daily":{
 "time":["2026-10-27","2026-10-28"],"weather_code":[3,95],
 "temperature_2m_max":[2.5,-1.0],"temperature_2m_min":[-4.1,-7.3],
 "precipitation_sum":[0.0,12.4],"precipitation_probability_max":[10,90],
 "wind_speed_10m_max":[4.2,11.0],"wind_gusts_10m_max":[9.0,21.5]}}`

func TestWeather(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	anna := register(t, ts, "anna")
	var hits atomic.Int32
	var mode atomic.Value
	mode.Store("ok")
	var lastQuery atomic.Value
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		lastQuery.Store(r.URL.RawQuery)
		time.Sleep(30 * time.Millisecond)
		if mode.Load() == "down" {
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte(meteoJSON))
	}))
	defer up.Close()

	srv.cfg.WeatherURL = ""
	anna.req("GET", p("/weather?lat=55.1&lon=59.7"), nil, 404)
	srv.cfg.WeatherURL = up.URL + "/v1/forecast?apikey=secret"
	srv.weather = newWeatherCache()
	srv.weatherLimiter = newIPRateLimiter(1000, 1000)

	for _, bad := range []string{"", "?lat=55", "?lat=95&lon=60", "?lat=55&lon=60&elevation=20000", "?lat=NaN&lon=60"} {
		anna.req("GET", p("/weather%s", bad), nil, 400)
	}

	// десять человек смотрят один район — к источнику один запрос
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, _ := anna.do("GET", p("/weather?lat=55.151&lon=59.702&elevation=1163"), anna.Token, nil); code != 200 {
				t.Errorf("weather: %d", code)
			}
		}()
	}
	wg.Wait()
	rep := anna.req("GET", p("/weather?lat=55.149&lon=59.699&elevation=1140"), nil, 200) // тот же район и высота ±25 м
	if n := hits.Load(); n != 1 {
		t.Fatalf("source hit %d times", n)
	}
	q := lastQuery.Load().(string)
	if !strings.Contains(q, "apikey=secret") || !strings.Contains(q, "latitude=55.15") || !strings.Contains(q, "elevation=1150") ||
		!strings.Contains(q, "wind_speed_unit=ms") {
		t.Errorf("query to source: %s", q)
	}
	days := rep["days"].([]any)
	d2 := days[1].(map[string]any)
	if len(days) != 2 || d2["code"].(float64) != 95 || d2["gust_max"].(float64) != 21.5 || rep["source"] != "Open-Meteo.com" {
		t.Fatalf("report: %v", rep)
	}

	// источник лёг: новый район — 502, известный — прошлый прогноз
	mode.Store("down")
	anna.req("GET", p("/weather?lat=54.5&lon=59.0"), nil, 502)
	srv.weather.mu.Lock()
	for _, e := range srv.weather.items {
		e.at = time.Now().Add(-2 * time.Hour)
	}
	srv.weather.mu.Unlock()
	anna.req("GET", p("/weather?lat=55.15&lon=59.7&elevation=1150"), nil, 200)

	// настройки клиента знают, что прогноз включён
	if cfg := anna.req("GET", p("/client-config"), nil, 200); cfg["weather"].(map[string]any)["enabled"] != true {
		t.Fatalf("client-config: %v", cfg)
	}
}
