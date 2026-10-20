package objectstore

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Env vars tuning the archiver; unset or invalid values use the defaults.
const (
	EnvArchivePrefix     = "APP_LOG_ARCHIVE_PREFIX"
	EnvArchiveMaxLines   = "APP_LOG_ARCHIVE_MAX_LINES_PER_OBJECT"
	EnvArchiveMaxWindows = "APP_LOG_ARCHIVE_MAX_WINDOWS_PER_RUN"
	EnvArchiveBackfill   = "APP_LOG_ARCHIVE_MAX_BACKFILL"
	EnvArchiveTick       = "APP_LOG_ARCHIVE_TICK"
)

// Options tune archive runs.
type Options struct {
	Prefix            string
	MaxLinesPerObject int
	MaxWindowsPerRun  int
	Lag               time.Duration
	MaxBackfill       time.Duration
	MaxDumpRange      time.Duration
	Tick              time.Duration
}

// DefaultOptions returns the built-in archive tuning.
func DefaultOptions() Options {
	return Options{
		Prefix: "log-archive", MaxLinesPerObject: 200000, MaxWindowsPerRun: 24,
		Lag: 30 * time.Second, MaxBackfill: 24 * time.Hour, MaxDumpRange: 31 * 24 * time.Hour, Tick: time.Minute,
	}
}

// OptionsFromEnv applies env overrides to DefaultOptions.
func OptionsFromEnv(lookup func(string) (string, bool)) Options {
	o := DefaultOptions()
	if v, ok := lookup(EnvArchivePrefix); ok && v != "" {
		o.Prefix = v
	}
	if n, ok := envInt(lookup, EnvArchiveMaxLines); ok {
		o.MaxLinesPerObject = n
	}
	if n, ok := envInt(lookup, EnvArchiveMaxWindows); ok {
		o.MaxWindowsPerRun = n
	}
	if d, ok := envDuration(lookup, EnvArchiveBackfill); ok {
		o.MaxBackfill = d
	}
	if d, ok := envDuration(lookup, EnvArchiveTick); ok {
		o.Tick = d
	}
	return o
}

func envInt(lookup func(string) (string, bool), name string) (int, bool) {
	v, ok := lookup(name)
	n, err := strconv.Atoi(v)
	return n, ok && err == nil && n > 0
}

func envDuration(lookup func(string) (string, bool), name string) (time.Duration, bool) {
	v, ok := lookup(name)
	d, err := time.ParseDuration(v)
	return d, ok && err == nil && d > 0
}

// ArchiveStore is the persistence surface the archiver needs.
type ArchiveStore interface {
	ListLogArchivePolicies(ctx context.Context) ([]store.LogArchivePolicy, error)
	RecordLogArchivePolicyRun(ctx context.Context, id, at string, watermarkNs int64, runErr string) error
	InsertLogArchiveRun(ctx context.Context, r store.LogArchiveRun) error
	FinishLogArchiveRun(ctx context.Context, r store.LogArchiveRun) error
	FailStaleLogArchiveRuns(ctx context.Context, at string) error
}

// LogReader is the node-local log store surface the archiver reads.
type LogReader interface {
	DistinctLogResources(ctx context.Context, fromNs, toNs int64) ([]string, error)
	StreamLogs(ctx context.Context, resourceID string, fromNs, toNs int64, maxLines int, fn func(telemetry.LogEntry) error) (telemetry.LogStreamResult, error)
}

// Archiver ships node-local logs to a storage destination as gzip NDJSON.
// One run at a time, so a slow bucket applies backpressure instead of
// piling up goroutines.
type Archiver struct {
	Store   ArchiveStore
	Clients *Resolver
	Logs    LogReader
	Logger  *slog.Logger
	Opts    Options
	Now     func() time.Time

	sem chan struct{}
	wg  sync.WaitGroup
}

// NewArchiver builds an Archiver.
func NewArchiver(st ArchiveStore, clients *Resolver, logs LogReader, logger *slog.Logger, opts Options) *Archiver {
	return &Archiver{Store: st, Clients: clients, Logs: logs, Logger: logger, Opts: opts, Now: time.Now, sem: make(chan struct{}, 1)}
}

// Wait blocks until every in-flight manual dump has finished.
func (a *Archiver) Wait() { a.wg.Wait() }

func (a *Archiver) acquire(ctx context.Context) error {
	select {
	case a.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("objectstore: wait for archive slot: %w", ctx.Err())
	}
}

func (a *Archiver) release() { <-a.sem }

type archiveStats struct {
	objects int
	lines   int64
	bytes   int64
}

func (s *archiveStats) add(o archiveStats) {
	s.objects += o.objects
	s.lines += o.lines
	s.bytes += o.bytes
}

