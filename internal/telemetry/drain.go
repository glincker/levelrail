package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// drainBatchMaxLines/drainBatchMaxWait bound how a DrainForwarder groups
// lines before calling a DrainSink's Send, the same batching shape
// LogCollector.StreamOne already uses for its own SQLite writes
// (logBatchMaxLines/logBatchMaxWait), sized smaller since a drain sink is
// an external network call, not a local write, and a live tail already
// exists for anyone who needs sub-second delivery.
const (
	drainBatchMaxLines = 50
	drainBatchMaxWait  = 2 * time.Second
)

// DrainSinkType selects which external sink protocol a drain uses.
// Defined locally rather than importing internal/store's LogDrainType:
// this package's own doc comments elsewhere (e.g. DesiredService's
// NodeStatus precedent) already establish "define locally, don't import
// a higher-level package's vocabulary" for cross-package enums like this.
type DrainSinkType string

// The two sink protocols buildDrainSink knows how to construct.
const (
	DrainSinkHTTP   DrainSinkType = "http"
	DrainSinkSyslog DrainSinkType = "syslog"
)

// DrainConfig is one resource's external log-forwarding configuration,
// the telemetry-package-local shape a caller's configFunc (see Run)
// resolves from wherever it actually persists (internal/store's
// LogDrain, for application services).
type DrainConfig struct {
	ResourceID string
	Type       DrainSinkType
	Target     string
	Enabled    bool
}

// DrainSink is the narrow surface a log drain forwarder needs from a
// concrete sink implementation (HTTPSink, syslog). Send should treat a
// partial failure (e.g. some lines rejected) as a whole-batch error:
// there's no per-line retry here, a failed batch is simply dropped and
// logged, the same best-effort shape LogBroadcaster.Publish's own doc
// comment already establishes for a slow live-tail subscriber.
type DrainSink interface {
	Send(ctx context.Context, entries []LogEntry) error
}

// DrainForwarder taps LogBroadcaster the same way a live SSE viewer
// does (Subscribe/Publish, broadcast.go), so it is purely an additional
// consumer of the log stream: it never touches LogCollector, WriteLogBatch,
// or QueryLogs, and a drain forwarding failure can never affect the
// node-local store or a live tail.
type DrainForwarder struct {
	broadcaster *LogBroadcaster
	logger      *slog.Logger
	buildSink   func(DrainConfig) (DrainSink, error)
}

// NewDrainForwarder builds a DrainForwarder reading from broadcaster.
// logger defaults to slog.Default() if nil. programTag is the syslog
// program tag a DrainSinkSyslog sink identifies itself with (see
// newSyslogSink); callers pass brand.Brand.ShortName, following the same
// "resolve brand outside this package" convention
// internal/reconcile/application.WithNetworkPrefix already establishes.
func NewDrainForwarder(broadcaster *LogBroadcaster, logger *slog.Logger, programTag string) *DrainForwarder {
	if logger == nil {
		logger = slog.Default()
	}
	return &DrainForwarder{
		broadcaster: broadcaster,
		logger:      logger,
		buildSink:   func(cfg DrainConfig) (DrainSink, error) { return buildDrainSink(cfg, programTag) },
	}
}

// Run derives the desired drain set via configFunc on a resync interval
// and keeps exactly one forwarding goroutine alive per currently-enabled
// ResourceID, mirroring LogCollector.Run's own reconcile-loop shape
// exactly (same active-map/start/cancel pattern) so the two collectors
// read as one family even though they serve different consumers.
func (f *DrainForwarder) Run(ctx context.Context, resync time.Duration, configFunc func(context.Context) ([]DrainConfig, error)) error {
	active := make(map[string]context.CancelFunc) // keyed by ResourceID

	reconcile := func() {
		configs, err := configFunc(ctx)
		if err != nil {
			f.logger.Error("telemetry: list log drain targets failed", slog.String("error", err.Error()))
			return
		}

		wanted := make(map[string]DrainConfig, len(configs))
		for _, c := range configs {
			if c.Enabled {
				wanted[c.ResourceID] = c
			}
		}

		for id, cancel := range active {
			if _, ok := wanted[id]; !ok {
				cancel()
				delete(active, id)
			}
		}

		for id, cfg := range wanted {
			if _, ok := active[id]; ok {
				continue
			}
			drainCtx, cancel := context.WithCancel(ctx) //nolint:gosec // cancel is stored in active and called from the two loops above, gosec's static check can't trace a func value stored in a map
			active[id] = cancel
			go f.forwardOne(drainCtx, cfg)
		}
	}

	reconcile()

	ticker := time.NewTicker(resync)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			for _, cancel := range active {
				cancel()
			}
			return ctx.Err()
		case <-ticker.C:
			reconcile()
		}
	}
}

// forwardOne subscribes to cfg.ResourceID on the broadcaster and batches
// every line it receives (drainBatchMaxLines/drainBatchMaxWait) into
// sink.Send calls, until ctx is cancelled. A sink build failure or a
// Send failure is logged and otherwise swallowed: a misconfigured or
// temporarily unreachable external sink must never affect log ingestion,
// storage, or the live tail, only this resource's own forwarding.
func (f *DrainForwarder) forwardOne(ctx context.Context, cfg DrainConfig) {
	sink, err := f.buildSink(cfg)
	if err != nil {
		f.logger.Error("telemetry: build log drain sink failed",
			slog.String("resource_id", cfg.ResourceID), slog.String("error", err.Error()))
		return
	}

	ch, unsubscribe := f.broadcaster.Subscribe(cfg.ResourceID)
	defer unsubscribe()

	ticker := time.NewTicker(drainBatchMaxWait)
	defer ticker.Stop()

	batch := make([]LogEntry, 0, drainBatchMaxLines)
	flush := func(sendCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		if err := sink.Send(sendCtx, batch); err != nil {
			f.logger.Error("telemetry: log drain send failed",
				slog.String("resource_id", cfg.ResourceID), slog.String("error", err.Error()))
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			flush(flushCtx)
			cancel()
			return
		case entry, ok := <-ch:
			if !ok {
				flush(ctx)
				return
			}
			batch = append(batch, entry)
			if len(batch) >= drainBatchMaxLines {
				flush(ctx)
			}
		case <-ticker.C:
			flush(ctx)
		}
	}
}

// buildDrainSink is DrainForwarder's default sink factory, overridable
// in tests via the buildSink field. programTag is only used by the
// syslog sink; see newSyslogSink.
func buildDrainSink(cfg DrainConfig, programTag string) (DrainSink, error) {
	switch cfg.Type {
	case DrainSinkHTTP:
		if cfg.Target == "" {
			return nil, errors.New("telemetry: http log drain requires a target URL")
		}
		return NewHTTPSink(cfg.Target, nil), nil
	case DrainSinkSyslog:
		return newSyslogSink(cfg.Target, programTag)
	default:
		return nil, fmt.Errorf("telemetry: unknown log drain sink type %q", cfg.Type)
	}
}
