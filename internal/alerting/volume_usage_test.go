package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type fakeVolumes []VolumeRef

func (f fakeVolumes) ListVolumes(context.Context) ([]VolumeRef, error) { return f, nil }

type fakeVolumeMetrics struct{ used, total float64 }

func (f *fakeVolumeMetrics) QueryMetrics(_ context.Context, _, metric string, _, to time.Time) ([]telemetry.Sample, error) {
	v := f.used
	if metric == MetricVolumeTotalBytes {
		v = f.total
	}
	return []telemetry.Sample{{Metric: metric, Timestamp: to, Value: v}}, nil
}

func TestEvaluateVolumeUsageFiresOnceAndResolves(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	m := &fakeVolumeMetrics{used: 91, total: 100}
	vols := fakeVolumes{{ID: "v1", Name: "data", App: "web"}}
	r := Rule{ID: "r1", Kind: KindVolumeUsage}

	r1, notices, err := EvaluateVolumeUsage(ctx, vols, m, r, 90, now, nil)
	if err != nil || !r1.Firing || len(notices) != 1 {
		t.Fatalf("first: firing=%v notices=%d err=%v", r1.Firing, len(notices), err)
	}
	first := *r1.FiringSince

	r2, _, _ := EvaluateVolumeUsage(ctx, vols, m, r1, 90, now.Add(time.Minute), nil)
	if !r2.Firing || !r2.FiringSince.Equal(first) {
		t.Fatalf("second: stayed firing with same start expected, got firing=%v", r2.Firing)
	}

	m.used = 50
	r3, notices, _ := EvaluateVolumeUsage(ctx, vols, m, r2, 90, now.Add(2*time.Minute), nil)
	if r3.Firing || len(notices) != 0 {
		t.Fatalf("third: expected resolved, firing=%v notices=%d", r3.Firing, len(notices))
	}
}

func TestVolumeUsageEngineCase(t *testing.T) {
	e := &Engine{}
	e.SetVolumeUsage(nil, func() float64 { return 90 })
	if e.volumeThreshold() != 90 {
		t.Fatal("threshold func not stored")
	}
}
