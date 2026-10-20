package fixit

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memStore struct{ rows []Attempt }

func (m *memStore) RecentAttempts(_ context.Context, id, _ string, limit int) ([]Attempt, error) {
	var out []Attempt
	for i := len(m.rows) - 1; i >= 0 && len(out) < limit; i-- {
		if m.rows[i].FixID == id {
			out = append(out, m.rows[i])
		}
	}
	return out, nil
}

func (m *memStore) CountAttemptsSince(_ context.Context, id string, since time.Time) (int, error) {
	n := 0
	for _, r := range m.rows {
		if r.FixID == id && !r.At.Before(since) {
			n++
		}
	}
	return n, nil
}

func (m *memStore) RecordAttempt(_ context.Context, a Attempt) error {
	m.rows = append(m.rows, a)
	return nil
}

type harness struct {
	eng     *Engine
	st      *memStore
	failing bool
	detail  string
	runErr  error
	runs    int
	clock   time.Time
}

func newHarness(t *testing.T, kind Kind, command string) *harness {
	t.Helper()
	h := &harness{st: &memStore{}, failing: true, detail: "blocked A", clock: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	h.eng = NewEngine(h.st, WithClock(func() time.Time { return h.clock }), WithMaxPerHour(3))
	f := Fix{ID: "x", CheckID: "c", Subject: "s", Kind: kind,
		Detect: func(context.Context) (Applicability, error) {
			return Applicability{Failing: h.failing, Detail: h.detail, Command: command}, nil
		}}
	if kind == KindAuto {
		f.Run = func(context.Context, Input) error {
			h.runs++
			if h.runErr == nil && h.detail == "blocked A" {
				h.failing = false
			}
			return h.runErr
		}
	}
	if err := h.eng.Register(f); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestApplyFixed(t *testing.T) {
	h := newHarness(t, KindAuto, "cmd")
	r, err := h.eng.Apply(context.Background(), "x", "root", nil)
	if err != nil || r.Outcome != OutcomeFixed || h.runs != 1 {
		t.Fatalf("got %+v err=%v runs=%d", r, err, h.runs)
	}
	r, _ = h.eng.Apply(context.Background(), "x", "root", nil)
	if r.Outcome != OutcomeSkipped || h.runs != 1 {
		t.Fatalf("idempotent apply must skip, got %+v runs=%d", r, h.runs)
	}
}

func TestDemotionAndRearm(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, KindAuto, "cmd")
	h.runErr = errors.New("boom")
	r, _ := h.eng.Apply(ctx, "x", "root", nil)
	if r.Outcome != OutcomeStillFailing || r.View.Kind != KindGuided || r.View.State != StateDemoted || r.View.Blocked == "" {
		t.Fatalf("first failure must demote to guided: %+v", r.View)
	}
	r, _ = h.eng.Apply(ctx, "x", "root", nil)
	if h.runs != 1 {
		t.Fatalf("demoted fix must not run again, runs=%d", h.runs)
	}
	if r.View.Kind != KindManual {
		t.Fatalf("second failed attempt must demote to manual, got %s", r.View.Kind)
	}
	h.detail = "blocked B"
	v, _ := h.eng.Preview(ctx, "x")
	if v.Kind != KindAuto || v.State != StateOffer {
		t.Fatalf("changed fingerprint must re-arm: %+v", v)
	}
}

func TestDemoteWithoutCommandGoesManual(t *testing.T) {
	h := newHarness(t, KindAuto, "")
	h.runErr = errors.New("boom")
	r, _ := h.eng.Apply(context.Background(), "x", "root", nil)
	if r.View.Kind != KindManual {
		t.Fatalf("no command means manual, got %s", r.View.Kind)
	}
}

func TestCannotFixHereOutcome(t *testing.T) {
	h := newHarness(t, KindAuto, "cmd")
	h.runErr = ErrCannotFixHere
	r, _ := h.eng.Apply(context.Background(), "x", "root", nil)
	if r.Outcome != OutcomeCannotFixHere {
		t.Fatalf("got %s", r.Outcome)
	}
}

func TestRateLimit(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, KindAuto, "cmd")
	for i := 0; i < 3; i++ {
		h.st.rows = append(h.st.rows, Attempt{FixID: "x", Subject: "s", At: h.clock.Add(-time.Minute), Outcome: OutcomeFixed, Fingerprint: "other"})
	}
	r, _ := h.eng.Apply(ctx, "x", "root", nil)
	if r.Outcome != OutcomeSkipped || h.runs != 0 || r.View.State != StateRateLimited {
		t.Fatalf("rate limited expected: %+v runs=%d", r, h.runs)
	}
	h.clock = h.clock.Add(2 * time.Hour)
	r, _ = h.eng.Apply(ctx, "x", "root", nil)
	if r.Outcome != OutcomeFixed {
		t.Fatalf("window should expire: %+v", r)
	}
}

