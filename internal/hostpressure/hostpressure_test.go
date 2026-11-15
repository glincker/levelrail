package hostpressure

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		Enabled: true, ShedEnabled: true,
		CPUWarn: 60, CPUCritical: 85, MemWarn: 15, MemCritical: 30, MemFullWarn: 5, MemFullCritical: 15,
		AvailWarn: 10, AvailCritical: 5, LoadWarn: 200, LoadCritical: 400,
		ExitPercent: 70, Hold: time.Minute, Jitter: 0, Interval: time.Second, NoSwapRAM: 16 << 30,
	}
}

type fake struct {
	mu  sync.Mutex
	s   Sample
	now time.Time
	cfg Config
}

func newFake() *fake {
	return &fake{s: Sample{PSI: true, MemTotal: 8 << 30, MemAvailable: 6 << 30, SwapTotal: 0, NumCPU: 2}, now: time.Unix(1000, 0), cfg: testConfig()}
}

func (f *fake) guard() *Guard {
	g := New(func() (Sample, error) { f.mu.Lock(); defer f.mu.Unlock(); return f.s, nil }, func() Config { return f.cfg }, nil)
	g.now = func() time.Time { f.mu.Lock(); defer f.mu.Unlock(); return f.now }
	return g
}

func (f *fake) set(fn func(*Sample))    { f.mu.Lock(); fn(&f.s); f.mu.Unlock() }
func (f *fake) advance(d time.Duration) { f.mu.Lock(); f.now = f.now.Add(d); f.mu.Unlock() }

func TestHysteresisAndHold(t *testing.T) {
	f := newFake()
	g := f.guard()
	ctx := context.Background()
	if st := g.Observe(ctx); st.State != StateOK || st.Source != "psi" {
		t.Fatalf("want ok psi, got %+v", st)
	}
	f.set(func(s *Sample) { s.CPUSome10 = 97 })
	if st := g.Observe(ctx); st.State != StateCritical {
		t.Fatalf("want critical, got %s", st.State)
	}
	// Eased below enter but above exit (85*0.7=59.5): stays critical.
	f.set(func(s *Sample) { s.CPUSome10 = 70 })
	f.advance(10 * time.Minute)
	if st := g.Observe(ctx); st.State != StateCritical {
		t.Fatalf("want critical held above exit, got %s", st.State)
	}
	// Below exit but hold not elapsed.
	f.set(func(s *Sample) { s.CPUSome10 = 5 })
	if st := g.Observe(ctx); st.State != StateCritical {
		t.Fatalf("want critical during hold, got %s", st.State)
	}
	f.advance(30 * time.Second)
	if st := g.Observe(ctx); st.State != StateCritical {
		t.Fatalf("want critical before hold ends, got %s", st.State)
	}
	f.advance(31 * time.Second)
	if st := g.Observe(ctx); st.State != StateOK {
		t.Fatalf("want ok after hold, got %s", st.State)
	}
}

func TestMemoryFullStallAndAvailable(t *testing.T) {
	f := newFake()
	g := f.guard()
	f.set(func(s *Sample) { s.MemFull10 = 50 })
	if st := g.Observe(context.Background()); st.State != StateCritical {
		t.Fatalf("memory full stall 50 must be critical, got %s", st.State)
	}
	f2 := newFake()
	g2 := f2.guard()
	f2.set(func(s *Sample) { s.MemAvailable = 400 << 20 })
	st := g2.Observe(context.Background())
	if st.State != StateCritical || len(st.Reasons) == 0 {
		t.Fatalf("low MemAvailable must be critical with a reason, got %+v", st)
	}
}

func TestFallbackWithoutPSI(t *testing.T) {
	f := newFake()
	f.set(func(s *Sample) { s.PSI = false; s.Load1 = 9 }) // 450 per cpu x100 on 2 cpus
	g := f.guard()
	st := g.Observe(context.Background())
	if st.State != StateCritical || st.Source != "fallback" {
		t.Fatalf("want critical fallback, got %+v", st)
	}
}

func TestNoSwapFlag(t *testing.T) {
	f := newFake()
	g := f.guard()
	if st := g.Observe(context.Background()); !st.NoSwap {
		t.Fatal("8 GB without swap should flag")
	}
	f.set(func(s *Sample) { s.SwapTotal = 2 << 30 })
	if st := g.Observe(context.Background()); st.NoSwap {
		t.Fatal("swap present should not flag")
	}
}

