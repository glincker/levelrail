// Package bench is an open-loop HTTP load driver with failure-window analysis.
package bench

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"
)

const defaultMaxInFlight = 20000

// Doer is the subset of *http.Client the driver needs.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Config describes one constant-arrival-rate run.
type Config struct {
	URL         string
	Rate        float64
	Duration    time.Duration
	Timeout     time.Duration
	MaxInFlight int
}

// Sample is one request, positioned by its intended send time.
type Sample struct {
	Offset  time.Duration
	Latency time.Duration
	Failed  bool
}

// Event is an action fired at a fixed offset into a run.
type Event struct {
	Name string
	At   time.Duration
	Fire func(ctx context.Context) error
}

// Stats summarises a run. Latencies are milliseconds measured from the intended send time.
type Stats struct {
	Sent        int     `json:"sent"`
	Succeeded   int     `json:"succeeded"`
	Failed      int     `json:"failed"`
	Saturated   int     `json:"client_saturated"`
	AchievedRPS float64 `json:"achieved_rps"`
	ErrorRate   float64 `json:"error_rate"`
	P50MS       float64 `json:"p50_ms"`
	P95MS       float64 `json:"p95_ms"`
	P99MS       float64 `json:"p99_ms"`
	MaxMS       float64 `json:"max_ms"`
}

// EventReport is the failure analysis for one fired event.
type EventReport struct {
	Name          string  `json:"name"`
	FiredAtMS     float64 `json:"fired_at_ms"`
	FireError     string  `json:"fire_error,omitempty"`
	Dropped       int     `json:"dropped_requests"`
	FirstErrorMS  float64 `json:"first_error_ms"`
	ErrorWindowMS float64 `json:"error_window_ms"`
	RecoveryMS    float64 `json:"recovery_ms"`
	Recovered     bool    `json:"recovered"`
}

// Result is the outcome of Run.
type Result struct {
	Stats  Stats        `json:"stats"`
	Event  *EventReport `json:"event,omitempty"`
	Sample []Sample     `json:"-"`
}

// Schedule returns the intended send offsets of a constant-rate run.
func Schedule(rate float64, d time.Duration) []time.Duration {
	if rate <= 0 || d <= 0 {
		return nil
	}
	n := int(rate * d.Seconds())
	interval := time.Duration(float64(time.Second) / rate)
	out := make([]time.Duration, n)
	for i := range out {
		out[i] = time.Duration(i) * interval
	}
	return out
}

// Percentile returns the nearest-rank percentile (0 < p <= 100) of sorted.
func Percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(p/100*float64(len(sorted)) + 0.999999999)
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