func TestGuidedRecheckNeverRuns(t *testing.T) {
	h := newHarness(t, KindGuided, "cmd")
	r, _ := h.eng.Apply(context.Background(), "x", "root", nil)
	if r.Outcome != OutcomeStillFailing || r.View.Kind != KindManual {
		t.Fatalf("guided recheck failing demotes to manual: %+v", r)
	}
	h.failing = false
	r, _ = h.eng.Apply(context.Background(), "x", "root", nil)
	if r.Outcome != OutcomeSkipped {
		t.Fatalf("healthy must skip: %+v", r)
	}
}

func TestListHidesHealthy(t *testing.T) {
	h := newHarness(t, KindAuto, "cmd")
	h.failing = false
	vs, _ := h.eng.List(context.Background(), "", false)
	if len(vs) != 0 {
		t.Fatalf("healthy hidden, got %d", len(vs))
	}
	vs, _ = h.eng.List(context.Background(), "s", true)
	if len(vs) != 1 || vs[0].State != StateHealthy {
		t.Fatalf("includeHealthy: %+v", vs)
	}
}

func TestFixDetectors(t *testing.T) {
	ctx := context.Background()
	fail := func(context.Context) (CheckState, error) { return CheckState{Failing: true, Detail: "blocked"}, nil }
	cases := []struct {
		name    string
		host    HostEnv
		wantCmd string
		manual  bool
	}{
		{"systemd", HostEnv{Mode: RunSystemd, Unit: "lr", TunPresent: true}, "AmbientCapabilities=CAP_NET_ADMIN", false},
		{"docker", HostEnv{Mode: RunDocker, TunPresent: true}, "--cap-add NET_ADMIN", false},
		{"dev", HostEnv{Mode: RunDev, Executable: "/x/lr"}, "sudo /x/lr", false},
		{"root", HostEnv{Mode: RunDev, Root: true}, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := WireGuardPrivilegeFix(WireGuardDeps{Check: fail, Host: func() HostEnv { return c.host }})
			a, err := f.Detect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if c.manual != (a.Kind == KindManual) || (c.wantCmd != "" && !contains(a.Command, c.wantCmd)) {
				t.Fatalf("%+v", a)
			}
		})
	}
	disk := DiskFix(DiskDeps{
		Free:            func(context.Context) (int64, int64, error) { return 4, 100, nil },
		Reclaimable:     func(context.Context) (DiskReclaim, error) { return DiskReclaim{ImagesBytes: 10}, nil },
		WarnFreePercent: 10,
	})
	a, _ := disk.Detect(ctx)
	if !a.Failing || len(a.Changes) != 1 || a.Changes[0].Bytes != 10 {
		t.Fatalf("disk: %+v", a)
	}
	if err := ValidAdvertiseHost("127.0.0.1"); err == nil {
		t.Fatal("loopback must be rejected")
	}
	if err := ValidAdvertiseHost("a.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := ValidAdvertiseHost("http://a.com"); err == nil {
		t.Fatal("url must be rejected")
	}
}

func TestCapNetAdmin(t *testing.T) {
	if !capNetAdmin("Name:\tx\nCapEff:\t0000000000001000\n") || capNetAdmin("CapEff:\t0000000000000000\n") {
		t.Fatal("cap parse")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
