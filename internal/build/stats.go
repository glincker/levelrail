package build

import (
	"math"
	"sort"
	"strings"
	"time"
)

// RunStatus is how one build ended.
type RunStatus string

// Build run outcomes recorded by the Limiter.
const (
	RunSuccess  RunStatus = "success"
	RunFailed   RunStatus = "failed"
	RunTimeout  RunStatus = "timeout"
	RunCanceled RunStatus = "canceled"
	// RunMemoryLimit is a build killed for exceeding its memory limit.
	RunMemoryLimit RunStatus = "memory_limit"
)

// Run is one finished build as the Limiter saw it.
type Run struct {
	App         string        `json:"app"`
	Tag         string        `json:"tag"`
	StartedAt   time.Time     `json:"started_at"`
	Duration    time.Duration `json:"-"`
	QueueWait   time.Duration `json:"-"`
	Status      RunStatus     `json:"status"`
	CachedSteps int           `json:"cached_steps"`
	TotalSteps  int           `json:"total_steps"`
	Error       string        `json:"error,omitempty"`
	Node        string        `json:"node,omitempty"`
	Platform    string        `json:"platform,omitempty"`
	ImageBytes  int64         `json:"image_bytes,omitempty"`
}

// Trend summarises the last N builds of one app. Percentiles cover successful
// builds only; failed, timed out and canceled builds are counted, not timed.
type Trend struct {
	Builds     int     `json:"builds"`
	Succeeded  int     `json:"succeeded"`
	Failed     int     `json:"failed"`
	TimedOut   int     `json:"timed_out"`
	Canceled   int     `json:"canceled"`
	Skewed     int     `json:"skewed"`
	P50Seconds float64 `json:"p50_seconds"`
	P95Seconds float64 `json:"p95_seconds"`
	MinSeconds float64 `json:"min_seconds"`
	MaxSeconds float64 `json:"max_seconds"`
	// CacheHitRatio is cached steps over total steps across successful builds,
	// valid only when CacheKnown is true.
	CacheHitRatio float64 `json:"cache_hit_ratio"`
	CacheKnown    bool    `json:"cache_known"`
}

// ComputeTrend folds runs into a Trend. A successful run with a negative
// duration (clock skew) is counted in Skewed and left out of the timings.
func ComputeTrend(runs []Run) Trend {
	t := Trend{Builds: len(runs)}
	var secs []float64
	var cached, total int
	for _, r := range runs {
		switch r.Status {
		case RunSuccess:
			t.Succeeded++
		case RunTimeout:
			t.TimedOut++
			continue
		case RunCanceled:
			t.Canceled++
			continue
		default:
			t.Failed++
			continue
		}
		if r.Duration < 0 {
			t.Skewed++
			continue
		}
		secs = append(secs, r.Duration.Seconds())
		if r.TotalSteps > 0 {
			cached += r.CachedSteps
			total += r.TotalSteps
		}
	}
	if total > 0 {
		t.CacheKnown = true
		t.CacheHitRatio = float64(cached) / float64(total)
	}
	if len(secs) == 0 {
		return t
	}
	sort.Float64s(secs)
	t.MinSeconds, t.MaxSeconds = secs[0], secs[len(secs)-1]
	t.P50Seconds = percentile(secs, 0.50)
	t.P95Seconds = percentile(secs, 0.95)
	return t
}

// percentile interpolates linearly between ranks of an ascending slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
}

// AppFromTag maps an image tag like "reg.io/org/web:abc" to the app key "web".
func AppFromTag(tag string) string {
	repo := tag
	if i := strings.LastIndex(repo, ":"); i > strings.LastIndex(repo, "/") {
		repo = repo[:i]
	}
	if i := strings.LastIndex(repo, "/"); i >= 0 {
		repo = repo[i+1:]
	}
	return repo
}

// stepCounter tallies distinct completed build steps and how many were cached.
type stepCounter struct {
	seen   map[string]bool
	cached int
}

func (s *stepCounter) observe(ev ProgressEvent) {
	if !ev.Completed || ev.Step == "" || ev.Error != "" {
		return
	}
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	if s.seen[ev.Step] {
		return
	}
	s.seen[ev.Step] = true
	if ev.Cached {
		s.cached++
	}
}

func (s *stepCounter) total() int { return len(s.seen) }
