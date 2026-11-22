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
	App          string        `json:"app"`
	Tag          string        `json:"tag"`
	StartedAt    time.Time     `json:"started_at"`
	Duration     time.Duration `json:"-"`
	QueueWait    time.Duration `json:"-"`
	Status       RunStatus     `json:"status"`
	CachedSteps  int           `json:"cached_steps"`
	TotalSteps   int           `json:"total_steps"`
	Error        string        `json:"error,omitempty"`
	Node         string        `json:"node,omitempty"`
	RunnerReason string        `json:"runner_reason,omitempty"`
	Platform     string        `json:"platform,omitempty"`
	ImageBytes   int64         `json:"image_bytes,omitempty"`
	// PeakMemoryMB is the build cgroup high water mark, 0 when it could not be read.
	PeakMemoryMB int `json:"peak_memory_mb,omitempty"`
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

// FleetBucket is one hour of the farm: builds started and average slots busy.
type FleetBucket struct {
	Start      time.Time `json:"start"`
	Builds     int       `json:"builds"`
	AvgSlots   float64   `json:"avg_slots"`
	WaitP95Sec float64   `json:"wait_p95_seconds"`
}

// FleetSummary aggregates every app's builds over a window to size the farm.
type FleetSummary struct {
	Builds        int           `json:"builds"`
	BuildsPerHour float64       `json:"builds_per_hour"`
	WaitP50Sec    float64       `json:"wait_p50_seconds"`
	WaitP95Sec    float64       `json:"wait_p95_seconds"`
	CacheHitRatio float64       `json:"cache_hit_ratio"`
	CacheKnown    bool          `json:"cache_known"`
	PeakSlots     float64       `json:"peak_slots"`
	Buckets       []FleetBucket `json:"buckets"`
}

// ComputeFleetSummary folds runs started inside the hours ending at now into
// hourly buckets. Slots busy is build time overlapping each hour, so it needs
// no sampling; canceled builds never waited for a slot and are skipped.
func ComputeFleetSummary(runs []Run, now time.Time, hours int) FleetSummary {
	if hours <= 0 {
		hours = 1
	}
	end := now.UTC().Truncate(time.Hour).Add(time.Hour)
	from := end.Add(-time.Duration(hours) * time.Hour)
	s := FleetSummary{Buckets: make([]FleetBucket, hours)}
	waits := make([][]float64, hours)
	var allWaits []float64
	var cached, total int
	for i := range s.Buckets {
		s.Buckets[i].Start = from.Add(time.Duration(i) * time.Hour)
	}
	for _, r := range runs {
		if r.Status == RunCanceled || r.Duration < 0 {
			continue
		}
		start := r.StartedAt.UTC()
		if start.Before(from) || !start.Before(end) {
			continue
		}
		s.Builds++
		idx := int(start.Sub(from) / time.Hour)
		s.Buckets[idx].Builds++
		w := r.QueueWait.Seconds()
		allWaits = append(allWaits, w)
		waits[idx] = append(waits[idx], w)
		if r.Status == RunSuccess && r.TotalSteps > 0 {
			cached += r.CachedSteps
			total += r.TotalSteps
		}
		stop := start.Add(r.Duration)
		for i := idx; i < hours && s.Buckets[i].Start.Before(stop); i++ {
			bs, be := s.Buckets[i].Start, s.Buckets[i].Start.Add(time.Hour)
			lo, hi := maxTime(bs, start), minTime(be, stop)
			if hi.After(lo) {
				s.Buckets[i].AvgSlots += hi.Sub(lo).Hours()
			}
		}
	}
	for i := range s.Buckets {
		if len(waits[i]) > 0 {
			sort.Float64s(waits[i])
			s.Buckets[i].WaitP95Sec = percentile(waits[i], 0.95)
		}
		if s.Buckets[i].AvgSlots > s.PeakSlots {
			s.PeakSlots = s.Buckets[i].AvgSlots
		}
	}
	s.BuildsPerHour = float64(s.Builds) / float64(hours)
	if total > 0 {
		s.CacheKnown, s.CacheHitRatio = true, float64(cached)/float64(total)
	}
	if len(allWaits) > 0 {
		sort.Float64s(allWaits)
		s.WaitP50Sec, s.WaitP95Sec = percentile(allWaits, 0.50), percentile(allWaits, 0.95)
	}
	return s
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
