// Package metrics — счётчики, гистограммы и датчики в текстовом формате
// Prometheus (exposition format 0.0.4). Без внешних зависимостей.
package metrics

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type Registry struct {
	mu      sync.Mutex
	metrics []metric
}

type metric interface {
	name() string
	write(w io.Writer)
}

func NewRegistry() *Registry { return &Registry{} }

func (r *Registry) add(m metric) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.metrics {
		if x.name() == m.name() {
			panic("metric registered twice: " + m.name())
		}
	}
	r.metrics = append(r.metrics, m)
}

// WriteText пишет все метрики, отсортированные по имени.
func (r *Registry) WriteText(w io.Writer) {
	r.mu.Lock()
	ms := append([]metric(nil), r.metrics...)
	r.mu.Unlock()
	sort.Slice(ms, func(i, j int) bool { return ms[i].name() < ms[j].name() })
	for _, m := range ms {
		m.write(w)
	}
}

func escapeLabel(v string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`).Replace(v)
}

func labelString(names, values []string, extra ...string) string {
	var parts []string
	for i, n := range names {
		parts = append(parts, n+`="`+escapeLabel(values[i])+`"`)
	}
	for i := 0; i+1 < len(extra); i += 2 {
		parts = append(parts, extra[i]+`="`+escapeLabel(extra[i+1])+`"`)
	}
	if len(parts) == 0 {
		return ""
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func formatFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "+Inf"
	case math.IsInf(f, -1):
		return "-Inf"
	case math.IsNaN(f):
		return "NaN"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// ---------- счётчик ----------

type CounterVec struct {
	n, help string
	labels  []string
	mu      sync.Mutex
	values  map[string]*atomic.Uint64
	keys    map[string][]string
}

func (r *Registry) Counter(name, help string, labels ...string) *CounterVec {
	c := &CounterVec{n: name, help: help, labels: labels, values: map[string]*atomic.Uint64{}, keys: map[string][]string{}}
	r.add(c)
	return c
}

func (c *CounterVec) name() string { return c.n }

func (c *CounterVec) Inc(labelValues ...string) { c.Add(1, labelValues...) }

func (c *CounterVec) Add(n uint64, labelValues ...string) {
	if len(labelValues) != len(c.labels) {
		panic("wrong label count for " + c.n)
	}
	key := strings.Join(labelValues, "\x00")
	c.mu.Lock()
	v, ok := c.values[key]
	if !ok {
		v = &atomic.Uint64{}
		c.values[key] = v
		c.keys[key] = append([]string(nil), labelValues...)
	}
	c.mu.Unlock()
	v.Add(n)
}

func (c *CounterVec) Value(labelValues ...string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.values[strings.Join(labelValues, "\x00")]; ok {
		return v.Load()
	}
	return 0
}

func (c *CounterVec) write(w io.Writer) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", c.n, c.help, c.n)
	c.mu.Lock()
	keys := make([]string, 0, len(c.values))
	for k := range c.values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(w, "%s%s %d\n", c.n, labelString(c.labels, c.keys[k]), c.values[k].Load())
	}
	c.mu.Unlock()
}

// ---------- гистограмма ----------

var DefaultBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

type histo struct {
	counts []uint64 // по корзинам, не накопительно
	sum    float64
	count  uint64
}

type HistogramVec struct {
	n, help string
	labels  []string
	buckets []float64
	mu      sync.Mutex
	values  map[string]*histo
	keys    map[string][]string
}

func (r *Registry) Histogram(name, help string, buckets []float64, labels ...string) *HistogramVec {
	h := &HistogramVec{n: name, help: help, labels: labels, buckets: buckets, values: map[string]*histo{}, keys: map[string][]string{}}
	r.add(h)
	return h
}

func (h *HistogramVec) name() string { return h.n }

func (h *HistogramVec) Observe(v float64, labelValues ...string) {
	key := strings.Join(labelValues, "\x00")
	h.mu.Lock()
	defer h.mu.Unlock()
	x, ok := h.values[key]
	if !ok {
		x = &histo{counts: make([]uint64, len(h.buckets))}
		h.values[key] = x
		h.keys[key] = append([]string(nil), labelValues...)
	}
	for i, b := range h.buckets {
		if v <= b {
			x.counts[i]++
			break
		}
	}
	x.sum += v
	x.count++
}

func (h *HistogramVec) write(w io.Writer) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s histogram\n", h.n, h.help, h.n)
	h.mu.Lock()
	defer h.mu.Unlock()
	keys := make([]string, 0, len(h.values))
	for k := range h.values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		x, lv := h.values[k], h.keys[k]
		var cum uint64
		for i, b := range h.buckets {
			cum += x.counts[i]
			_, _ = fmt.Fprintf(w, "%s_bucket%s %d\n", h.n, labelString(h.labels, lv, "le", formatFloat(b)), cum)
		}
		_, _ = fmt.Fprintf(w, "%s_bucket%s %d\n", h.n, labelString(h.labels, lv, "le", "+Inf"), x.count)
		_, _ = fmt.Fprintf(w, "%s_sum%s %s\n", h.n, labelString(h.labels, lv), formatFloat(x.sum))
		_, _ = fmt.Fprintf(w, "%s_count%s %d\n", h.n, labelString(h.labels, lv), x.count)
	}
}

// ---------- датчик, значение считается при сборе ----------

type GaugeFunc struct {
	n, help string
	labels  []string
	fn      func() map[string]float64 // значение метки (первой) → число; "" — без меток
}

// Gauge: fn вызывается при каждом сборе метрик.
func (r *Registry) Gauge(name, help string, fn func() float64) {
	r.add(&GaugeFunc{n: name, help: help, fn: func() map[string]float64 { return map[string]float64{"": fn()} }})
}

// GaugeVec: одна метка, значения возвращает fn.
func (r *Registry) GaugeVec(name, help, label string, fn func() map[string]float64) {
	r.add(&GaugeFunc{n: name, help: help, labels: []string{label}, fn: fn})
}

func (g *GaugeFunc) name() string { return g.n }

func (g *GaugeFunc) write(w io.Writer) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n", g.n, g.help, g.n)
	vals := g.fn()
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if len(g.labels) == 0 {
			_, _ = fmt.Fprintf(w, "%s %s\n", g.n, formatFloat(vals[k]))
		} else {
			_, _ = fmt.Fprintf(w, "%s%s %s\n", g.n, labelString(g.labels, []string{k}), formatFloat(vals[k]))
		}
	}
}
