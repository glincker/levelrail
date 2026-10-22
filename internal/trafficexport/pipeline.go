package trafficexport

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math/rand/v2"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxLastErrorLen = 500

// SinkConfig attaches a sink to the pipeline.
type SinkConfig struct {
	Sink Sink
	// Sample is the fraction of traces exported, 0 to 1. Zero means none, so
	// callers wanting everything pass 1.
	Sample float64
	// Secrets are scrubbed from the sink's last_error.
	Secrets []string
}

// SinkStats is a point-in-time view of one sink.
type SinkStats struct {
	Name        string    `json:"name"`
	Sent        uint64    `json:"sent"`
	Dropped     uint64    `json:"dropped"`
	Failed      uint64    `json:"failed"`
	SampledOut  uint64    `json:"sampled_out"`
	Queued      int       `json:"queued"`
	LastError   string    `json:"last_error,omitempty"`
	LastSuccess time.Time `json:"last_success"`
}

// Pipeline fans events out to per-sink bounded queues. Enqueue never blocks.
type Pipeline struct {
	mu      sync.RWMutex
	closed  bool
	workers []*worker
	cancel  context.CancelFunc
}

// NewPipeline starts one delivery goroutine per sink.
func NewPipeline(opts Options, sinks ...SinkConfig) *Pipeline {
	opts = opts.withDefaults()
	ctx, cancel := context.WithCancel(context.Background())
	p := &Pipeline{cancel: cancel}
	for _, sc := range sinks {
		w := &worker{
			sink: sc.Sink, opts: opts, sample: sc.Sample, secrets: sc.Secrets,
			queue: make(chan Event, opts.QueueSize), quit: make(chan struct{}), done: make(chan struct{}),
			wait: sleepCtx, jitter: rand.Float64, //nolint:forbidigo // retry jitter needs no crypto randomness
			now: time.Now,
		}
		p.workers = append(p.workers, w)
		go w.run(ctx)
	}
	return p
}

// Enqueue offers e to every sink and drops it for any sink whose queue is
// full or whose sampler rejects it.
func (p *Pipeline) Enqueue(e Event) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return
	}
	for _, w := range p.workers {
		w.offer(e)
	}
}

// Stats returns one entry per sink in the order they were added.
func (p *Pipeline) Stats() []SinkStats {
	out := make([]SinkStats, 0, len(p.workers))
	for _, w := range p.workers {
		out = append(out, w.stats())
	}
	return out
}

// Close stops accepting events, flushes what is queued and waits for the
// workers. If ctx ends first, in-flight deliveries are cancelled.
func (p *Pipeline) Close(ctx context.Context) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()
	for _, w := range p.workers {
		close(w.quit)
	}
	defer p.cancel()
	for _, w := range p.workers {
		select {
		case <-w.done:
		case <-ctx.Done():
			p.cancel()
			<-w.done
			return fmt.Errorf("trafficexport: close: %w", ctx.Err())
		}
	}
	return nil
}

type worker struct {
	sink    Sink
	opts    Options
	sample  float64
	secrets []string
	queue   chan Event
	quit    chan struct{}
	done    chan struct{}

	sent, dropped, failed, sampledOut atomic.Uint64

	mu          sync.Mutex
	lastError   string
	lastSuccess time.Time

	wait   func(ctx context.Context, d time.Duration) bool
	jitter func() float64
	now    func() time.Time
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// sampled decides per trace (request id when there is no trace) so every
// request of a trace is kept or dropped together.
func sampled(e Event, rate float64) bool {
	if rate >= 1 {
		return true
	}
	if rate <= 0 {
		return false
	}
	key := e.TraceID
	if key == "" {
		key = e.RequestID
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return float64(mix64(h.Sum64())>>11)/float64(1<<53) < rate
}

// mix64 is the splitmix64 finalizer: FNV alone spreads near-identical ids
// (sequential trace ids) unevenly across the range.
func mix64(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	return x ^ x>>31
}

func (w *worker) offer(e Event) {
	if !sampled(e, w.sample) {
		w.sampledOut.Add(1)
		return
	}
	select {
	case w.queue <- e:
	default:
		w.dropped.Add(1)
	}
}

func (w *worker) stats() SinkStats {
	w.mu.Lock()
	le, ls := w.lastError, w.lastSuccess
	w.mu.Unlock()
	return SinkStats{
		Name: w.sink.Name(), Sent: w.sent.Load(), Dropped: w.dropped.Load(), Failed: w.failed.Load(),
		SampledOut: w.sampledOut.Load(), Queued: len(w.queue), LastError: le, LastSuccess: ls,
	}
}

func (w *worker) run(ctx context.Context) {
	defer close(w.done)
	tick := time.NewTicker(w.opts.FlushInterval)
	defer tick.Stop()
	batch := make([]Event, 0, w.opts.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		w.deliver(ctx, batch)
		batch = batch[:0]
	}
	for {
		select {
		case e := <-w.queue:
			batch = append(batch, e)
			if len(batch) >= w.opts.BatchSize {
				flush()
			}
		case <-tick.C:
			flush()
		case <-w.quit:
			for {
				select {
				case e := <-w.queue:
					batch = append(batch, e)
					if len(batch) >= w.opts.BatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

type retryable interface{ Retryable() bool }

func (w *worker) deliver(ctx context.Context, batch []Event) {
	var err error
	for attempt := 1; attempt <= w.opts.MaxAttempts; attempt++ {
		if err = w.sink.Send(ctx, batch); err == nil {
			w.sent.Add(uint64(len(batch)))
			w.mu.Lock()
			w.lastSuccess = w.now()
			w.mu.Unlock()
			return
		}
		var re retryable
		permanent := errors.As(err, &re) && !re.Retryable()
		slog.Warn("trafficexport: delivery failed", "sink", w.sink.Name(), "attempt", attempt,
			"events", len(batch), "permanent", permanent, "error", w.redact(err.Error()))
		if permanent || attempt == w.opts.MaxAttempts || ctx.Err() != nil {
			break
		}
		if !w.wait(ctx, w.backoff(attempt)) {
			break
		}
	}
	w.failed.Add(uint64(len(batch)))
	w.mu.Lock()
	w.lastError = w.redact(err.Error())
	w.mu.Unlock()
}

// backoff doubles per attempt up to the ceiling, then takes a random value in
// its upper half so synchronized sinks do not retry in lockstep.
func (w *worker) backoff(attempt int) time.Duration {
	d := w.opts.BackoffBase
	for i := 1; i < attempt && d < w.opts.BackoffMax; i++ {
		d *= 2
	}
	if d > w.opts.BackoffMax {
		d = w.opts.BackoffMax
	}
	return d/2 + time.Duration(w.jitter()*float64(d/2))
}

var (
	schemeToken = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[^\s"',;&]+`)
	keyedSecret = regexp.MustCompile(`(?i)(api[_-]?key|token|authorization|secret|password|passwd)([=:"'\s]+)[^\s"',;&]+`)
)

func (w *worker) redact(msg string) string {
	for _, s := range w.secrets {
		if s != "" {
			msg = strings.ReplaceAll(msg, s, "[redacted]")
		}
	}
	msg = schemeToken.ReplaceAllString(msg, "$1 [redacted]")
	msg = keyedSecret.ReplaceAllString(msg, "$1$2[redacted]")
	if len(msg) > maxLastErrorLen {
		msg = msg[:maxLastErrorLen]
	}
	return msg
}
