package store

import (
	"context"
	"fmt"
	"sort"
)

// WakeTimingSamples is how many of an app's most recent timed wakes feed its
// wake timing, so the numbers follow the app as it changes.
const WakeTimingSamples = 50

// WakeTiming summarises an app's recent wakes. Ready is start to the first
// ready signal, Released is start to the moment held requests were let go.
// Pointers are nil when no wake has been timed yet.
type WakeTiming struct {
	Samples        int
	ReadyLastMs    *int64
	ReadyP50Ms     *int64
	ReadyP95Ms     *int64
	ReleasedLastMs *int64
	ReleasedP50Ms  *int64
	ReleasedP95Ms  *int64
}

// WakeSample is one timed wake.
type WakeSample struct {
	ReadyMs    int64
	ReleasedMs *int64
}

// ComputeWakeTiming derives the summary from samples, newest first.
func ComputeWakeTiming(samples []WakeSample) WakeTiming {
	var t WakeTiming
	if len(samples) == 0 {
		return t
	}
	if len(samples) > WakeTimingSamples {
		samples = samples[:WakeTimingSamples]
	}
	t.Samples = len(samples)
	ready := make([]int64, 0, len(samples))
	var released []int64
	for _, s := range samples {
		ready = append(ready, s.ReadyMs)
		if s.ReleasedMs != nil {
			released = append(released, *s.ReleasedMs)
		}
	}
	last := samples[0].ReadyMs
	t.ReadyLastMs = &last
	t.ReadyP50Ms, t.ReadyP95Ms = percentiles(ready)
	t.ReleasedLastMs = samples[0].ReleasedMs
	t.ReleasedP50Ms, t.ReleasedP95Ms = percentiles(released)
	return t
}

func percentiles(v []int64) (p50, p95 *int64) {
	n := len(v)
	if n == 0 {
		return nil, nil
	}
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	a, b := s[(n*50+99)/100-1], s[(n*95+99)/100-1]
	return &a, &b
}

// SetSleepEventReleased records how long a wake held requests before letting them go.
func (db *DB) SetSleepEventReleased(ctx context.Context, id, releasedMs int64) error {
	if _, err := db.ExecContext(ctx, `UPDATE sleep_events SET wake_released_ms = ? WHERE id = ?`, releasedMs, id); err != nil {
		return fmt.Errorf("store: set sleep event released: %w", err)
	}
	return nil
}

// GetWakeTiming summarises name's most recent timed wakes.
func (db *DB) GetWakeTiming(ctx context.Context, name string) (WakeTiming, error) {
	rows, err := db.QueryContext(ctx, `SELECT wake_latency_ms, wake_released_ms FROM sleep_events
		WHERE service_name = ? AND kind = ? AND wake_latency_ms IS NOT NULL ORDER BY at DESC, id DESC LIMIT ?`,
		name, SleepEventWoke, WakeTimingSamples)
	if err != nil {
		return WakeTiming{}, fmt.Errorf("store: wake timing for %q: %w", name, err)
	}
	defer func() { _ = rows.Close() }()
	var samples []WakeSample
	for rows.Next() {
		var s WakeSample
		if err := rows.Scan(&s.ReadyMs, &s.ReleasedMs); err != nil {
			return WakeTiming{}, fmt.Errorf("store: scan wake timing: %w", err)
		}
		samples = append(samples, s)
	}
	return ComputeWakeTiming(samples), rows.Err()
}
