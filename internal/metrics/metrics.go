// Package metrics writes metrics in the Prometheus text format (version
// 0.0.4): counters and histograms kept in memory, and values gathered when
// they are scraped. It is small on purpose – the client library would bring
// a dozen dependencies for what fits here.
package metrics

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Registry holds the metrics of the server.
type Registry struct {
	mu         sync.Mutex
	counters   []*Counter
	histograms []*Histogram
	collectors []func(*Writer)
}

// New returns an empty registry.
func New() *Registry { return &Registry{} }

// series is one combination of label values.
type series struct {
	values []string
	n      float64
	// histograms only
	buckets []uint64
	sum     float64
}

// Counter counts events, by labels.
type Counter struct {
	name, help string
	labels     []string
	mu         sync.Mutex
	series     map[string]*series
}

// Counter registers a counter.
func (r *Registry) Counter(name, help string, labels ...string) *Counter {
	c := &Counter{name: name, help: help, labels: labels, series: map[string]*series{}}
	r.mu.Lock()
	r.counters = append(r.counters, c)
	r.mu.Unlock()
	return c
}

func key(values []string) string { return strings.Join(values, "\x00") }

// Inc adds one for the label values (in the order of the labels).
func (c *Counter) Inc(values ...string) { c.Add(1, values...) }

// Add adds v for the label values.
func (c *Counter) Add(v float64, values ...string) {
	if len(values) != len(c.labels) {
		panic(fmt.Sprintf("metrics: %s takes %d labels, got %d", c.name, len(c.labels), len(values)))
	}
	k := key(values)
	c.mu.Lock()
	s := c.series[k]
	if s == nil {
		s = &series{values: slices.Clone(values)}
		c.series[k] = s
	}
	s.n += v
	c.mu.Unlock()
}

// Histogram counts observations (such as durations) into buckets.
type Histogram struct {
	name, help string
	labels     []string
	bounds     []float64
	mu         sync.Mutex
	series     map[string]*series
}

// Histogram registers a histogram with the upper bounds of its buckets.
func (r *Registry) Histogram(name, help string, bounds []float64, labels ...string) *Histogram {
	h := &Histogram{name: name, help: help, labels: labels, bounds: slices.Sorted(slices.Values(bounds)), series: map[string]*series{}}
	r.mu.Lock()
	r.histograms = append(r.histograms, h)
	r.mu.Unlock()
	return h
}

// Observe records v for the label values.
func (h *Histogram) Observe(v float64, values ...string) {
	if len(values) != len(h.labels) {
		panic(fmt.Sprintf("metrics: %s takes %d labels, got %d", h.name, len(h.labels), len(values)))
	}
	k := key(values)
	h.mu.Lock()
	s := h.series[k]
	if s == nil {
		s = &series{values: slices.Clone(values), buckets: make([]uint64, len(h.bounds))}
		h.series[k] = s
	}
	for i, b := range h.bounds {
		if v <= b {
			s.buckets[i]++
		}
	}
	s.n++
	s.sum += v
	h.mu.Unlock()
}

// Collect adds a function that writes values gathered at scrape time.
func (r *Registry) Collect(fn func(*Writer)) {
	r.mu.Lock()
	r.collectors = append(r.collectors, fn)
	r.mu.Unlock()
}

// Writer writes metric families and samples.
type Writer struct {
	b *bytes.Buffer
}

// Family starts a metric family (written once, before its samples).
func (w *Writer) Family(name, typ, help string) {
	fmt.Fprintf(w.b, "# HELP %s %s\n# TYPE %s %s\n", name, escapeHelp(help), name, typ)
}

// Sample writes one value; labels are name, value pairs.
func (w *Writer) Sample(name string, v float64, labels ...string) {
	w.b.WriteString(name)
	if len(labels) > 0 {
		w.b.WriteByte('{')
		for i := 0; i+1 < len(labels); i += 2 {
			if i > 0 {
				w.b.WriteByte(',')
			}
			w.b.WriteString(labels[i])
			w.b.WriteString(`="`)
			w.b.WriteString(escapeValue(labels[i+1]))
			w.b.WriteByte('"')
		}
		w.b.WriteByte('}')
	}
	w.b.WriteByte(' ')
	w.b.WriteString(formatValue(v))
	w.b.WriteByte('\n')
}

// Gauge writes a family with a single unlabeled value.
func (w *Writer) Gauge(name, help string, v float64) {
	w.Family(name, "gauge", help)
	w.Sample(name, v)
}

// WriteTo writes all metrics, sorted by series within a family.
func (r *Registry) WriteTo(out io.Writer) (int64, error) {
	r.mu.Lock()
	counters, histograms, collectors := slices.Clone(r.counters), slices.Clone(r.histograms), slices.Clone(r.collectors)
	r.mu.Unlock()
	w := &Writer{b: &bytes.Buffer{}}
	for _, c := range counters {
		w.Family(c.name, "counter", c.help)
		c.mu.Lock()
		for _, s := range sorted(c.series) {
			w.Sample(c.name, s.n, pairs(c.labels, s.values)...)
		}
		c.mu.Unlock()
	}
	for _, h := range histograms {
		w.Family(h.name, "histogram", h.help)
		h.mu.Lock()
		for _, s := range sorted(h.series) {
			labels := pairs(h.labels, s.values)
			for i, b := range h.bounds {
				w.Sample(h.name+"_bucket", float64(s.buckets[i]), append(slices.Clone(labels), "le", formatValue(b))...)
			}
			w.Sample(h.name+"_bucket", s.n, append(slices.Clone(labels), "le", "+Inf")...)
			w.Sample(h.name+"_sum", s.sum, labels...)
			w.Sample(h.name+"_count", s.n, labels...)
		}
		h.mu.Unlock()
	}
	for _, fn := range collectors {
		fn(w)
	}
	return w.b.WriteTo(out)
}

func sorted(m map[string]*series) []*series {
	keys := slices.Sorted(func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	})
	out := make([]*series, len(keys))
	for i, k := range keys {
		out[i] = m[k]
	}
	return out
}

func pairs(names, values []string) []string {
	out := make([]string, 0, 2*len(names))
	for i, n := range names {
		out = append(out, n, values[i])
	}
	return out
}

func formatValue(v float64) string {
	switch {
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	case math.IsNaN(v):
		return "NaN"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

var valueEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
var helpEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`)

func escapeValue(s string) string { return valueEscaper.Replace(s) }
func escapeHelp(s string) string  { return helpEscaper.Replace(s) }

// ContentType is the media type of the text format.
const ContentType = "text/plain; version=0.0.4; charset=utf-8"
