package alerting

import (
	"context"
	"testing"
	"time"
)

type fakePoolerHealth struct {
	fakeDBHealth
	waiting int
	wait    float64
	err     error
}

func (f fakePoolerHealth) PoolerSaturation(context.Context, string) (int, float64, error) {
	return f.waiting, f.wait, f.err
}

func TestEvaluatePoolerSaturated(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		rule   Rule
		health DatabaseHealthSource
		fire   bool
	}{
		{"queued past default", Rule{Kind: KindPoolerSaturated}, fakePoolerHealth{waiting: 4, wait: 9}, true},
		{"queued under default", Rule{Kind: KindPoolerSaturated}, fakePoolerHealth{waiting: 4, wait: 1}, false},
		{"custom threshold", Rule{Kind: KindPoolerSaturated, Threshold: 30}, fakePoolerHealth{waiting: 4, wait: 9}, false},
		{"nobody waiting", Rule{Kind: KindPoolerSaturated}, fakePoolerHealth{waiting: 0, wait: 9}, false},
		{"no pooler", Rule{Kind: KindPoolerSaturated}, fakePoolerHealth{err: ErrDatabaseHealthUnavailable}, false},
		{"source cannot read pooler", Rule{Kind: KindPoolerSaturated}, fakeDBHealth{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, notice, err := EvaluateDatabaseRule(context.Background(), tc.health, nil, tc.rule, now)
			if err != nil {
				t.Fatal(err)
			}
			if out.Firing != tc.fire || (notice != "") != tc.fire {
				t.Fatalf("firing=%v notice=%q, want fire=%v", out.Firing, notice, tc.fire)
			}
		})
	}
}