type archiveLine struct {
	Resource   string          `json:"resource"`
	Stream     string          `json:"stream"`
	Timestamp  string          `json:"ts"`
	Message    string          `json:"message"`
	Structured bool            `json:"structured,omitempty"`
	Fields     json.RawMessage `json:"fields,omitempty"`
}

func toArchiveLine(e telemetry.LogEntry) archiveLine {
	l := archiveLine{Resource: e.ResourceID, Stream: e.Stream, Timestamp: e.Timestamp.UTC().Format(time.RFC3339Nano), Message: e.Message, Structured: e.Structured}
	if e.Structured && json.Valid([]byte(e.FieldsJSON)) {
		l.Fields = json.RawMessage(e.FieldsJSON)
	}
	return l
}

// archiveRange uploads resource's lines in [from, to) as one or more objects.
// Lines are spooled to a temp file first so the node-local log database is
// never held open across a network upload.
func (a *Archiver) archiveRange(ctx context.Context, cl *Client, resource string, from, to int64) (archiveStats, error) {
	var total archiveStats
	for from < to {
		st, end, err := a.archiveChunk(ctx, cl, resource, from, to)
		if err != nil {
			return total, err
		}
		total.add(st)
		if end <= from {
			break
		}
		from = end
	}
	return total, nil
}

