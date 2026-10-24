package cleanup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeRuns struct {
	mu   sync.Mutex
	rows []store.CleanupRun
}

func (f *fakeRuns) InsertCleanupRun(_ context.Context, r store.CleanupRun) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r.ID = int64(len(f.rows) + 1)
	f.rows = append(f.rows, r)
	return r.ID, nil
}

func (f *fakeRuns) ListCleanupRuns(_ context.Context, fl store.CleanupRunFilter) ([]store.CleanupRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.CleanupRun
	for i := len(f.rows) - 1; i >= 0; i-- {
		r := f.rows[i]
		if (fl.Target != "" && r.Target != fl.Target) || (fl.BeforeID > 0 && r.ID >= fl.BeforeID) {
			continue
		}
		out = append(out, r)
		if fl.Limit > 0 && len(out) == fl.Limit {
			break
		}
	}
	return out, nil
}

func (f *fakeRuns) LatestCleanupRuns(context.Context) ([]store.CleanupRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	latest := map[string]store.CleanupRun{}
	for _, r := range f.rows {
		latest[r.Target] = r
	}
	var out []store.CleanupRun
	for _, r := range latest {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeRuns) CleanupFailedSince(_ context.Context, target string, since time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.Target == target && r.Error != "" && !r.StartedAt.Before(since) {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRuns) PruneCleanupRuns(_ context.Context, before time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := f.rows[:0]
	var n int64
	for _, r := range f.rows {
		if r.StartedAt.Before(before) {
			n++
			continue
		}
		kept = append(kept, r)
	}
	f.rows = kept
	return n, nil
}

type fakeAudit struct {
	mu      sync.Mutex
	entries []store.AuditEntry
}

func (f *fakeAudit) SaveAuditEntry(_ context.Context, e store.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
	return nil
}

type fakeSettings struct {
	ints    map[string]int
	strs    map[string]string
	enabled bool
}

func (s fakeSettings) Int(_ context.Context, k string) int       { return s.ints[k] }
func (s fakeSettings) String(_ context.Context, k string) string { return s.strs[k] }
func (s fakeSettings) Bool(context.Context, string) bool         { return s.enabled }
func (s fakeSettings) Duration(context.Context, string) time.Duration {
	return time.Hour
}

type fakeTarget struct {
	meta     Meta
	runs     int
	previews int
	rows     int64
	err      error
	block    chan struct{}
	entered  chan struct{}
	gotDays  []int
}

func (f *fakeTarget) Meta() Meta { return f.meta }
func (f *fakeTarget) Measure(context.Context) (Usage, error) {
	return Usage{Rows: 10}, nil
}
func (f *fakeTarget) Preview(_ context.Context, days int) (Impact, error) {
	f.previews++
	f.gotDays = append(f.gotDays, days)
	return Impact{Rows: f.rows, Bytes: f.rows * 10}, nil
}
func (f *fakeTarget) Run(_ context.Context, days int, _ RunOptions) Result {
	f.runs++
	f.gotDays = append(f.gotDays, days)
	if f.block != nil {
		f.entered <- struct{}{}
		<-f.block
	}
	return Result{Rows: f.rows, Bytes: f.rows * 10, Err: f.err}
}

type harness struct {
	c      *Center
	runs   *fakeRuns
	audit  *fakeAudit
	events []string
	data   []map[string]any
	notes  []string
	now    time.Time
	set    fakeSettings
	reg    *Registry
}

func newHarness(t *testing.T, targets ...*fakeTarget) *harness {
	t.Helper()
	h := &harness{runs: &fakeRuns{}, audit: &fakeAudit{}, now: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		set: fakeSettings{ints: map[string]int{}, strs: map[string]string{}, enabled: true}, reg: NewRegistry(nil)}
	for _, tg := range targets {
		h.reg.Register(tg)
	}
	h.c = New(Config{
		Registry: h.reg, Store: h.runs, Settings: &h.set, Audit: h.audit,
		Emit:   func(typ string, d map[string]any) { h.events = append(h.events, typ); h.data = append(h.data, d) },
		Notify: func(_ context.Context, text string) error { h.notes = append(h.notes, text); return nil },
		Limits: Limits{MaxRows: 1000, Slice: time.Hour, Stagger: 0, StartupDelay: time.Hour, HistoryDays: 365, UsageTTL: time.Minute},
		Now:    func() time.Time { return h.now },
	})
	return h
}

func tgt(id string, origin Origin, floor int) *fakeTarget {
	return &fakeTarget{meta: Meta{ID: id, Category: CategoryOther, Title: id, PolicySetting: "retention." + id, Origin: origin, Floor: floor}, rows: 7}
}

func intp(n int) *int { return &n }

func TestModeDefaultsByOrigin(t *testing.T) {
	var defs []runtimesettings.Definition
	reg := NewRegistry(func(d runtimesettings.Definition) { defs = append(defs, d) })
	reg.Register(tgt("old.one", OriginExisting, 0))
	reg.Register(tgt("new.one", OriginNew, 0))
	ro := tgt("managed.one", OriginExisting, 0)
	ro.meta.ReadOnly, ro.meta.PolicySetting = true, ""
	reg.Register(ro)
	want := map[string]string{"cleanup.old.one.mode": "on", "cleanup.new.one.mode": "preview"}
	if len(defs) != len(want) {
		t.Fatalf("registered %d mode settings, want %d (read-only targets have none)", len(defs), len(want))
	}
	svc := runtimesettings.NewWithDefinitions(nil, func(string) (string, bool) { return "", false }, defs)
	for key, mode := range want {
		if got := svc.String(t.Context(), key); got != mode {
			t.Errorf("%s defaults to %q, want %q", key, got, mode)
		}
	}
	if _, err := runtimesettings.ParseJSON(defs[0], json.RawMessage(`"sometimes"`)); err == nil {
		t.Error("an unknown mode was accepted")
	}
}

func TestRegisterRejectsBadTargets(t *testing.T) {
	reg := NewRegistry(nil)
	reg.Register(tgt("a.b", OriginNew, 0))
	for name, bad := range map[string]Target{
		"duplicate":  tgt("a.b", OriginNew, 0),
		"no origin":  tgt("c.d", "", 0),
		"no policy":  func() Target { x := tgt("e.f", OriginNew, 0); x.meta.PolicySetting = ""; return x }(),
		"empty id":   tgt("", OriginNew, 0),
		"no title":   func() Target { x := tgt("g.h", OriginNew, 0); x.meta.Title = ""; return x }(),
		"no catgory": func() Target { x := tgt("i.j", OriginNew, 0); x.meta.Category = ""; return x }(),
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: registration did not panic", name)
				}
			}()
			reg.Register(bad)
		}()
	}
	if _, ok := reg.Get("a.b"); !ok || len(reg.Targets()) != 1 {
		t.Fatal("registry lost the valid target")
	}
}

