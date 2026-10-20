package pipeline

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	logBatchSize     = 50
	logFlushInterval = 300 * time.Millisecond
)

// logSink batches a job's output lines into the store, masks secrets, and
// stops storing once the per-job line cap is hit.
type logSink struct {
	store  Store
	log    *slog.Logger
	runID  string
	jobKey string
	mask   *masker
	now    func() time.Time
	limit  int

	mu        sync.Mutex
	buf       []store.PipelineLogLine
	count     int
	truncated bool
	done      chan struct{}
	stopped   sync.WaitGroup
}

func newLogSink(st Store, log *slog.Logger, runID, jobKey string, m *masker, now func() time.Time, limit int) *logSink {
	s := &logSink{store: st, log: log, runID: runID, jobKey: jobKey, mask: m, now: now, limit: limit, done: make(chan struct{})}
	s.stopped.Add(1)
	go s.loop()
	return s
}

func (s *logSink) loop() {
	defer s.stopped.Done()
	t := time.NewTicker(logFlushInterval)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			s.flush()
		}
	}
}

// Line records one output line for a step.
func (s *logSink) Line(step int, stream, text string) {
	text = s.mask.mask(text)
	s.mu.Lock()
	if s.count >= s.limit {
		if !s.truncated {
			s.truncated = true
			s.buf = append(s.buf, store.PipelineLogLine{RunID: s.runID, JobKey: s.jobKey, StepIndex: step, Stream: "stderr", Line: "[log limit reached, further output dropped]", CreatedAt: s.now()})
		}
		s.mu.Unlock()
		return
	}
	s.count++
	s.buf = append(s.buf, store.PipelineLogLine{RunID: s.runID, JobKey: s.jobKey, StepIndex: step, Stream: stream, Line: text, CreatedAt: s.now()})
	full := len(s.buf) >= logBatchSize
	s.mu.Unlock()
	if full {
		s.flush()
	}
}

func (s *logSink) flush() {
	s.mu.Lock()
	batch := s.buf
	s.buf = nil
	s.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	if err := s.store.AppendPipelineLogs(context.Background(), batch); err != nil {
		s.log.Warn("pipeline: store log lines failed", slog.String("run_id", s.runID), slog.String("error", err.Error()))
	}
}

// Close flushes remaining lines and stops the background flusher.
func (s *logSink) Close() {
	close(s.done)
	s.stopped.Wait()
	s.flush()
}