func TestBuildCapOnlyWhenWarn(t *testing.T) {
	f := newFake()
	g := f.guard()
	g.Observe(context.Background())
	if got := g.BuildCap(4); got != 4 {
		t.Fatalf("ok keeps limit, got %d", got)
	}
	f.set(func(s *Sample) { s.CPUSome10 = 65 })
	g.Observe(context.Background())
	if got := g.BuildCap(4); got != 1 {
		t.Fatalf("warn caps to 1, got %d", got)
	}
	f.cfg.ShedEnabled = false
	if got := g.BuildCap(4); got != 4 {
		t.Fatalf("shed off keeps limit, got %d", got)
	}
}

func TestAdmitQueuesAndResumes(t *testing.T) {
	f := newFake()
	g := f.guard()
	g.rand = func(time.Duration) time.Duration { return time.Millisecond }
	ctx := context.Background()
	f.set(func(s *Sample) { s.MemFull10 = 60 })
	g.Observe(ctx)

	msgs := make(chan string, 4)
	done := make(chan error, 1)
	go func() { done <- g.Admit(ctx, func(m string) { msgs <- m }) }()
	select {
	case m := <-msgs:
		if m != WaitMessage {
			t.Fatalf("unexpected message %q", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("build was not told to wait")
	}
	select {
	case err := <-done:
		t.Fatalf("build admitted while critical: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	f.set(func(s *Sample) { s.MemFull10 = 0 })
	f.advance(2 * time.Minute)
	g.Observe(ctx) // starts the hold
	f.advance(2 * time.Minute)
	g.Observe(ctx) // returns to ok
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("admit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("build did not resume")
	}
}

func TestAdmitCancelAndDisabled(t *testing.T) {
	f := newFake()
	g := f.guard()
	f.set(func(s *Sample) { s.MemFull10 = 60 })
	g.Observe(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- g.Admit(ctx, nil) }()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("want canceled, got %v", err)
	}
	f.cfg.ShedEnabled = false
	if err := g.Admit(context.Background(), nil); err != nil {
		t.Fatalf("shed off must admit, got %v", err)
	}
}

func TestOnChangeAndConsumers(t *testing.T) {
	f := newFake()
	g := f.guard()
	g.SetConsumers(func(context.Context) ([]Consumer, error) { return []Consumer{{Name: "web", MemBytes: 1 << 30}}, nil })
	var got []Status
	g.OnChange = func(s Status) { got = append(got, s) }
	f.set(func(s *Sample) { s.CPUSome10 = 90 })
	g.Observe(context.Background())
	g.Observe(context.Background())
	if len(got) != 1 || len(got[0].TopConsumers) != 1 || len(got[0].Actions) == 0 {
		t.Fatalf("want one change with consumers and actions, got %+v", got)
	}
}

func TestProcReaderParsesTree(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("meminfo", "MemTotal: 8000000 kB\nMemAvailable: 1000000 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n")
	write("loadavg", "180.50 90.00 40.00 1/200 123\n")
	write("pressure/cpu", "some avg10=97.00 avg60=80.00 avg300=10.00 total=1\n")
	write("pressure/memory", "some avg10=70.00 avg60=60.00 avg300=1.00 total=1\nfull avg10=55.00 avg60=40.00 avg300=1.00 total=1\n")
	s, err := ProcReader{Root: dir, NumCPU: func() int { return 2 }}.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !s.PSI || s.CPUSome10 != 97 || s.MemFull10 != 55 || s.Load1 != 180.5 || s.MemAvailable != 1000000*1024 {
		t.Fatalf("bad parse: %+v", s)
	}
	// Without the pressure files the reader degrades to the fallback signals.
	if err := os.RemoveAll(filepath.Join(dir, "pressure")); err != nil {
		t.Fatal(err)
	}
	s, err = ProcReader{Root: dir}.Read()
	if err != nil || s.PSI {
		t.Fatalf("want no psi and no error, got %+v %v", s, err)
	}
	if _, err := (ProcReader{Root: filepath.Join(dir, "missing")}).Read(); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("want unsupported, got %v", err)
	}
}

func TestUnavailableReaderStaysOK(t *testing.T) {
	g := New(func() (Sample, error) { return Sample{}, ErrUnsupported }, testConfig, nil)
	if st := g.Observe(context.Background()); st.State != StateOK || st.Source != "unavailable" {
		t.Fatalf("got %+v", st)
	}
}