func TestDryRunNeverWrites(t *testing.T) {
	tg := tgt("x.y", OriginNew, 0)
	h := newHarness(t, tg)
	h.set.ints["retention.x.y"] = 30
	h.c = New(Config{Registry: h.reg, Store: h.runs, Settings: &h.set, Audit: h.audit, Now: func() time.Time { return h.now }, Limits: h.c.cfg.Limits})
	rec, res, err := h.c.Run(t.Context(), "x.y", Request{DryRun: true, Opts: RunOptions{Trigger: TriggerAPI, Actor: "root"}})
	if err != nil || res.Err != nil {
		t.Fatalf("dry run failed: %v %v", err, res.Err)
	}
	if tg.runs != 0 || tg.previews != 1 {
		t.Fatalf("Run called %d times, Preview %d; a dry run must only preview", tg.runs, tg.previews)
	}
	if !rec.DryRun || rec.Rows != 7 || rec.Days != 30 {
		t.Fatalf("record = %+v", rec)
	}
	var d auditDetail
	if len(h.audit.entries) != 1 || json.Unmarshal([]byte(h.audit.entries[0].Detail), &d) != nil || !d.DryRun || d.Target != "x.y" {
		t.Fatalf("a dry run must still be audited: %+v", h.audit.entries)
	}
}

func TestAuditRowHoldsCountsOnly(t *testing.T) {
	tg := tgt("a.b", OriginExisting, 0)
	tg.err = errors.New("disk is full")
	h := newHarness(t, tg)
	h.set.ints["retention.a.b"] = 90
	if _, _, err := h.c.Run(t.Context(), "a.b", Request{Opts: RunOptions{Trigger: TriggerManual, Actor: "alice", ActorType: "session", ActorID: "u1"}}); err != nil {
		t.Fatal(err)
	}
	e := h.audit.entries[0]
	if e.Action != AuditActionRun || e.ActorName != "alice" || e.ActorType != "session" || e.Ability != "root" {
		t.Fatalf("audit entry = %+v", e)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(e.Detail), &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"target", "days", "dry_run", "rows", "bytes", "trigger", "error"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("detail lacks %q", k)
		}
	}
	if len(raw) != 7 {
		t.Errorf("detail has %d keys, want exactly the seven count fields: %v", len(raw), raw)
	}
}

