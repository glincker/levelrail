package footprint

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sort"
	"sync"
	"time"
)

const jitterDivisor = 10

// LoopInfo is the observable state of one background loop.
type LoopInfo struct {
	Name         string    `json:"name"`
	IntervalSecs float64   `json:"interval_seconds"`
	Runs         int64     `json:"runs"`
	Errors       int64     `json:"errors"`
	LastRunAt    time.Time `json:"last_run_at"`
	LastDuration float64   `json:"last_duration_ms"`
	BusyMS       float64   `json:"busy_ms_total"`
	CostPercent  float64   `json:"cost_percent"`
	Running      bool      `json:"running"`
	LastError    string    `json:"last_error,omitempty"`

	registeredAt time.Time
	runners      int
}

// Loop records the runs of one background loop.
type Loop struct {
	mu   sync.Mutex
	info LoopInfo
}

var (
	regMu sync.Mutex
	reg   = map[string]*Loop{}
)

// Track returns the registry entry for name, creating it on first use.
func Track(name string, interval time.Duration) *Loop {
	regMu.Lock()
	defer regMu.Unlock()
	if l, ok := reg[name]; ok {
		return l
	}
	l := &Loop{info: LoopInfo{Name: name, IntervalSecs: interval.Seconds(), registeredAt: time.Now()}}
	reg[name] = l
	return l
}

// Observe records one finished run that began at start.
func (l *Loop) Observe(start time.Time, err error) {
	d := time.Since(start)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.info.Runs++
	l.info.LastRunAt = start.UTC()
	l.info.LastDuration = float64(d) / float64(time.Millisecond)
	l.info.BusyMS += l.info.LastDuration
	if err != nil {
		l.info.Errors++
		l.info.LastError = err.Error()
		return
	}
	l.info.LastError = ""
}

// Active marks the loop running and returns the func that marks it stopped.
func (l *Loop) Active() func() {
	l.mu.Lock()
	l.info.runners++
	l.info.Running = true
	l.mu.Unlock()
	return func() {
		l.mu.Lock()
		l.info.runners--
		l.info.Running = l.info.runners > 0
		l.mu.Unlock()
	}
}

// Every runs tick on interval until ctx ends. The first tick is delayed by a random jitter of up to a tenth of the interval so loops do not wake together, and each tick is recorded.
func Every(ctx context.Context, name string, interval time.Duration, logger *slog.Logger, tick func(context.Context) error) error {
	l := Track(name, interval)
	defer l.Active()()
	timer := time.NewTimer(interval + jitter(interval))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			start := time.Now()
			err := tick(ctx)
			l.Observe(start, err)
			if err != nil && logger != nil {
				logger.Warn(name+": tick failed", slog.String("error", err.Error()))
			}
			timer.Reset(interval)
		}
	}
}

func jitter(interval time.Duration) time.Duration {
	if interval <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(interval)/jitterDivisor + 1)) //nolint:gosec,forbidigo // scheduling jitter, not security
}

// Loops returns every registered loop, busiest first.
func Loops() []LoopInfo {
	regMu.Lock()
	ls := make([]*Loop, 0, len(reg))
	for _, l := range reg {
		ls = append(ls, l)
	}
	regMu.Unlock()
	out := make([]LoopInfo, 0, len(ls))
	for _, l := range ls {
		l.mu.Lock()
		i := l.info
		l.mu.Unlock()
		if el := time.Since(i.registeredAt).Seconds(); el > 0 {
			i.CostPercent = i.BusyMS / 1000 / el * 100
		}
		out = append(out, i)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].BusyMS != out[b].BusyMS {
			return out[a].BusyMS > out[b].BusyMS
		}
		return out[a].Name < out[b].Name
	})
	return out
}
