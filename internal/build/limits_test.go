package build

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []chan time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func (c *fakeClock) After(time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	c.timers = append(c.timers, ch)
	return ch
}

func (c *fakeClock) fireAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ch := range c.timers {
		ch <- c.now
	}
}

type memRecorder struct {
	mu   sync.Mutex
	runs []Run
	err  error
}

func (m *memRecorder) RecordBuildRun(_ context.Context, r Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs = append(m.runs, r)
	return m.err
}

type fakeBuilder struct {
	fn func(ctx context.Context, p func(ProgressEvent)) (*Result, error)
}

func (f fakeBuilder) Build(ctx context.Context, _ Request, p func(ProgressEvent)) (*Result, error) {
	return f.fn(ctx, p)
}

func (f fakeBuilder) BuildRailpack(ctx context.Context, _ RailpackRequest, p func(ProgressEvent)) (*Result, error) {
	return f.fn(ctx, p)
}

func fixed(l Limits) LimitsSource { return func(context.Context, string) Limits { return l } }

func TestLimitsFromEnv(t *testing.T) {
	env := func(m map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}
	tests := []struct {
		name string
		env  map[string]string
		want Limits
	}{
		{"defaults", nil, Limits{MaxConcurrent: DefaultBuildMaxConcurrent, Timeout: DefaultBuildTimeout}},
		{"overrides", map[string]string{EnvBuildTimeout: "90s", EnvBuildMaxConcurrent: "5"}, Limits{MaxConcurrent: 5, Timeout: 90 * time.Second}},
		{"zero disables", map[string]string{EnvBuildTimeout: "0s", EnvBuildMaxConcurrent: "0"}, Limits{}},
		{"resources", map[string]string{EnvBuildMemoryMB: "2048", EnvBuildCPUMillis: "1500"}, Limits{MaxConcurrent: DefaultBuildMaxConcurrent, Timeout: DefaultBuildTimeout, MemoryMB: 2048, CPUMillis: 1500}},
		{"malformed falls back", map[string]string{EnvBuildTimeout: "soon", EnvBuildMaxConcurrent: "-2"}, Limits{MaxConcurrent: DefaultBuildMaxConcurrent, Timeout: DefaultBuildTimeout}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := LimitsFromEnv(env(tc.env)); got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestLimiterTimeoutStopsBuildAndRecordsPartialSteps(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	rec := &memRecorder{}
	started := make(chan struct{})
	b := fakeBuilder{fn: func(ctx context.Context, p func(ProgressEvent)) (*Result, error) {
		p(ProgressEvent{Step: "base", Completed: true, Cached: true})
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	l := NewLimiter(b, fixed(Limits{MaxConcurrent: 1, Timeout: time.Minute}), WithLimiterClock(clock), WithRunRecorder(rec))

	errc := make(chan error, 1)
	go func() { _, err := l.Build(context.Background(), Request{Tag: "reg/web:abc"}, nil); errc <- err }()
	<-started
	clock.advance(time.Minute)
	clock.fireAll()
	err := <-errc

	if !errors.Is(err, ErrBuildTimeout) {
		t.Fatalf("err = %v, want build timeout", err)
	}
	if len(rec.runs) != 1 {
		t.Fatalf("runs = %d", len(rec.runs))
	}
	r := rec.runs[0]
	if r.Status != RunTimeout || r.App != "web" || r.Duration != time.Minute || r.CachedSteps != 1 || r.TotalSteps != 1 {
		t.Fatalf("run = %+v", r)
	}
}

func TestLimiterRecorderFailureKeepsSuccessfulBuild(t *testing.T) {
	rec := &memRecorder{err: errors.New("disk full")}
	want := &Result{Tag: "web:abc"}
	b := fakeBuilder{fn: func(context.Context, func(ProgressEvent)) (*Result, error) { return want, nil }}
	l := NewLimiter(b, fixed(Limits{}), WithRunRecorder(rec))
	got, err := l.Build(context.Background(), Request{Tag: "web:abc"}, nil)
	if err != nil || got != want {
		t.Fatalf("got %v, %v", got, err)
	}
	if len(rec.runs) != 1 || rec.runs[0].Status != RunSuccess {
		t.Fatalf("runs = %+v", rec.runs)
	}
}

func TestLimiterBuildFinishingAsTimerFiresIsASuccess(t *testing.T) {
	clock := &fakeClock{now: time.Unix(0, 0)}
	rec := &memRecorder{}
	b := fakeBuilder{fn: func(context.Context, func(ProgressEvent)) (*Result, error) {
		clock.fireAll()
		return &Result{Tag: "web:abc"}, nil
	}}
	l := NewLimiter(b, fixed(Limits{Timeout: time.Minute}), WithLimiterClock(clock), WithRunRecorder(rec))
	if _, err := l.Build(context.Background(), Request{Tag: "web:abc"}, nil); err != nil {
		t.Fatal(err)
	}
	if rec.runs[0].Status != RunSuccess {
		t.Fatalf("status = %s", rec.runs[0].Status)
	}
}

func TestLimiterQueuesPastConcurrencyLimit(t *testing.T) {
	clock := &fakeClock{now: time.Unix(0, 0)}
	rec := &memRecorder{}
	release := make(chan struct{})
	entered := make(chan string, 3)
	b := fakeBuilder{fn: func(ctx context.Context, _ func(ProgressEvent)) (*Result, error) {
		entered <- "in"
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return &Result{}, nil
	}}
	l := NewLimiter(b, fixed(Limits{MaxConcurrent: 1}), WithLimiterClock(clock), WithRunRecorder(rec))

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = l.Build(context.Background(), Request{Tag: "web:a"}, nil) }()
	}
	<-entered
	waitFor(t, func() bool { w, r := l.Queued(); return w == 1 && r == 1 })
	release <- struct{}{}
	<-entered
	release <- struct{}{}
	wg.Wait()
	if w, r := l.Queued(); w != 0 || r != 0 {
		t.Fatalf("leaked slots: waiting=%d running=%d", w, r)
	}
}

func TestLimiterCanceledWhileQueuedNeverRunsAndFreesNothing(t *testing.T) {
	block := make(chan struct{})
	entered := make(chan struct{}, 2)
	b := fakeBuilder{fn: func(_ context.Context, _ func(ProgressEvent)) (*Result, error) {
		entered <- struct{}{}
		<-block
		return &Result{}, nil
	}}
	rec := &memRecorder{}
	l := NewLimiter(b, fixed(Limits{MaxConcurrent: 1}), WithRunRecorder(rec))
	done := make(chan struct{})
	go func() { _, _ = l.Build(context.Background(), Request{Tag: "web:a"}, nil); close(done) }()
	<-entered

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { _, err := l.Build(ctx, Request{Tag: "web:b"}, nil); errc <- err }()
	waitFor(t, func() bool { w, _ := l.Queued(); return w == 1 })
	cancel()
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	close(block)
	<-done
	if w, r := l.Queued(); w != 0 || r != 0 {
		t.Fatalf("waiting=%d running=%d", w, r)
	}
	if len(rec.runs) != 1 {
		t.Fatalf("queued-and-canceled build must not be recorded, runs=%d", len(rec.runs))
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(time.Millisecond)
	}
}

type fakeCgroups struct {
	parent  string
	mem     int
	cpu     int
	cleaned bool
}

func (f *fakeCgroups) Provision(_ string, mem, cpu int) (string, func(), error) {
	f.mem, f.cpu = mem, cpu
	return f.parent, func() { f.cleaned = true }, nil
}

func TestLimiterMemoryKillGetsOwnStatusAndCgroup(t *testing.T) {
	rec := &memRecorder{}
	cg := &fakeCgroups{parent: "levelrail-builds/web"}
	var gotParent string
	inner := cgroupSpy{onBuild: func(r Request) { gotParent = r.CgroupParent }, err: errors.New("process did not complete successfully: exit code: 137")}
	l := NewLimiter(inner, fixed(Limits{MemoryMB: 512, CPUMillis: 500}), WithRunRecorder(rec), WithCgroupProvisioner(cg))

	_, err := l.Build(context.Background(), Request{Tag: "reg/web:abc"}, nil)
	if !errors.Is(err, ErrBuildMemoryLimit) {
		t.Fatalf("err = %v, want memory limit", err)
	}
	if rec.runs[0].Status != RunMemoryLimit {
		t.Fatalf("status = %s", rec.runs[0].Status)
	}
	if gotParent != cg.parent || cg.mem != 512 || cg.cpu != 500 || !cg.cleaned {
		t.Fatalf("parent=%q mem=%d cpu=%d cleaned=%v", gotParent, cg.mem, cg.cpu, cg.cleaned)
	}
}

func TestLimiterWithoutResourceLimitsSkipsCgroup(t *testing.T) {
	cg := &fakeCgroups{parent: "x"}
	l := NewLimiter(cgroupSpy{}, fixed(Limits{}), WithCgroupProvisioner(cg))
	if _, err := l.Build(context.Background(), Request{Tag: "reg/web:abc"}, nil); err != nil {
		t.Fatal(err)
	}
	if cg.cleaned || cg.mem != 0 {
		t.Fatal("cgroup provisioned without a resource limit")
	}
}

type cgroupSpy struct {
	onBuild func(Request)
	err     error
}

func (c cgroupSpy) Build(_ context.Context, r Request, _ func(ProgressEvent)) (*Result, error) {
	if c.onBuild != nil {
		c.onBuild(r)
	}
	return nil, c.err
}

func (c cgroupSpy) BuildRailpack(context.Context, RailpackRequest, func(ProgressEvent)) (*Result, error) {
	return nil, c.err
}