func TestEventsAndOneFailureNoticePerDay(t *testing.T) {
	tg := tgt("a.b", OriginExisting, 0)
	h := newHarness(t, tg)
	h.set.ints["retention.a.b"] = 90
	run := func() { _, _, _ = h.c.Run(t.Context(), "a.b", Request{Opts: RunOptions{Trigger: TriggerCLI}}) }

	run()
	if len(h.events) != 1 || h.events[0] != EventRun || h.data[0]["trigger"] != "cli" {
		t.Fatalf("events after success = %v %v", h.events, h.data)
	}
	tg.err = errors.New("boom")
	run()
	run()
	if got := strings.Join(h.events, ","); got != "cleanup.run,cleanup.failed,cleanup.failed" {
		t.Fatalf("events = %s", got)
	}
	if len(h.notes) != 1 || !strings.Contains(h.notes[0], "boom") {
		t.Fatalf("notices = %v, want exactly one for the day", h.notes)
	}
	h.now = h.now.Add(25 * time.Hour)
	run()
	if len(h.notes) != 2 {
		t.Fatalf("a failure on the next day must notify again: %v", h.notes)
	}
}

func TestFloorsAreEnforced(t *testing.T) {
	tg := tgt("audit.x", OriginExisting, 90)
	h := newHarness(t, tg)
	h.set.ints["retention.audit.x"] = 30
	tests := []struct {
		name string
		days *int
		want error
	}{
		{"below floor", intp(60), &FloorError{}},
		{"at floor", intp(90), nil},
		{"zero override", intp(0), ErrNoRetention},
		{"negative", intp(-1), ErrInvalidDays},
	}
	for _, tc := range tests {
		_, _, err := h.c.Run(t.Context(), "audit.x", Request{Days: tc.days})
		var fe *FloorError
		switch {
		case tc.want == nil && err != nil, tc.want != nil && err == nil:
			t.Errorf("%s: err = %v", tc.name, err)
		case errors.As(tc.want, &fe) && !errors.As(err, &fe):
			t.Errorf("%s: err = %v, want a floor error", tc.name, err)
		case fe != nil && fe.Key != "retention.audit.x":
			t.Errorf("%s: floor error names %q", tc.name, fe.Key)
		}
	}
	if _, err := h.c.Preview(t.Context(), "audit.x", intp(10)); err == nil {
		t.Error("preview accepted a window below the floor")
	}
	if tg.runs != 1 {
		t.Fatalf("only the at-floor run may reach the target, got %d", tg.runs)
	}

	tg.gotDays = nil
	h.set.strs["cleanup.audit.x.mode"] = "on"
	h.c.runScheduled(t.Context(), tg)
	if tg.runs != 2 || tg.gotDays[len(tg.gotDays)-1] != 90 {
		t.Fatalf("a scheduled run with a saved 30 must clamp to the floor, days = %v", tg.gotDays)
	}
}

