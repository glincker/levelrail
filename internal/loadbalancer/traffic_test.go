package loadbalancer

import (
	"math"
	"testing"
	"time"
)

var testBounds = []float64{10, 100}

func snap(sec int, req, errs uint64, lat ...uint64) trafficSnap {
	return trafficSnap{at: time.Unix(1000, 0).Add(time.Duration(sec) * time.Second), t: TrafficTotals{Requests: req, Errors: errs, Latency: lat}}
}

func TestComputeTraffic(t *testing.T) {
	tests := []struct {
		name              string
		snaps             []trafficSnap
		wantNil           bool
		rps, errRate, p50 float64
		series            int
	}{
		{name: "no snapshots", wantNil: true},
		{name: "one snapshot", snaps: []trafficSnap{snap(0, 1, 0, 1, 0, 0)}, wantNil: true},
		{name: "same instant", snaps: []trafficSnap{snap(0, 1, 0, 1, 0, 0), snap(0, 5, 0, 5, 0, 0)}, wantNil: true},
		{name: "idle", snaps: []trafficSnap{snap(0, 10, 1, 10, 0, 0), snap(10, 10, 1, 10, 0, 0)}, series: 1},
		{name: "busy with errors", snaps: []trafficSnap{snap(0, 0, 0, 0, 0, 0), snap(10, 100, 25, 100, 0, 0)}, rps: 10, errRate: 0.25, p50: 5, series: 1},
		{name: "three points", snaps: []trafficSnap{snap(0, 0, 0, 0, 0, 0), snap(5, 10, 0, 10, 0, 0), snap(10, 40, 0, 40, 0, 0)}, rps: 4, p50: 5, series: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeTraffic(tc.snaps, testBounds)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("got nil")
			}
			if math.Abs(got.RequestsPerSec-tc.rps) > 1e-9 || math.Abs(got.ErrorRate-tc.errRate) > 1e-9 || math.Abs(got.P50Ms-tc.p50) > 1e-9 || len(got.RPSSeries) != tc.series {
				t.Errorf("got %+v, want rps=%v err=%v p50=%v series=%d", got, tc.rps, tc.errRate, tc.p50, tc.series)
			}
		})
	}
}

func TestBucketQuantile(t *testing.T) {
	tests := []struct {
		name   string
		counts []float64
		q      float64
		want   float64
	}{
		{"empty", []float64{0, 0, 0}, 0.5, 0},
		{"first bucket", []float64{10, 0, 0}, 0.5, 5},
		{"second bucket p95", []float64{0, 10, 0}, 0.95, 95.5},
		{"overflow reports last bound", []float64{0, 0, 4}, 0.95, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := bucketQuantile(tc.counts, testBounds, tc.q); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAppendSnapWindowAndReset(t *testing.T) {
	cutoff := time.Unix(1000, 0).Add(20 * time.Second)
	snaps := []trafficSnap{snap(0, 1, 0), snap(10, 2, 0), snap(25, 3, 0)}
	got := appendSnap(snaps, snap(30, 4, 0), cutoff)
	if len(got) != 3 || got[0].t.Requests != 2 {
		t.Errorf("window trim kept %d starting at %d, want 3 starting at 2", len(got), got[0].t.Requests)
	}
	got = appendSnap(got, snap(40, 1, 0), cutoff)
	if len(got) != 1 || got[0].t.Requests != 1 {
		t.Errorf("counter reset should drop history, got %+v", got)
	}
}

func TestRecordStatusFillsTraffic(t *testing.T) {
	reg := NewRegistryWithHistory(HistoryConfig{Checks: 5, Transitions: 5, SeriesStep: time.Second, SeriesWindow: time.Minute, MaxAge: time.Hour, TrafficWindow: time.Minute})
	now := time.Unix(1000, 0)
	reg.now = func() time.Time { return now }
	total := uint64(0)
	reg.SetTrafficSource(func() map[string]TrafficTotals {
		return map[string]TrafficTotals{"a:1": {Requests: total, Errors: total / 10, Latency: []uint64{total, 0, 0}}}
	}, testBounds)
	run := func() Status {
		st := Status{Service: "web", Upstreams: []UpstreamStatus{{ID: "u1", Dial: "a:1"}, {ID: "u2", Dial: "b:2"}}}
		reg.RecordStatus(&st)
		return st
	}
	if st := run(); st.Upstreams[0].Traffic != nil {
		t.Fatal("first sample must not report traffic")
	}
	now, total = now.Add(10*time.Second), 100
	st := run()
	if tr := st.Upstreams[0].Traffic; tr == nil || tr.RequestsPerSec != 10 || tr.ErrorRate != 0.1 {
		t.Errorf("u1 traffic = %+v", tr)
	}
	if st.Upstreams[1].Traffic != nil {
		t.Errorf("upstream with no ingress data must stay nil, got %+v", st.Upstreams[1].Traffic)
	}
}
