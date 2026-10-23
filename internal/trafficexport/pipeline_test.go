package trafficexport

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeSink struct {
	mu       sync.Mutex
	name     string
	failures []error // consumed one per call; nil entry or exhausted means success
	calls    int
	got      []Event
	gate     chan struct{} // when set, Send blocks until closed
}

func (f *fakeSink) Name() string { return f.name }

func (f *fakeSink) Send(ctx context.Context, ev []Event) error {
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if len(f.failures) > 0 {
		err := f.failures[0]
		f.failures = f.failures[1:]
		if err != nil {
			return err
		}
	}
	f.got = append(f.got, ev...)
	return nil
}

func (f *fakeSink) received() []Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Event(nil), f.got...)
}

func (f *fakeSink) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func fastOpts() Options {
	return Options{QueueSize: 100, BatchSize: 10, FlushInterval: 5 * time.Millisecond, MaxAttempts: 4, BackoffBase: time.Millisecond, BackoffMax: 8 * time.Millisecond}
}

func eventN(i int) Event {
	e := testEvent(0)
	e.RequestID = fmt.Sprintf("req-%d", i)
	e.TraceID = fmt.Sprintf("%032x", i+1)
	return e
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(time.Millisecond)
	}
}

func closeAll(t *testing.T, p *Pipeline) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineQueueFullDrops(t *testing.T) {
	gate := make(chan struct{})
	sink := &fakeSink{name: "slow", gate: gate}
	opts := fastOpts()
	opts.QueueSize, opts.BatchSize = 3, 1
	p := NewPipeline(opts, SinkConfig{Sink: sink, Sample: 1})

	// First event is taken by the worker and parks in Send; the queue then holds 3.
	p.Enqueue(eventN(0))
	waitFor(t, func() bool { return p.Stats()[0].Queued == 0 })
	for i := 1; i <= 10; i++ {
		p.Enqueue(eventN(i))
	}
	st := p.Stats()[0]
	if st.Dropped != 7 || st.Queued != 3 {
		t.Fatalf("dropped %d queued %d, want 7 and 3", st.Dropped, st.Queued)
	}
	close(gate)
	closeAll(t, p)
	if got := len(sink.received()); got != 4 {
		t.Fatalf("delivered %d, want 4", got)
	}
}

func TestPipelineEnqueueNeverBlocks(t *testing.T) {
	sink := &fakeSink{name: "stuck", gate: make(chan struct{})}
	opts := fastOpts()
	opts.QueueSize = 1
	p := NewPipeline(opts, SinkConfig{Sink: sink, Sample: 1})
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			p.Enqueue(eventN(i))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue blocked")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := p.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("close with stuck sink: %v", err)
	}
}

func TestPipelineRetry(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name       string
		failures   []error
		wantCalls  int
		wantSent   uint64
		wantFailed uint64
		wantDelays int
	}{
		{"succeeds first time", nil, 1, 3, 0, 0},
		{"retry then success", []error{boom, boom}, 3, 3, 0, 2},
		{"gives up after max attempts", []error{boom, boom, boom, boom, boom}, 4, 0, 3, 3},
		{"permanent status not retried", []error{&StatusError{Sink: "x", Code: 400}}, 1, 0, 3, 0},
		{"retryable status retried", []error{&StatusError{Sink: "x", Code: 503}}, 2, 3, 0, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sink := &fakeSink{name: "s", failures: tc.failures}
			p := NewPipeline(fastOpts(), SinkConfig{Sink: sink, Sample: 1})
			var delays []time.Duration
			w := p.workers[0]
			// Swapped before any event is enqueued, so the worker cannot race it.
			var dmu sync.Mutex
			w.wait = func(_ context.Context, d time.Duration) bool {
				dmu.Lock()
				delays = append(delays, d)
				dmu.Unlock()
				return true
			}
			w.jitter = func() float64 { return 1 }
			for i := 0; i < 3; i++ {
				p.Enqueue(eventN(i))
			}
			closeAll(t, p)
			st := p.Stats()[0]
			if sink.callCount() != tc.wantCalls || st.Sent != tc.wantSent || st.Failed != tc.wantFailed {
				t.Fatalf("calls %d sent %d failed %d", sink.callCount(), st.Sent, st.Failed)
			}
			dmu.Lock()
			defer dmu.Unlock()
			if len(delays) != tc.wantDelays {
				t.Fatalf("delays %v", delays)
			}
			if tc.wantSent > 0 && st.LastSuccess.IsZero() {
				t.Fatal("last_success not set")
			}
			if tc.wantFailed > 0 && st.LastError == "" {
				t.Fatal("last_error not set")
			}
		})
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	w := &worker{opts: Options{BackoffBase: 100 * time.Millisecond, BackoffMax: 400 * time.Millisecond}.withDefaults()}
	tests := []struct {
		attempt int
		jitter  float64
		want    time.Duration
	}{
		{1, 1, 100 * time.Millisecond},
		{1, 0, 50 * time.Millisecond},
		{2, 1, 200 * time.Millisecond},
		{3, 1, 400 * time.Millisecond},
		{9, 1, 400 * time.Millisecond},
		{9, 0, 200 * time.Millisecond},
	}
	for _, tc := range tests {
		w.jitter = func() float64 { return tc.jitter }
		if got := w.backoff(tc.attempt); got != tc.want {
			t.Errorf("attempt %d jitter %v: %v want %v", tc.attempt, tc.jitter, got, tc.want)
		}
	}
}

