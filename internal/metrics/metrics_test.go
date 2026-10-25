package metrics

import (
	"strings"
	"sync"
	"testing"
)

func TestExpositionFormat(t *testing.T) {
	r := NewRegistry()
	c := r.Counter("http_requests_total", "Requests.", "method", "status")
	h := r.Histogram("latency_seconds", "Latency.", []float64{0.1, 1}, "route")
	r.Gauge("ws_connections", "Open sockets.", func() float64 { return 3 })
	r.GaugeVec("queue_depth", "Queue.", "queue", func() map[string]float64 { return map[string]float64{"jobs": 2, "callbacks": 0} })

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.Inc("GET", "200") }()
	}
	wg.Wait()
	c.Inc("POST", "500")
	h.Observe(0.05, "/a")
	h.Observe(0.5, "/a")
	h.Observe(5, "/a")
	c.Inc(`we"ird`, "x\ny")

	var b strings.Builder
	r.WriteText(&b)
	out := b.String()
	for _, want := range []string{
		"# TYPE http_requests_total counter\n",
		`http_requests_total{method="GET",status="200"} 100`,
		`http_requests_total{method="we\"ird",status="x\ny"} 1`,
		`latency_seconds_bucket{route="/a",le="0.1"} 1`,
		`latency_seconds_bucket{route="/a",le="1"} 2`,
		`latency_seconds_bucket{route="/a",le="+Inf"} 3`,
		`latency_seconds_sum{route="/a"} 5.55`,
		`latency_seconds_count{route="/a"} 3`,
		"ws_connections 3\n",
		`queue_depth{queue="jobs"} 2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if c.Value("GET", "200") != 100 {
		t.Fatal("value")
	}
}
