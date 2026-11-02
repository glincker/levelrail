package fixit

import (
	"context"
	"errors"
	"testing"
	"time"
)

type poolerHarness struct {
	eng     *Engine
	pooled  map[string]bool
	high    []string
	enabled []string
	failOn  string
	blocked string
}

func newPoolerHarness(t *testing.T, high ...string) *poolerHarness {
	t.Helper()
	h := &poolerHarness{pooled: map[string]bool{}, high: high}
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h.eng = NewEngine(&memStore{}, WithClock(func() time.Time { return clock }), WithMaxPerHour(3))
	d := DatabasePoolerDeps{
		Candidates: func(context.Context) ([]PoolerCandidate, error) {
			var out []PoolerCandidate
			for _, n := range h.high {
				if !h.pooled[n] {
					out = append(out, PoolerCandidate{Name: n, Detail: "92 of 100 connections in use"})
				}
			}
			return out, nil
		},
		Enable: func(_ context.Context, name string) error {
			if name == h.failOn {
				return errors.New("store down")
			}
			h.enabled = append(h.enabled, name)
			h.pooled[name] = true
			return nil
		},
		Blocked: func() string { return h.blocked },
	}
	if err := h.eng.Register(DatabasePoolerFix(d)); err != nil {
		t.Fatal(err)
	}
	return h
}

const poolerFixID = "database.connections.enable_pooler"

func TestPoolerFixHealthyWhenNoCandidates(t *testing.T) {
	h := newPoolerHarness(t)
	vs, err := h.eng.List(context.Background(), "databases", false)
	if err != nil || len(vs) != 0 {
		t.Fatalf("healthy fix must not be listed: %+v err=%v", vs, err)
	}
}

func TestPoolerFixPreviewApplyAndIdempotent(t *testing.T) {
	ctx := context.Background()
	h := newPoolerHarness(t, "orders", "billing")
	v, err := h.eng.Preview(ctx, poolerFixID)
	if err != nil || v.Kind != KindAuto || v.Risk != RiskRestarts || len(v.Changes) != 2 || len(h.enabled) != 0 {
		t.Fatalf("preview must plan without acting: %+v err=%v", v, err)
	}
	r, err := h.eng.Apply(ctx, poolerFixID, "root", nil)
	if err != nil || r.Outcome != OutcomeFixed || len(h.enabled) != 2 {
		t.Fatalf("apply: %+v err=%v enabled=%v", r, err, h.enabled)
	}
	r, _ = h.eng.Apply(ctx, poolerFixID, "root", nil)
	if r.Outcome != OutcomeSkipped || len(h.enabled) != 2 {
		t.Fatalf("second apply must be a no-op: %+v enabled=%v", r, h.enabled)
	}
}

func TestPoolerFixEnableErrorAndBlocked(t *testing.T) {
	ctx := context.Background()
	h := newPoolerHarness(t, "orders")
	h.failOn = "orders"
	r, _ := h.eng.Apply(ctx, poolerFixID, "root", nil)
	if r.Outcome == OutcomeFixed {
		t.Fatalf("a failed enable must not report fixed: %+v", r)
	}
	b := newPoolerHarness(t, "orders")
	b.blocked = "no pooler image is configured"
	v, _ := b.eng.Preview(ctx, poolerFixID)
	if v.Kind != KindManual || v.Reason == "" {
		t.Fatalf("blocked pooler must lower the tier: %+v", v)
	}
}
