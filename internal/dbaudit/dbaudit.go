// Package dbaudit records every database explorer action off the request path.
// A full queue drops the event and counts it: audit must never block a query,
// but the loss has to be visible.
package dbaudit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Outcomes and operation kinds the explorer reports.
const (
	OutcomeOK        = "ok"
	OutcomeDenied    = "denied"
	OutcomeRejected  = "rejected"
	OutcomeTimeout   = "timeout"
	OutcomeCancelled = "cancelled"
	OutcomeError     = "error"
)

// Sink is the store surface the writer and sweeper need.
type Sink interface {
	InsertDatabaseStatementAudits(ctx context.Context, rows []store.DatabaseStatementAudit) error
	PruneDatabaseStatementAudits(ctx context.Context, cutoff time.Time, chunk int) (int64, error)
}

// Settings are read on every call so a live change applies at once.
type Settings interface {
	StoreText(ctx context.Context) bool
	QueueSize(ctx context.Context) int
	RetentionDays(ctx context.Context) int
}

// Event is one explorer action. StatementText is dropped unless the setting allows it.
type Event struct {
	At            time.Time
	DatabaseName  string
	Engine        string
	PrincipalType string
	PrincipalID   string
	PrincipalName string
	Ability       string
	Action        string
	OpKind        string
	Fingerprint   string
	StatementText string
	RowsReturned  int64
	Bytes         int64
	DurationMs    int64
	Outcome       string
	ErrorClass    string
	ClientIP      string
	ClientKind    string
	BranchName    string
	RequestID     string
}

// Options tune the writer; zero values use the defaults below, which
// callers seed from env.
type Options struct {
	FlushInterval time.Duration
	FlushRows     int
	Logger        *slog.Logger
}

// Writer batches events and flushes them on an interval or when enough rows wait.
type Writer struct {
	sink     Sink
	set      Settings
	log      *slog.Logger
	interval time.Duration
	rows     int

	mu      sync.Mutex
	buf     []store.DatabaseStatementAudit
	dropped atomic.Uint64
	failed  atomic.Uint64
	wake    chan struct{}
	done    chan struct{}
	stop    chan struct{}
	started bool
}

// New builds a writer; call Start before Record and Close at shutdown.
func New(sink Sink, set Settings, o Options) *Writer {
	if o.FlushInterval <= 0 {
		o.FlushInterval = time.Second
	}
	if o.FlushRows <= 0 {
		o.FlushRows = 200
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return &Writer{sink: sink, set: set, log: o.Logger, interval: o.FlushInterval, rows: o.FlushRows,
		wake: make(chan struct{}, 1), done: make(chan struct{}), stop: make(chan struct{})}
}

// Start runs the flush loop until Close.
func (w *Writer) Start() {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.mu.Unlock()
	go w.loop()
}

// Record queues one event and never blocks. A full queue drops it and counts the drop.
func (w *Writer) Record(ctx context.Context, ev Event) {
	if w == nil {
		return
	}
	row, err := w.row(ctx, ev)
	if err != nil {
		w.dropped.Add(1)
		return
	}
	limit := w.set.QueueSize(ctx)
	w.mu.Lock()
	if limit > 0 && len(w.buf) >= limit {
		w.mu.Unlock()
		w.dropped.Add(1)
		return
	}
	w.buf = append(w.buf, row)
	full := len(w.buf) >= w.rows
	w.mu.Unlock()
	if full {
		select {
		case w.wake <- struct{}{}:
		default:
		}
	}
}

func (w *Writer) row(ctx context.Context, ev Event) (store.DatabaseStatementAudit, error) {
	id, err := newID()
	if err != nil {
		return store.DatabaseStatementAudit{}, err
	}
	at := ev.At
	if at.IsZero() {
		at = time.Now()
	}
	text := ""
	if w.set.StoreText(ctx) {
		text = ev.StatementText
	}
	return store.DatabaseStatementAudit{
		ID: id, At: at.UTC().Format(store.StatementAuditTimeLayout), DatabaseName: ev.DatabaseName, Engine: ev.Engine,
		PrincipalType: ev.PrincipalType, PrincipalID: ev.PrincipalID, PrincipalName: ev.PrincipalName,
		Ability: ev.Ability, Action: ev.Action, OpKind: ev.OpKind, Fingerprint: ev.Fingerprint, StatementText: text,
		RowsReturned: ev.RowsReturned, Bytes: ev.Bytes, DurationMs: ev.DurationMs, Outcome: ev.Outcome,
		ErrorClass: ev.ErrorClass, ClientIP: ev.ClientIP, ClientKind: ev.ClientKind, BranchName: ev.BranchName, RequestID: ev.RequestID,
	}, nil
}

func newID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("dbaudit: id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (w *Writer) loop() {
	defer close(w.done)
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			w.Flush(context.Background())
		case <-w.wake:
			w.Flush(context.Background())
		case <-w.stop:
			w.Flush(context.Background())
			return
		}
	}
}

// Flush writes everything queued now. A failed write is counted and logged, not retried:
// retrying would grow the queue behind a broken database.
func (w *Writer) Flush(ctx context.Context) {
	w.mu.Lock()
	batch := w.buf
	w.buf = nil
	w.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	if err := w.sink.InsertDatabaseStatementAudits(ctx, batch); err != nil {
		w.failed.Add(uint64(len(batch)))
		w.log.Warn("dbaudit: flush failed", slog.String("error", err.Error()), slog.Int("rows", len(batch)))
	}
}

// Close stops the loop and flushes what is left.
func (w *Writer) Close() {
	w.mu.Lock()
	started := w.started
	w.started = false
	w.mu.Unlock()
	if !started {
		w.Flush(context.Background())
		return
	}
	close(w.stop)
	<-w.done
}

// Dropped counts events lost to a full queue or a failed id; Failed counts rows a flush could not write.
func (w *Writer) Dropped() uint64 { return w.dropped.Load() }

// Failed counts rows lost to a failed flush.
func (w *Writer) Failed() uint64 { return w.failed.Load() }

// WriteMetrics renders the loss counters in the Prometheus text format.
func (w *Writer) WriteMetrics(out io.Writer, prefix string) error {
	_, err := fmt.Fprintf(out, "# HELP %[1]sdatabase_audit_dropped_total Explorer audit events dropped because the queue was full.\n# TYPE %[1]sdatabase_audit_dropped_total counter\n%[1]sdatabase_audit_dropped_total %[2]d\n# HELP %[1]sdatabase_audit_failed_total Explorer audit rows lost to a failed write.\n# TYPE %[1]sdatabase_audit_failed_total counter\n%[1]sdatabase_audit_failed_total %[3]d\n", prefix, w.Dropped(), w.Failed())
	return err
}

// Sweep deletes rows past the retention window once.
func Sweep(ctx context.Context, sink Sink, set Settings, now time.Time) (int64, error) {
	days := set.RetentionDays(ctx)
	if days <= 0 {
		return 0, nil
	}
	return sink.PruneDatabaseStatementAudits(ctx, now.AddDate(0, 0, -days), 0)
}

// RunSweeper sweeps every interval until ctx ends.
func RunSweeper(ctx context.Context, sink Sink, set Settings, interval time.Duration, log *slog.Logger) {
	if interval <= 0 {
		interval = time.Hour
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := Sweep(ctx, sink, set, time.Now()); err != nil {
				log.Warn("dbaudit: sweep failed", slog.String("error", err.Error()))
			} else if n > 0 {
				log.Info("dbaudit: swept", slog.Int64("rows", n))
			}
		}
	}
}
