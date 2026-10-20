package deploywatch

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// splitMetrics answers with baseline values for queries ending at or before
// cutover and with release values after it.
type splitMetrics struct {
	cutover       time.Time
	before, after map[string]float64
}

func (f splitMetrics) QueryMetrics(_ context.Context, _ string, metric string, _, to time.Time) ([]telemetry.Sample, error) {
	src := f.after
	if !to.After(f.cutover) {
		src = f.before
	}
	return []telemetry.Sample{{Metric: metric, Value: src[metric], Timestamp: to}}, nil
}

func latencyMetrics(cutover time.Time, requests float64) splitMetrics {
	return splitMetrics{
		cutover: cutover,
		before:  map[string]float64{telemetry.MetricHTTPRequests: requests, telemetry.LatencyBucketMetric(3): requests},
		after:   map[string]float64{telemetry.MetricHTTPRequests: requests, telemetry.LatencyBucketMetric(7): requests},
	}
}

func TestWatcher_NoisySingleRequestDoesNotTrip(t *testing.T) {
	f := newWatchFixture(t, true)
	ctx := context.Background()
	f.watcher.Metrics = fakeMetrics{telemetry.MetricHTTPRequests: 1, telemetry.MetricHTTPResponses5xx: 1}
	f.watcher.Tick(ctx)
	if events := f.events(t); hasEvent(events, deploysafety.PhaseWatch, deploysafety.StatusFailed, "") {
		t.Fatalf("events = %+v, one failed request must not trip the watch", events)
	}
	if svc, _ := f.db.GetDesiredService(ctx, "web"); svc.Image != "app:v2" {
		t.Errorf("image = %q, want no rollback", svc.Image)
	}
}

func TestWatcher_LatencyRegressionIsSoft(t *testing.T) {
	tests := []struct {
		name         string
		softRollback bool
		slow         bool
		ticks        int
		wantRollback bool
		wantNotice   bool
	}{
		{"one slow tick is ignored", true, true, 1, false, false},
		{"sustained, notify only by default", false, true, 3, false, true},
		{"sustained, opted in to soft rollback", true, true, 3, true, false},
		{"healthy latency", true, false, 3, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newWatchFixture(t, true)
			ctx := context.Background()
			cutover := f.now.Add(-time.Minute)
			m := latencyMetrics(cutover, 100)
			if !tt.slow {
				m.after = m.before
			}
			f.watcher.Metrics = m
			if tt.softRollback {
				on := true
				if err := f.db.SaveDeploySafetySettings(ctx, store.DeploySafetySettings{ServiceName: "web", WatchSoftRollback: &on}); err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < tt.ticks; i++ {
				f.watcher.Tick(ctx)
			}
			svc, _ := f.db.GetDesiredService(ctx, "web")
			if rolledBack := svc.Image != "app:v2"; rolledBack != tt.wantRollback {
				t.Errorf("rolled back = %v (image %q), want %v", rolledBack, svc.Image, tt.wantRollback)
			}
			notice := hasEvent(f.events(t), deploysafety.PhaseWatch, deploysafety.StatusInfo, deploysafety.ReasonWatchLatency)
			if notice != tt.wantNotice {
				t.Errorf("latency notice = %v, want %v (events %+v)", notice, tt.wantNotice, f.events(t))
			}
		})
	}
}