func (a *Archiver) archiveChunk(ctx context.Context, cl *Client, resource string, from, to int64) (archiveStats, int64, error) {
	tmp, err := os.CreateTemp("", "log-archive-*.ndjson.gz")
	if err != nil {
		return archiveStats{}, 0, fmt.Errorf("objectstore: create spool file: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	gz := gzip.NewWriter(tmp)
	enc := json.NewEncoder(gz)
	res, err := a.Logs.StreamLogs(ctx, resource, from, to, a.Opts.MaxLinesPerObject, func(e telemetry.LogEntry) error {
		return enc.Encode(toArchiveLine(e))
	})
	if err != nil {
		return archiveStats{}, 0, fmt.Errorf("objectstore: read logs for %s: %w", resource, err)
	}
	if err := gz.Close(); err != nil {
		return archiveStats{}, 0, fmt.Errorf("objectstore: compress logs for %s: %w", resource, err)
	}
	if res.Lines == 0 {
		return archiveStats{}, res.EndNs, nil
	}
	size, err := tmp.Seek(0, io.SeekEnd)
	if err != nil {
		return archiveStats{}, 0, fmt.Errorf("objectstore: size spool file: %w", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return archiveStats{}, 0, fmt.Errorf("objectstore: rewind spool file: %w", err)
	}
	if err := cl.Put(ctx, ObjectKey(a.Opts.Prefix, resource, from), tmp, "application/gzip", ""); err != nil {
		return archiveStats{}, 0, err
	}
	return archiveStats{objects: 1, lines: res.Lines, bytes: size}, res.EndNs, nil
}

// archiveWindow archives one window for a scope: one app, or every resource with logs.
func (a *Archiver) archiveWindow(ctx context.Context, cl *Client, app string, from, to int64) (archiveStats, error) {
	resources := []string{ResourceID(app)}
	if app == "" {
		var err error
		if resources, err = a.Logs.DistinctLogResources(ctx, from, to); err != nil {
			return archiveStats{}, fmt.Errorf("objectstore: %w", err)
		}
	}
	var total archiveStats
	for _, r := range resources {
		st, err := a.archiveRange(ctx, cl, r, from, to)
		total.add(st)
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func (a *Archiver) stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func newRunID(now time.Time) string {
	return "lar_" + strconv.FormatInt(now.UnixNano(), 36)
}

// RunPolicy archives everything a policy has not shipped yet, up to
// MaxWindowsPerRun hour windows, then advances its watermark. A failed
// window keeps the watermark in place so the next run retries it.
func (a *Archiver) RunPolicy(ctx context.Context, p store.LogArchivePolicy) error {
	if err := a.acquire(ctx); err != nil {
		return err
	}
	defer a.release()

	now := a.Now()
	end := now.Add(-a.Opts.Lag).UnixNano()
	start := max(p.WatermarkNs, end-int64(a.Opts.MaxBackfill))
	if end <= start {
		return nil
	}

	run := store.LogArchiveRun{ID: newRunID(now), PolicyID: p.ID, AppName: p.AppName, TargetID: p.TargetID, Kind: "scheduled", FromNs: start, ToNs: start, Status: "running", StartedAt: a.stamp(now)}
	if err := a.Store.InsertLogArchiveRun(ctx, run); err != nil {
		return fmt.Errorf("objectstore: %w", err)
	}

	watermark, stats, runErr := a.runWindows(ctx, p, start, end)
	run.ToNs, run.Objects, run.Lines, run.Bytes = watermark, stats.objects, stats.lines, stats.bytes
	run.FinishedAt = a.stamp(a.Now())
	run.Status = "succeeded"
	errText := ""
	if runErr != nil {
		run.Status, run.Error = "failed", runErr.Error()
		errText = runErr.Error()
		a.Logger.Error("log archive run failed", slog.String("policy_id", p.ID), slog.String("app", p.AppName), slog.String("error", errText))
	}
	if err := a.Store.FinishLogArchiveRun(ctx, run); err != nil {
		a.Logger.Error("log archive: finish run failed", slog.String("run_id", run.ID), slog.String("error", err.Error()))
	}
	if err := a.Store.RecordLogArchivePolicyRun(ctx, p.ID, a.stamp(a.Now()), watermark, errText); err != nil {
		return fmt.Errorf("objectstore: %w", err)
	}
	return runErr
}

func (a *Archiver) runWindows(ctx context.Context, p store.LogArchivePolicy, start, end int64) (int64, archiveStats, error) {
	var stats archiveStats
	cl, _, err := a.Clients.Client(ctx, p.TargetID)
	if err != nil {
		return start, stats, err
	}
	watermark := start
	for _, w := range hourWindows(start, end, a.Opts.MaxWindowsPerRun) {
		st, err := a.archiveWindow(ctx, cl, p.AppName, w[0], w[1])
		stats.add(st)
		if err != nil {
			return watermark, stats, err
		}
		watermark = w[1]
	}
	if p.RetentionDays > 0 {
		if err := a.prune(ctx, cl, p); err != nil {
			a.Logger.Warn("log archive retention sweep failed", slog.String("policy_id", p.ID), slog.String("error", err.Error()))
		}
	}
	return watermark, stats, nil
}

// DumpRequest is a manual export of a time range.
type DumpRequest struct {
	TargetID string
	AppName  string
	From     time.Time
	To       time.Time
}

// ErrInvalidDump is returned for a bad dump request.
var ErrInvalidDump = errors.New("objectstore: invalid dump request")

// StartDump validates a manual dump, records a running row, and archives the
// range in the background. Poll the run row (or Wait) for the outcome.
func (a *Archiver) StartDump(ctx context.Context, req DumpRequest) (store.LogArchiveRun, error) {
	if !req.From.Before(req.To) {
		return store.LogArchiveRun{}, fmt.Errorf("%w: from must be before to", ErrInvalidDump)
	}
	if req.To.Sub(req.From) > a.Opts.MaxDumpRange {
		return store.LogArchiveRun{}, fmt.Errorf("%w: range is longer than %s", ErrInvalidDump, a.Opts.MaxDumpRange)
	}
	cl, _, err := a.Clients.Client(ctx, req.TargetID)
	if err != nil {
		return store.LogArchiveRun{}, err
	}
	now := a.Now()
	run := store.LogArchiveRun{ID: newRunID(now), AppName: req.AppName, TargetID: req.TargetID, Kind: "manual", FromNs: req.From.UnixNano(), ToNs: req.To.UnixNano(), Status: "running", StartedAt: a.stamp(now)}
	if err := a.Store.InsertLogArchiveRun(ctx, run); err != nil {
		return store.LogArchiveRun{}, fmt.Errorf("objectstore: %w", err)
	}
	a.wg.Add(1)
	go a.runDump(context.WithoutCancel(ctx), cl, run)
	return run, nil
}

func (a *Archiver) runDump(ctx context.Context, cl *Client, run store.LogArchiveRun) {
	defer a.wg.Done()
	if err := a.acquire(ctx); err != nil {
		return
	}
	defer a.release()

	var stats archiveStats
	var runErr error
	for _, w := range hourWindows(run.FromNs, run.ToNs, 0) {
		st, err := a.archiveWindow(ctx, cl, run.AppName, w[0], w[1])
		stats.add(st)
		if err != nil {
			runErr = err
			break
		}
	}
	run.Objects, run.Lines, run.Bytes = stats.objects, stats.lines, stats.bytes
	run.FinishedAt = a.stamp(a.Now())
	run.Status = "succeeded"
	if runErr != nil {
		run.Status, run.Error = "failed", runErr.Error()
		a.Logger.Error("log archive dump failed", slog.String("run_id", run.ID), slog.String("error", runErr.Error()))
	}
	if err := a.Store.FinishLogArchiveRun(ctx, run); err != nil {
		a.Logger.Error("log archive: finish dump failed", slog.String("run_id", run.ID), slog.String("error", err.Error()))
	}
}
