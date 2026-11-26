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
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fieldMaxBytes bounds every free text field of a row so a hostile request cannot bloat the table.
const fieldMaxBytes = 128

// textPreMaskBytes bounds the work secret masking does on one statement.
const textPreMaskBytes = 65536

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
	TrimDatabaseStatementAudits(ctx context.Context, maxRows, chunk int) (int64, error)
}

// Settings are read on every call so a live change applies at once.
type Settings interface {
	StoreText(ctx context.Context) bool
	QueueSize(ctx context.Context) int
	RetentionDays(ctx context.Context) int
	MaxRows(ctx context.Context) int
	TextMaxBytes(ctx context.Context) int
}

// Event is one explorer action. StatementText is dropped unless the setting allows it.
type Event struct {
	At            time.Time
	DatabaseName  string
	DatabaseID    string
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
	noPrin  atomic.Uint64
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
		wake: make(chan struct{}, 1)}
}

// Start runs the flush loop until Close.
func (w *Writer) Start() {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.stop, w.done = make(chan struct{}), make(chan struct{})
	stop, done := w.stop, w.done
	w.mu.Unlock()
	go w.loop(stop, done)
}

// HasRoom reports whether the queue can take one more event; a write path asks before it acts.
func (w *Writer) HasRoom(ctx context.Context) bool {
	if w == nil {
		return true
	}
	limit := w.set.QueueSize(ctx)
	w.mu.Lock()
	defer w.mu.Unlock()
	return limit <= 0 || len(w.buf) < limit
}

// CountUnauthenticated counts a request that had no principal; no row is written for it.
func (w *Writer) CountUnauthenticated() {
	if w != nil {
		w.noPrin.Add(1)
	}
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
		text = cutBytes(secretmask.Mask(cutBytes(ev.StatementText, textPreMaskBytes)), w.set.TextMaxBytes(ctx))
	}
	return store.DatabaseStatementAudit{
		ID: id, At: at.UTC().Format(store.StatementAuditTimeLayout), DatabaseName: cutBytes(ev.DatabaseName, fieldMaxBytes), DatabaseID: cutBytes(ev.DatabaseID, fieldMaxBytes), Engine: cutBytes(ev.Engine, fieldMaxBytes),
		PrincipalType: cutBytes(ev.PrincipalType, fieldMaxBytes), PrincipalID: cutBytes(ev.PrincipalID, fieldMaxBytes), PrincipalName: cutBytes(ev.PrincipalName, fieldMaxBytes),
		Ability: cutBytes(ev.Ability, fieldMaxBytes), Action: ev.Action, OpKind: cutBytes(ev.OpKind, fieldMaxBytes), Fingerprint: cutBytes(ev.Fingerprint, fieldMaxBytes), StatementText: text,
		RowsReturned: ev.RowsReturned, Bytes: ev.Bytes, DurationMs: ev.DurationMs, Outcome: ev.Outcome,
		ErrorClass: cutBytes(ev.ErrorClass, fieldMaxBytes), ClientIP: cutBytes(ev.ClientIP, fieldMaxBytes), ClientKind: cutBytes(ev.ClientKind, fieldMaxBytes),
		BranchName: cutBytes(ev.BranchName, fieldMaxBytes), RequestID: cutBytes(ev.RequestID, fieldMaxBytes),
	}, nil
}

// cutBytes truncates to at most n bytes on a rune boundary.
func cutBytes(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func newID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("dbaudit: id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (w *Writer) loop(stop, done chan struct{}) {
	defer close(done)
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			w.Flush(context.Background())
		case <-w.wake:
			w.Flush(context.Background())
		case <-stop:
			w.Flush(context.Background())
			return
		}
	}
}

// Flush writes everything queued now. A failed batch is retried once, then written row by
// row so one bad row costs only itself; rows that still fail are counted and logged.
func (w *Writer) Flush(ctx context.Context) {
	w.mu.Lock()
	batch := w.buf
	w.buf = nil
	w.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	if err := w.sink.InsertDatabaseStatementAudits(ctx, batch); err == nil {
		return
	}
	if err := w.sink.InsertDatabaseStatementAudits(ctx, batch); err == nil {
		return
	}
	var lost uint64
	var last error
	for _, row := range batch {
		if err := w.sink.InsertDatabaseStatementAudits(ctx, []store.DatabaseStatementAudit{row}); err != nil {
			lost++
			last = err
		}
	}
	if lost > 0 {
		w.failed.Add(lost)
		w.log.Warn("dbaudit: flush failed", slog.String("error", last.Error()), slog.Uint64("rows_lost", lost), slog.Int("rows", len(batch)))
	}
}

// Close stops the loop and flushes what is left.
func (w *Writer) Close() {
	w.mu.Lock()
	started := w.started
	stop, done := w.stop, w.done
	w.started = false
	w.mu.Unlock()
	if !started {
		w.Flush(context.Background())
		return
	}
	close(stop)
	<-done
}

// Dropped counts events lost to a full queue or a failed id; Failed counts rows a flush could not write.
func (w *Writer) Dropped() uint64 { return w.dropped.Load() }

// Failed counts rows lost to a failed flush.
func (w *Writer) Failed() uint64 { return w.failed.Load() }

// Unauthenticated counts explorer requests that carried no principal and wrote no row.
func (w *Writer) Unauthenticated() uint64 { return w.noPrin.Load() }

// WriteMetrics renders the loss counters in the Prometheus text format.
func (w *Writer) WriteMetrics(out io.Writer, prefix string) error {
	_, err := fmt.Fprintf(out, "# HELP %[1]sdatabase_audit_dropped_total Explorer audit events dropped because the queue was full.\n# TYPE %[1]sdatabase_audit_dropped_total counter\n%[1]sdatabase_audit_dropped_total %[2]d\n# HELP %[1]sdatabase_audit_failed_total Explorer audit rows lost to a failed write.\n# TYPE %[1]sdatabase_audit_failed_total counter\n%[1]sdatabase_audit_failed_total %[3]d\n# HELP %[1]sdatabase_audit_unauthenticated_total Explorer requests without a principal, not recorded.\n# TYPE %[1]sdatabase_audit_unauthenticated_total counter\n%[1]sdatabase_audit_unauthenticated_total %[4]d\n", prefix, w.Dropped(), w.Failed(), w.Unauthenticated())
	return err
}

// Sweep deletes rows past the retention window, then the oldest rows beyond the row cap, once.
func Sweep(ctx context.Context, sink Sink, set Settings, now time.Time) (int64, error) {
	var total int64
	if days := set.RetentionDays(ctx); days > 0 {
		n, err := sink.PruneDatabaseStatementAudits(ctx, now.AddDate(0, 0, -days), 0)
		total += n
		if err != nil {
			return total, err
		}
	}
	if maxRows := set.MaxRows(ctx); maxRows > 0 {
		n, err := sink.TrimDatabaseStatementAudits(ctx, maxRows, 0)
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// RunSweeper sweeps once after firstDelay, then every interval until ctx ends.
func RunSweeper(ctx context.Context, sink Sink, set Settings, firstDelay, interval time.Duration, log *slog.Logger) {
	if interval <= 0 {
		interval = time.Hour
	}
	sweep := func() {
		if n, err := Sweep(ctx, sink, set, time.Now()); err != nil {
			log.Warn("dbaudit: sweep failed", slog.String("error", err.Error()))
		} else if n > 0 {
			log.Info("dbaudit: swept", slog.Int64("rows", n))
		}
	}
	first := time.NewTimer(firstDelay)
	defer first.Stop()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-first.C:
			sweep()
		case <-t.C:
			sweep()
		}
	}
}
