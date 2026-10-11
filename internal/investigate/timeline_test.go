package investigate

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/changes"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

var t0 = time.Date(2026, 10, 3, 3, 0, 0, 0, time.UTC)

func at(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }

func TestMergeOrdering(t *testing.T) {
	deploy := Event{At: at(5), Kind: "deploy", Title: "deploy"}
	cases := []struct {
		name  string
		lists [][]Event
		want  []string
	}{
		{
			name: "sorted ascending across sources",
			lists: [][]Event{
				{{At: at(9), Kind: KindRestart, Title: "restart"}},
				{deploy},
				{{At: at(7), Kind: KindSaturation, Title: "cpu"}},
			},
			want: []string{"deploy", "cpu", "restart"},
		},
		{
			name: "tie breaks cause before symptom",
			lists: [][]Event{
				{{At: at(5), Kind: KindRestart, Title: "restart"}},
				{{At: at(5), Kind: KindSaturation, Title: "cpu"}},
				{deploy},
			},
			want: []string{"deploy", "cpu", "restart"},
		},
		{name: "empty is non-nil", lists: nil, want: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Merge(tc.lists...)
			if got == nil || len(got) != len(tc.want) {
				t.Fatalf("got %+v", got)
			}
			for i, e := range got {
				if e.Title != tc.want[i] {
					t.Errorf("position %d = %q, want %q", i, e.Title, tc.want[i])
				}
			}
		})
	}
}

func TestRestartEventsCollapseBursts(t *testing.T) {
	samples := []telemetry.Sample{
		{Timestamp: at(10)}, {Timestamp: at(10).Add(20 * time.Second)}, {Timestamp: at(10).Add(40 * time.Second)},
		{Timestamp: at(40)},
	}
	got := RestartEvents(samples, 2*time.Minute)
	if len(got) != 2 {
		t.Fatalf("events = %+v", got)
	}
	if got[0].Title != "Container restarted 3 times" || got[0].Severity != SeverityCritical {
		t.Errorf("burst = %+v", got[0])
	}
	if got[1].Title != "Container restarted" || got[1].Severity != SeverityWarning {
		t.Errorf("single = %+v", got[1])
	}
	if RestartEvents(nil, time.Minute) != nil {
		t.Error("no samples should yield no events")
	}
}

func TestSaturationEvents(t *testing.T) {
	mk := func(vals ...float64) []telemetry.Sample {
		out := make([]telemetry.Sample, len(vals))
		for i, v := range vals {
			out[i] = telemetry.Sample{Timestamp: t0.Add(time.Duration(i) * 15 * time.Second), Value: v}
		}
		return out
	}
	th := Thresholds{CPUPercent: 90, MemoryPercent: 90, MinSamples: 2}
	cases := []struct {
		name            string
		cpu, use, limit []telemetry.Sample
		want            []string
	}{
		{"sustained cpu", mk(10, 95, 99, 20), nil, nil, []string{"CPU at 99% of its limit"}},
		{"single blip ignored", mk(10, 99, 10), nil, nil, nil},
		{"memory over limit ratio", nil, mk(950, 960, 100), mk(1000, 1000, 1000), []string{"Memory at 96% of its limit"}},
		{"no limit skips memory", nil, mk(950, 960), nil, nil},
		{"two runs", mk(95, 95, 10, 97, 97), nil, nil, []string{"CPU at 95% of its limit", "CPU at 97% of its limit"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SaturationEvents(tc.cpu, tc.use, tc.limit, th)
			if len(got) != len(tc.want) {
				t.Fatalf("events = %+v, want %v", got, tc.want)
			}
			for i, e := range got {
				if e.Title != tc.want[i] {
					t.Errorf("title = %q, want %q", e.Title, tc.want[i])
				}
			}
		})
	}
}

func TestFromChangesKeepsLikelyCause(t *testing.T) {
	got := FromChanges([]changes.Change{
		{At: at(1), Kind: changes.KindDeploy, Title: "Deployed img:2", LikelyCause: true},
		{At: at(2), Kind: changes.KindRollback, Title: "Rolled back"},
	})
	if !got[0].LikelyCause || got[1].Severity != SeverityWarning {
		t.Errorf("events = %+v", got)
	}
}

func TestWindowClamps(t *testing.T) {
	events := []Event{{At: at(1)}, {At: at(5)}, {At: at(9)}}
	got := Window(events, at(2), at(8))
	if len(got) != 1 || !got[0].At.Equal(at(5)) {
		t.Errorf("window = %+v", got)
	}
}
