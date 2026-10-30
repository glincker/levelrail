package monitor

import (
	"context"
	"sync"
	"sync/atomic"
)

// ResultSink persists a batch of results and updates the hourly rollups in the same transaction.
type ResultSink interface {
	InsertResults(ctx context.Context, results []Result) error
}

// Batcher buffers results so the single SQLite writer sees one transaction per flush.
type Batcher struct {
	mu      sync.Mutex
	buf     []Result
	max     int
	sink    ResultSink
	dropped atomic.Uint64
}

// NewBatcher builds a Batcher that holds at most capacity results.
func NewBatcher(sink ResultSink, capacity int) *Batcher {
	return &Batcher{sink: sink, max: capacity}
}

// Add queues a result, dropping the oldest when the buffer is full.
func (b *Batcher) Add(r Result) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, r)
	b.trimLocked()
}

func (b *Batcher) trimLocked() {
	if over := len(b.buf) - b.max; over > 0 {
		b.buf = append([]Result(nil), b.buf[over:]...)
		b.dropped.Add(uint64(over))
	}
}

// Flush writes the queued results. On failure they are put back, bounded by the buffer size.
func (b *Batcher) Flush(ctx context.Context) error {
	b.mu.Lock()
	batch := b.buf
	b.buf = nil
	b.mu.Unlock()
	if len(batch) == 0 {
		return nil
	}
	if err := b.sink.InsertResults(ctx, batch); err != nil {
		b.mu.Lock()
		b.buf = append(batch, b.buf...)
		b.trimLocked()
		b.mu.Unlock()
		return err
	}
	return nil
}

// Dropped counts results discarded because the buffer was full.
func (b *Batcher) Dropped() uint64 { return b.dropped.Load() }

// Pending is the number of queued results.
func (b *Batcher) Pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.buf)
}

// HourBucket is the start of the hour containing unix second at.
func HourBucket(at int64) int64 { return at - at%3600 }

// Aggregate folds results into hour rollups. Latency counts only successful probes.
func Aggregate(results []Result) []Rollup {
	type key struct {
		id     string
		bucket int64
	}
	idx := map[key]int{}
	var out []Rollup
	for _, r := range results {
		k := key{r.MonitorID, HourBucket(r.At)}
		i, ok := idx[k]
		if !ok {
			i = len(out)
			idx[k] = i
			out = append(out, Rollup{MonitorID: r.MonitorID, Bucket: k.bucket})
		}
		out[i].Total++
		if !r.OK {
			out[i].Failed++
			continue
		}
		out[i].LatSum += int64(r.LatencyMS)
		if r.LatencyMS > out[i].LatMax {
			out[i].LatMax = r.LatencyMS
		}
	}
	return out
}
