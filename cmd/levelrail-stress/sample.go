package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// recorder keeps request latencies per endpoint label.
type recorder struct {
	mu      sync.Mutex
	ms      map[string][]float64
	errs    map[string]int
	lastErr map[string]string
}

func newRecorder() *recorder {
	return &recorder{ms: map[string][]float64{}, errs: map[string]int{}, lastErr: map[string]string{}}
}

func (r *recorder) add(label string, d time.Duration, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ms[label] = append(r.ms[label], float64(d)/float64(time.Millisecond))
	if err != nil {
		r.errs[label]++
		r.lastErr[label] = err.Error()
	}
}

// do times one request and records it; a status of 400 or above counts as an error.
func (r *recorder) do(ctx context.Context, c *http.Client, label, method, url string, body []byte) {
	var rd io.Reader
	if body != nil {
		rd = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		r.add(label, 0, err)
		return
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	start := time.Now()
	resp, err := c.Do(req)
	if err == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= http.StatusBadRequest {
			err = fmt.Errorf("status %d", resp.StatusCode)
		}
	}
	if ctx.Err() != nil {
		return
	}
	r.add(label, time.Since(start), err)
}

// EndpointStat is the latency summary for one endpoint label.
type EndpointStat struct {
	Endpoint string  `json:"endpoint"`
	Count    int     `json:"count"`
	Errors   int     `json:"errors"`
	LastErr  string  `json:"last_error,omitempty"`
	P50Ms    float64 `json:"p50_ms"`
	P95Ms    float64 `json:"p95_ms"`
	P99Ms    float64 `json:"p99_ms"`
	MaxMs    float64 `json:"max_ms"`
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	return sorted[max(0, min(idx, len(sorted)-1))]
}

func (r *recorder) stats() []EndpointStat {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]EndpointStat, 0, len(r.ms))
	for label, v := range r.ms {
		s := append([]float64(nil), v...)
		sort.Float64s(s)
		out = append(out, EndpointStat{
			Endpoint: label, Count: len(s), Errors: r.errs[label], LastErr: r.lastErr[label],
			P50Ms: percentile(s, 50), P95Ms: percentile(s, 95), P99Ms: percentile(s, 99), MaxMs: s[len(s)-1],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Endpoint < out[j].Endpoint })
	return out
}

// Resources are the peak control plane process readings over a run.
type Resources struct {
	MaxRSSMB           float64 `json:"max_rss_mb"`
	MaxGoroutines      int     `json:"max_goroutines"`
	MaxSQLiteMB        float64 `json:"max_sqlite_mb"`
	MaxReconcilePassMs float64 `json:"max_reconcile_pass_ms"`
	ReconcileP95Ms     float64 `json:"reconcile_p95_ms"`
	ReconcilePasses    int64   `json:"reconcile_passes"`
	Samples            int     `json:"samples"`
}

// sampler polls the control plane process once a second.
type sampler struct {
	pid      int
	dataDir  string
	pprof    string
	api      string
	client   *http.Client
	mu       sync.Mutex
	res      Resources
	lastPoll time.Time
}

func (s *sampler) run(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sample(ctx)
		}
	}
}

func (s *sampler) sample(ctx context.Context) {
	rss := float64(rssKB(s.pid)) / 1024
	gor := s.goroutines(ctx)
	size := float64(dbBytes(s.dataDir)) / (1 << 20)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.res.Samples++
	s.res.MaxRSSMB = math.Max(s.res.MaxRSSMB, rss)
	s.res.MaxSQLiteMB = math.Max(s.res.MaxSQLiteMB, size)
	if gor > s.res.MaxGoroutines {
		s.res.MaxGoroutines = gor
	}
	if time.Since(s.lastPoll) >= 3*time.Second {
		s.lastPoll = time.Now()
		s.mu.Unlock()
		last, p95, passes := s.reconcile(ctx)
		s.mu.Lock()
		s.res.MaxReconcilePassMs = math.Max(s.res.MaxReconcilePassMs, last)
		s.res.ReconcileP95Ms = math.Max(s.res.ReconcileP95Ms, p95)
		s.res.ReconcilePasses = max(s.res.ReconcilePasses, passes)
	}
}

func (s *sampler) snapshot() Resources {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.res
}

// goroutines reads the total from the pprof text profile header.
func (s *sampler) goroutines(ctx context.Context) int {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.pprof+"/debug/pprof/goroutine?debug=1", nil)
	if err != nil {
		return 0
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0
	}
	defer func() { _ = resp.Body.Close() }()
	head := make([]byte, 128)
	n, _ := io.ReadFull(resp.Body, head)
	line, _, _ := strings.Cut(string(head[:n]), "\n")
	n2, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "goroutine profile: total")))
	return n2
}

func (s *sampler) reconcile(ctx context.Context) (last, p95 float64, passes int64) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.api+"/api/v1/system/control-plane-health", nil)
	if err != nil {
		return 0, 0, 0
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, 0, 0
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Reconcile struct {
			Passes     int64   `json:"passes"`
			LastPassMS float64 `json:"last_pass_ms"`
			P95MS      float64 `json:"p95_ms"`
		} `json:"reconcile"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) != nil {
		return 0, 0, 0
	}
	return body.Reconcile.LastPassMS, body.Reconcile.P95MS, body.Reconcile.Passes
}

// dbBytes sums the SQLite files in the data dir, WAL included.
func dbBytes(dir string) int64 {
	var total int64
	matches, _ := filepath.Glob(filepath.Join(dir, "*.db*"))
	for _, m := range matches {
		if st, err := os.Stat(m); err == nil {
			total += st.Size()
		}
	}
	return total
}

func fmtMs(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