func TestSingleFlight(t *testing.T) {
	tg := tgt("a.b", OriginExisting, 0)
	tg.block, tg.entered = make(chan struct{}), make(chan struct{}, 2)
	h := newHarness(t, tg)
	h.set.ints["retention.a.b"] = 30
	done := make(chan error, 1)
	go func() {
		_, _, err := h.c.Run(t.Context(), "a.b", Request{})
		done <- err
	}()
	<-tg.entered
	if _, _, err := h.c.Run(t.Context(), "a.b", Request{}); !errors.Is(err, ErrBusy) {
		t.Fatalf("second run err = %v, want ErrBusy", err)
	}
	close(tg.block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.c.Run(t.Context(), "a.b", Request{}); err != nil {
		t.Fatalf("the target stayed locked after the first run: %v", err)
	}
}

func TestReadOnlyAndUnknownTargets(t *testing.T) {
	ro := tgt("docker.x", OriginExisting, 0)
	ro.meta.ReadOnly, ro.meta.PolicySetting = true, ""
	h := newHarness(t, ro)
	if _, _, err := h.c.Run(t.Context(), "docker.x", Request{}); !errors.Is(err, ErrReadOnly) {
		t.Errorf("read-only run err = %v", err)
	}
	if _, _, err := h.c.Run(t.Context(), "nope", Request{}); !errors.Is(err, ErrUnknownTarget) {
		t.Errorf("unknown run err = %v", err)
	}
	if _, err := h.c.Preview(t.Context(), "nope", nil); !errors.Is(err, ErrUnknownTarget) {
		t.Errorf("unknown preview err = %v", err)
	}
	if _, err := h.c.Get(t.Context(), "nope"); !errors.Is(err, ErrUnknownTarget) {
		t.Errorf("unknown get err = %v", err)
	}
}

func TestTickHonoursModes(t *testing.T) {
	on, prev, off, forever := tgt("t.on", OriginExisting, 0), tgt("t.prev", OriginNew, 0), tgt("t.off", OriginExisting, 0), tgt("t.forever", OriginExisting, 0)
	h := newHarness(t, on, prev, off, forever)
	for _, id := range []string{"t.on", "t.prev", "t.off"} {
		h.set.ints["retention."+id] = 30
	}
	h.set.strs["cleanup.t.off.mode"] = "off"
	h.set.strs["cleanup.t.prev.mode"] = "preview"
	h.c.Tick(t.Context())
	if on.runs != 1 || on.previews != 0 {
		t.Errorf("on target: runs %d previews %d", on.runs, on.previews)
	}
	if prev.runs != 0 || prev.previews != 1 {
		t.Errorf("preview target must only be previewed: runs %d previews %d", prev.runs, prev.previews)
	}
	if off.runs+off.previews != 0 {
		t.Error("an off target was touched")
	}
	if forever.runs+forever.previews != 0 {
		t.Error("a keep-forever target was touched")
	}
	if len(h.runs.rows) != 2 || h.runs.rows[0].Trigger != "scheduled" || !h.runs.rows[1].DryRun {
		t.Fatalf("history = %+v", h.runs.rows)
	}
	h.set.enabled = false
	h.c.Tick(t.Context())
	if on.runs != 1 {
		t.Error("a disabled scheduler still ran")
	}
}

func TestStartWaitsAndStopsOnCancel(t *testing.T) {
	tg := tgt("a.b", OriginExisting, 0)
	h := newHarness(t, tg)
	h.set.ints["retention.a.b"] = 30
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { h.c.Start(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the scheduler ignored cancellation")
	}
	if tg.runs+tg.previews != 0 {
		t.Fatal("the scheduler ran inside the startup delay")
	}
}

func TestSweepByCutoffBatchesAndCaps(t *testing.T) {
	oldest := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cutoff := oldest.Add(10 * time.Hour)
	var calls []time.Time
	del := func(_ context.Context, c time.Time) (int64, error) { calls = append(calls, c); return 5, nil }

	rows, more, err := SweepByCutoff(t.Context(), oldest, cutoff, 3*time.Hour, 0, del)
	if err != nil || more || rows != 20 || len(calls) != 4 {
		t.Fatalf("full sweep: rows %d more %v calls %d err %v", rows, more, len(calls), err)
	}
	for _, c := range calls {
		if c.After(cutoff) {
			t.Fatalf("a batch reached past the cutoff: %v", c)
		}
	}
	if !calls[len(calls)-1].Equal(cutoff) {
		t.Fatal("the last batch must end exactly at the cutoff")
	}

	calls = nil
	rows, more, err = SweepByCutoff(t.Context(), oldest, cutoff, 3*time.Hour, 10, del)
	if err != nil || !more || rows != 10 || len(calls) != 2 {
		t.Fatalf("capped sweep: rows %d more %v calls %d err %v", rows, more, len(calls), err)
	}

	calls = nil
	if rows, more, _ := SweepByCutoff(t.Context(), cutoff, cutoff, time.Hour, 0, del); rows != 0 || more || len(calls) != 0 {
		t.Fatal("nothing older than the cutoff must mean no delete")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, more, err := SweepByCutoff(ctx, oldest, cutoff, time.Hour, 0, del); err == nil || !more {
		t.Fatalf("a cancelled sweep must stop with an error: more %v err %v", more, err)
	}

	boom := errors.New("boom")
	if rows, _, err := SweepByCutoff(t.Context(), oldest, cutoff, time.Hour, 0, func(context.Context, time.Time) (int64, error) { return 2, boom }); !errors.Is(err, boom) || rows != 2 {
		t.Fatalf("delete error: rows %d err %v", rows, err)
	}
}

func TestRunsPagingAndOverview(t *testing.T) {
	tg := tgt("a.b", OriginExisting, 0)
	h := newHarness(t, tg)
	h.set.ints["retention.a.b"] = 30
	for range 5 {
		if _, _, err := h.c.Run(t.Context(), "a.b", Request{}); err != nil {
			t.Fatal(err)
		}
	}
	p1, err := h.c.Runs(t.Context(), RunsFilter{Limit: 2})
	if err != nil || len(p1.Runs) != 2 || p1.NextCursor == 0 {
		t.Fatalf("page 1 = %+v, %v", p1, err)
	}
	p3, _ := h.c.Runs(t.Context(), RunsFilter{Limit: 2, Cursor: h.c.mustCursor(t, 2)})
	if len(p3.Runs) != 2 {
		t.Fatalf("page 2 = %+v", p3)
	}
	last, _ := h.c.Runs(t.Context(), RunsFilter{Limit: 5})
	if len(last.Runs) != 5 || last.NextCursor != 0 {
		t.Fatalf("last page = %+v", last)
	}

	ov, err := h.c.List(t.Context())
	if err != nil || len(ov.Targets) != 1 {
		t.Fatalf("overview = %+v, %v", ov, err)
	}
	v := ov.Targets[0]
	if v.Mode != "on" || v.Days != 30 || v.LastRun == nil || v.LastRun.ID != 5 || v.Usage.Rows != 10 || v.Origin != "existing" || len(v.Presets) == 0 {
		t.Fatalf("view = %+v", v)
	}
	if ov.Scheduler.Enabled {
		t.Error("the scheduler reports enabled before it started")
	}
	body, _ := json.Marshal(v)
	for _, k := range []string{`"last_run"`, `"next_run"`, `"policy_key"`, `"usage"`, `"floor"`, `"destructive"`} {
		if !strings.Contains(string(body), k) {
			t.Errorf("target JSON lacks %s: %s", k, body)
		}
	}
}

func (c *Center) mustCursor(t *testing.T, skip int) int64 {
	t.Helper()
	page, err := c.Runs(t.Context(), RunsFilter{Limit: skip})
	if err != nil {
		t.Fatal(err)
	}
	return page.NextCursor
}

func TestHistoryPrunedByTick(t *testing.T) {
	tg := tgt("a.b", OriginExisting, 0)
	h := newHarness(t, tg)
	h.runs.rows = []store.CleanupRun{{ID: 1, Target: "a.b", StartedAt: h.now.Add(-400 * 24 * time.Hour)}, {ID: 2, Target: "a.b", StartedAt: h.now.Add(-time.Hour)}}
	h.c.Tick(t.Context())
	for _, r := range h.runs.rows {
		if r.ID == 1 {
			t.Fatal("a run older than a year survived the tick")
		}
	}
}

func TestWriteRunsCSV(t *testing.T) {
	var buf bytes.Buffer
	err := WriteRunsCSV(&buf, []RunRecord{{ID: 3, Target: "a.b", StartedAt: time.Unix(0, 0), FinishedAt: time.Unix(5, 0), Days: 30, Rows: 9, Trigger: "api", Actor: "=cmd()", Error: "x, y"}})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 || lines[0] != strings.Join(RunsCSVHeader, ",") {
		t.Fatalf("csv = %q", buf.String())
	}
	if !strings.Contains(lines[1], `'=cmd()`) || !strings.Contains(lines[1], `"x, y"`) {
		t.Fatalf("row = %q", lines[1])
	}
}

func TestAuditWindows(t *testing.T) {
	tests := []struct{ g, s, wantG, wantS int }{
		{90, 365, 90, 365},
		{30, 365, 90, 365},
		{90, 30, 90, 365},
		{730, 365, 730, 730},
		{90, 0, 90, 0},
		{0, 365, 0, 365},
	}
	for _, tc := range tests {
		g, s := AuditWindows(tc.g, tc.s)
		if g != tc.wantG || s != tc.wantS {
			t.Errorf("AuditWindows(%d,%d) = %d,%d want %d,%d", tc.g, tc.s, g, s, tc.wantG, tc.wantS)
		}
	}
}

func TestLimitsFromEnv(t *testing.T) {
	env := map[string]string{EnvMaxRows: "50", EnvSlice: "6h", EnvStagger: "0s", EnvStartupDelay: "bad", EnvHistoryDays: "-3"}
	l := LimitsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if l.MaxRows != 50 || l.Slice != 6*time.Hour || l.Stagger != 0 || l.StartupDelay != 5*time.Minute || l.HistoryDays != 365 {
		t.Fatalf("limits = %+v", l)
	}
}
