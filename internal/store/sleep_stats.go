package store

import (
	"context"
	"fmt"
	"sort"
	"time"
)

const (
	sleepWindow7d  = 7 * 24 * time.Hour
	sleepWindow30d = 30 * sleepWindow7d / 7
)

// SleepStats aggregates an app's sleep history. Asleep time counts an open
// sleep interval up to now and clips intervals at the window edges.
type SleepStats struct {
	CurrentlyAsleepSince *time.Time
	AsleepSeconds7d      int64
	AsleepSeconds30d     int64
	WakeCount7d          int
	AvgWakeLatencyMs     *int64
	P95WakeLatencyMs     *int64
	LastSleptAt          *time.Time
	LastWokeAt           *time.Time
	LastWakeReason       string
}

type sleepInterval struct{ from, to time.Time }

// ComputeSleepStats derives stats at now from events in the last 30 days
// (oldest first). priorAsleep says the app was asleep when the window opened;
// currentSince is app_sleep.Since when the app is asleep now, which covers
// sleeps that predate the history table.
func ComputeSleepStats(events []SleepEvent, priorAsleep bool, currentSince *time.Time, now time.Time) SleepStats {
	start30 := now.Add(-sleepWindow30d)
	var intervals []sleepInterval
	var open *time.Time
	if priorAsleep {
		t := start30
		open = &t
	}
	var st SleepStats
	var latencies []int64
	for _, e := range events {
		switch e.Kind {
		case SleepEventSlept:
			if open == nil {
				t := e.At
				open = &t
			}
			t := e.At
			st.LastSleptAt = &t
		case SleepEventWoke:
			if open != nil {
				intervals = append(intervals, sleepInterval{*open, e.At})
				open = nil
			}
			t := e.At
			st.LastWokeAt = &t
			st.LastWakeReason = e.Reason
			if e.At.After(now.Add(-sleepWindow7d)) {
				st.WakeCount7d++
			}
			if e.WakeLatencyMs != nil {
				latencies = append(latencies, *e.WakeLatencyMs)
			}
		}
	}
	if open == nil && currentSince != nil {
		open = currentSince
	}
	if open != nil {
		intervals = append(intervals, sleepInterval{*open, now})
		t := *open
		st.CurrentlyAsleepSince = &t
	}
	st.AsleepSeconds7d = overlapSeconds(intervals, now.Add(-sleepWindow7d), now)
	st.AsleepSeconds30d = overlapSeconds(intervals, start30, now)
	if n := len(latencies); n > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		var sum int64
		for _, l := range latencies {
			sum += l
		}
		avg := sum / int64(n)
		p95 := latencies[(n*95+99)/100-1]
		st.AvgWakeLatencyMs, st.P95WakeLatencyMs = &avg, &p95
	}
	return st
}

func overlapSeconds(iv []sleepInterval, from, to time.Time) int64 {
	var total time.Duration
	for _, i := range iv {
		a, b := i.from, i.to
		if a.Before(from) {
			a = from
		}
		if b.After(to) {
			b = to
		}
		if b.After(a) {
			total += b.Sub(a)
		}
	}
	return int64(total / time.Second)
}

// GetSleepStats loads name's history and aggregates it at now.
func (db *DB) GetSleepStats(ctx context.Context, name string, now time.Time) (SleepStats, error) {
	start := now.Add(-sleepWindow30d)
	events, err := db.ListSleepEventsAsc(ctx, name, start)
	if err != nil {
		return SleepStats{}, err
	}
	prior, err := db.LastSleepStateEventBefore(ctx, name, start)
	if err != nil {
		return SleepStats{}, err
	}
	cur, err := db.GetAppSleep(ctx, name)
	if err != nil {
		return SleepStats{}, fmt.Errorf("store: sleep stats for %q: %w", name, err)
	}
	var since *time.Time
	if cur.Sleeping {
		since = &cur.Since
	}
	return ComputeSleepStats(events, prior != nil && prior.Kind == SleepEventSlept, since, now), nil
}