// Summarize computes Stats from samples; elapsed is the wall time of the run.
func Summarize(samples []Sample, elapsed time.Duration) Stats {
	s := Stats{Sent: len(samples)}
	lat := make([]time.Duration, 0, len(samples))
	for _, x := range samples {
		if x.Failed {
			s.Failed++
		} else {
			s.Succeeded++
		}
		lat = append(lat, x.Latency)
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	if s.Sent > 0 {
		s.ErrorRate = float64(s.Failed) / float64(s.Sent)
	}
	if elapsed > 0 {
		s.AchievedRPS = float64(s.Succeeded) / elapsed.Seconds()
	}
	s.P50MS = ms(Percentile(lat, 50))
	s.P95MS = ms(Percentile(lat, 95))
	s.P99MS = ms(Percentile(lat, 99))
	if len(lat) > 0 {
		s.MaxMS = ms(lat[len(lat)-1])
	}
	return s
}

// AnalyzeEvent measures the failures at or after eventAt. A run counts as
// recovered when its last failure is more than tail before the run ends.
func AnalyzeEvent(samples []Sample, eventAt, runLen, tail time.Duration) EventReport {
	r := EventReport{FiredAtMS: ms(eventAt), Recovered: true}
	var first, last time.Duration
	for _, x := range samples {
		if !x.Failed || x.Offset < eventAt {
			continue
		}
		if r.Dropped == 0 || x.Offset < first {
			first = x.Offset
		}
		if x.Offset > last {
			last = x.Offset
		}
		r.Dropped++
	}
	if r.Dropped == 0 {
		return r
	}
	r.FirstErrorMS = ms(first)
	r.ErrorWindowMS = ms(last - first)
	r.RecoveryMS = ms(last - eventAt)
	r.Recovered = runLen-last > tail
	return r
}

// Overhead is the latency a proxied path adds over a direct one, in milliseconds.
type Overhead struct {
	P50MS float64 `json:"p50_ms"`
	P95MS float64 `json:"p95_ms"`
	P99MS float64 `json:"p99_ms"`
}

// Compare returns proxied minus direct latency per percentile.
func Compare(proxied, direct Stats) Overhead {
	return Overhead{P50MS: proxied.P50MS - direct.P50MS, P95MS: proxied.P95MS - direct.P95MS, P99MS: proxied.P99MS - direct.P99MS}
}

// Run drives cfg.URL at a constant arrival rate. Each request is launched on
// its own goroutine at its intended time and latency is measured from that
// time, so a stalled server cannot hide its own queueing delay.
func Run(ctx context.Context, d Doer, cfg Config, ev *Event) (Result, error) {
	if cfg.URL == "" {
		return Result{}, errors.New("bench: empty URL")
	}
	sched := Schedule(cfg.Rate, cfg.Duration)
	if len(sched) == 0 {
		return Result{}, errors.New("bench: rate and duration must be positive")
	}
	maxIn := cfg.MaxInFlight
	if maxIn <= 0 {
		maxIn = defaultMaxInFlight
	}
	sem := make(chan struct{}, maxIn)
	samples := make([]Sample, len(sched))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var lastDone time.Time
	var saturated int
	var fired *EventReport
	start := time.Now()

	if ev != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !sleepUntil(ctx, start.Add(ev.At)) {
				return
			}
			rep := EventReport{Name: ev.Name}
			if err := ev.Fire(ctx); err != nil {
				rep.FireError = err.Error()
			}
			mu.Lock()
			fired = &rep
			mu.Unlock()
		}()
	}

	for i, off := range sched {
		if !sleepUntil(ctx, start.Add(off)) {
			samples = samples[:i]
			break
		}
		select {
		case sem <- struct{}{}:
		default:
			samples[i] = Sample{Offset: off, Failed: true, Latency: time.Since(start.Add(off))}
			saturated++
			continue
		}
		wg.Add(1)
		go func(i int, off time.Duration) {
			defer wg.Done()
			defer func() { <-sem }()
			failed := doOne(ctx, d, cfg.URL, cfg.Timeout)
			now := time.Now()
			samples[i] = Sample{Offset: off, Latency: now.Sub(start.Add(off)), Failed: failed}
			mu.Lock()
			if now.After(lastDone) {
				lastDone = now
			}
			mu.Unlock()
		}(i, off)
	}
	wg.Wait()

	elapsed := lastDone.Sub(start)
	if elapsed < cfg.Duration {
		elapsed = cfg.Duration
	}
	res := Result{Stats: Summarize(samples, elapsed), Sample: samples}
	res.Stats.Saturated = saturated
	if fired != nil {
		a := AnalyzeEvent(samples, ev.At, cfg.Duration, time.Second)
		a.Name, a.FireError = fired.Name, fired.FireError
		res.Event = &a
	}
	return res, ctx.Err()
}

func doOne(ctx context.Context, d Doer, url string, timeout time.Duration) bool {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return true
	}
	resp, err := d.Do(req)
	if err != nil {
		return true
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode >= http.StatusInternalServerError
}

func sleepUntil(ctx context.Context, t time.Time) bool {
	wait := time.Until(t)
	if wait <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