func TestSamplingDeterministic(t *testing.T) {
	tests := []struct {
		rate     float64
		min, max float64
	}{
		{0, 0, 0}, {1, 1, 1}, {0.25, 0.20, 0.30}, {0.5, 0.45, 0.55},
	}
	for _, tc := range tests {
		kept := 0
		const n = 4000
		for i := 0; i < n; i++ {
			e := eventN(i)
			a, b := sampled(e, tc.rate), sampled(e, tc.rate)
			if a != b {
				t.Fatalf("rate %v: same event decided differently", tc.rate)
			}
			if a {
				kept++
			}
		}
		if frac := float64(kept) / n; frac < tc.min || frac > tc.max {
			t.Errorf("rate %v kept %.3f", tc.rate, frac)
		}
	}
	// Two requests of one trace share a decision; with no trace the request id decides.
	a, b := eventN(7), eventN(7)
	b.RequestID = "other-request"
	for _, rate := range []float64{0.1, 0.5, 0.9} {
		if sampled(a, rate) != sampled(b, rate) {
			t.Errorf("rate %v: trace split", rate)
		}
	}
	noTrace := Event{RequestID: "r1"}
	first := sampled(noTrace, 0.5)
	for i := 0; i < 5; i++ {
		if sampled(noTrace, 0.5) != first {
			t.Error("request id fallback not deterministic")
		}
	}
}

func TestPipelineSamplingPerSink(t *testing.T) {
	all, none := &fakeSink{name: "all"}, &fakeSink{name: "none"}
	p := NewPipeline(fastOpts(), SinkConfig{Sink: all, Sample: 1}, SinkConfig{Sink: none, Sample: 0})
	for i := 0; i < 20; i++ {
		p.Enqueue(eventN(i))
	}
	closeAll(t, p)
	st := p.Stats()
	if len(all.received()) != 20 || len(none.received()) != 0 || st[1].SampledOut != 20 || st[1].Dropped != 0 {
		t.Fatalf("all %d none %d stats %+v", len(all.received()), len(none.received()), st)
	}
}

func TestPipelineCloseFlushes(t *testing.T) {
	sink := &fakeSink{name: "s"}
	opts := fastOpts()
	opts.FlushInterval = time.Hour
	opts.BatchSize = 1000
	p := NewPipeline(opts, SinkConfig{Sink: sink, Sample: 1})
	for i := 0; i < 25; i++ {
		p.Enqueue(eventN(i))
	}
	closeAll(t, p)
	if got := len(sink.received()); got != 25 {
		t.Fatalf("flushed %d, want 25", got)
	}
	p.Enqueue(eventN(99))
	if st := p.Stats()[0]; st.Dropped != 0 || st.Queued != 0 {
		t.Fatalf("enqueue after close should be ignored: %+v", st)
	}
	closeAll(t, p)
}

func TestPipelineBatchesBySize(t *testing.T) {
	sink := &fakeSink{name: "s"}
	opts := fastOpts()
	opts.FlushInterval = time.Hour
	opts.BatchSize = 5
	p := NewPipeline(opts, SinkConfig{Sink: sink, Sample: 1})
	for i := 0; i < 10; i++ {
		p.Enqueue(eventN(i))
	}
	waitFor(t, func() bool { return len(sink.received()) == 10 })
	if sink.callCount() != 2 {
		t.Fatalf("calls %d, want 2", sink.callCount())
	}
	closeAll(t, p)
}

func TestPipelineFlushesByInterval(t *testing.T) {
	sink := &fakeSink{name: "s"}
	opts := fastOpts()
	opts.BatchSize = 1000
	p := NewPipeline(opts, SinkConfig{Sink: sink, Sample: 1})
	p.Enqueue(eventN(0))
	waitFor(t, func() bool { return len(sink.received()) == 1 })
	closeAll(t, p)
}

func TestStatsRedaction(t *testing.T) {
	tests := []struct {
		name   string
		err    string
		secret []string
	}{
		{"explicit secret", "post https failed for phc_abcdef123", []string{"phc_abcdef123"}},
		{"bearer header", "Authorization: Bearer sk-live-xyz rejected", nil},
		{"basic header", "sent Basic dXNlcjpwYXNz", nil},
		{"query token", "GET /x?api_key=hunter2&a=1 failed", nil},
		{"json token", `body {"token":"hunter2"} rejected`, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sink := &fakeSink{name: "s", failures: []error{errors.New(tc.err)}}
			opts := fastOpts()
			opts.MaxAttempts = 1
			p := NewPipeline(opts, SinkConfig{Sink: sink, Sample: 1, Secrets: tc.secret})
			p.Enqueue(eventN(0))
			closeAll(t, p)
			le := p.Stats()[0].LastError
			for _, leak := range []string{"phc_abcdef123", "sk-live-xyz", "dXNlcjpwYXNz", "hunter2"} {
				if strings.Contains(le, leak) {
					t.Fatalf("leaked %q in %q", leak, le)
				}
			}
			if !strings.Contains(le, "[redacted]") {
				t.Fatalf("nothing redacted: %q", le)
			}
		})
	}
}

func TestOptionsFromEnv(t *testing.T) {
	t.Setenv(EnvQueueSize, "7")
	t.Setenv(EnvBatchSize, "3")
	t.Setenv(EnvFlushInterval, "250ms")
	t.Setenv(EnvMaxAttempts, "2")
	t.Setenv(EnvBackoffBase, "1s")
	t.Setenv(EnvBackoffMax, "bad")
	o := OptionsFromEnv()
	if o.QueueSize != 7 || o.BatchSize != 3 || o.FlushInterval != 250*time.Millisecond || o.MaxAttempts != 2 ||
		o.BackoffBase != time.Second || o.BackoffMax != defaultBackoffMax {
		t.Fatalf("%+v", o)
	}
}
