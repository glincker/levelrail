package store

import (
	"testing"
	"time"
)

func i64(v int64) *int64 { return &v }

func TestSleepEventsInsertAndTrim(t *testing.T) {
	t.Setenv(EnvSleepEventsKeep, "3")
	db := openTestDB(t)
	ctx := t.Context()
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		if _, err := db.AddSleepEvent(ctx, SleepEvent{ServiceName: "web", Kind: SleepEventSlept, At: t0.Add(time.Duration(i) * time.Hour), Reason: SleepReasonIdleTimeout}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.AddSleepEvent(ctx, SleepEvent{ServiceName: "other", Kind: SleepEventSlept, At: t0}); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListSleepEvents(ctx, "web", time.Time{}, 0)
	if err != nil || len(got) != 3 {
		t.Fatalf("events = %d, %v; want 3 kept", len(got), err)
	}
	if !got[0].At.Equal(t0.Add(4*time.Hour)) || got[0].Actor != "system" {
		t.Errorf("newest = %+v", got[0])
	}
	if o, _ := db.ListSleepEvents(ctx, "other", time.Time{}, 0); len(o) != 1 {
		t.Errorf("other app trimmed: %d", len(o))
	}
	id := got[0].ID
	if err := db.SetSleepEventLatency(ctx, id, 1500); err != nil {
		t.Fatal(err)
	}
	got, _ = db.ListSleepEvents(ctx, "web", t0.Add(4*time.Hour), 0)
	if len(got) != 1 || got[0].WakeLatencyMs == nil || *got[0].WakeLatencyMs != 1500 {
		t.Errorf("latency not stored: %+v", got)
	}
}

func TestComputeSleepStats(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	h := func(d float64) time.Time { return now.Add(-time.Duration(d * float64(time.Hour))) }
	events := []SleepEvent{
		{Kind: SleepEventWoke, At: h(24 * 29), Reason: "manual", WakeLatencyMs: i64(1000)},
		{Kind: SleepEventSlept, At: h(24 * 8)},
		{Kind: SleepEventWoke, At: h(24*7 - 6), Reason: "request", WakeLatencyMs: i64(3000)},
		{Kind: SleepEventSlept, At: h(5)},
	}
	st := ComputeSleepStats(events, true, nil, now)
	// 30d: prior sleep opened at window start until 29d ago = 1d, then 18h clipped
	// to 7d window edge; 7d: 6h of the edge interval + open 5h.
	wantOpen := int64(5 * 3600)
	if st.CurrentlyAsleepSince == nil || !st.CurrentlyAsleepSince.Equal(h(5)) {
		t.Fatalf("since = %v", st.CurrentlyAsleepSince)
	}
	if st.AsleepSeconds7d != 6*3600+wantOpen {
		t.Errorf("7d = %d, want %d", st.AsleepSeconds7d, 6*3600+wantOpen)
	}
	wantInterval1 := int64(24 * 3600)
	wantInterval2 := int64((24*8 - (24*7 - 6)) * 3600)
	if st.AsleepSeconds30d != wantInterval1+wantInterval2+wantOpen {
		t.Errorf("30d = %d, want %d", st.AsleepSeconds30d, wantInterval1+wantInterval2+wantOpen)
	}
	if st.WakeCount7d != 1 || st.LastWakeReason != "request" {
		t.Errorf("wakes = %d reason %q", st.WakeCount7d, st.LastWakeReason)
	}
	if st.AvgWakeLatencyMs == nil || *st.AvgWakeLatencyMs != 2000 || *st.P95WakeLatencyMs != 3000 {
		t.Errorf("latency avg=%v p95=%v", st.AvgWakeLatencyMs, st.P95WakeLatencyMs)
	}
}

func TestComputeSleepStatsLegacyOpenSleep(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	since := now.Add(-2 * time.Hour)
	st := ComputeSleepStats(nil, false, &since, now)
	if st.AsleepSeconds7d != 7200 || st.CurrentlyAsleepSince == nil {
		t.Errorf("stats = %+v", st)
	}
}

func TestGetSleepStatsPriorAsleep(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	now := time.Now().UTC()
	if err := db.SetAppSleepIdle(ctx, "web", 30, now.Add(-90*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddSleepEvent(ctx, SleepEvent{ServiceName: "web", Kind: SleepEventSlept, At: now.Add(-60 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	st, err := db.GetSleepStats(ctx, "web", now)
	if err != nil {
		t.Fatal(err)
	}
	if st.AsleepSeconds7d < 7*24*3600-5 || st.CurrentlyAsleepSince == nil {
		t.Errorf("a sleep older than the window must fill it: %+v", st)
	}
}
