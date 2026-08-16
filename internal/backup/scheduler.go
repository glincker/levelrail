package backup

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/cronexpr"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ScheduleStore is the narrow store surface Scheduler needs: which
// databases currently have a due-able cron schedule, re-derived fresh
// on every tick, and the retention prune a successful scheduled run
// triggers afterward. *store.DB satisfies this structurally, the same
// "consumer defines its own narrow interface" shape HistoryStore
// (runner.go) already establishes for this package's other store
// dependency.
type ScheduleStore interface {
	ListScheduledDatabases(ctx context.Context) ([]store.DesiredDatabase, error)
	PruneBackupHistory(ctx context.Context, databaseName string, keep int) ([]store.PrunedBackup, error)
}

// ScheduledBackupRunner is the surface Scheduler needs to actually run a
// backup once its schedule comes due: identical in shape to
// internal/api's own BackupRunner interface (internal/api/backups.go),
// redeclared here rather than imported across the package boundary,
// the same reasoning that file's own BackupRunner doc comment gives for
// why internal/api never imports internal/backup directly. *Runner
// satisfies this structurally, so cmd/levelrail/main.go wires the exact
// same *Runner instance into both api.WithBackupRunner (the manual
// trigger path) and NewScheduler (this one): one backup implementation,
// two ways to invoke it, exactly the design internal/spec.Backup's own
// doc comment implies by giving app.yaml a single databases[].backup
// block rather than separate manual/scheduled config shapes.
type ScheduledBackupRunner interface {
	RunBackup(ctx context.Context, historyID, databaseName, engine, containerName, targetID string) error
	// ResolveDestination resolves a backup target ID into a live
	// Destination (*Runner.ResolveDestination, runner.go), the same
	// resolution RunBackup does internally before every upload. Needed
	// separately here because retention pruning deletes objects for a
	// pruned row's own TargetID after a run, not during one.
	ResolveDestination(ctx context.Context, targetID string) (Destination, error)
}

// Scheduler periodically checks which databases have a due cron
// schedule (store.DesiredDatabase's BackupSchedule/BackupTargetID/
// BackupRetain, migrations/0023_scheduled_backups.sql) and runs a
// backup through the same Runner the manual trigger path
// (internal/api's handleTriggerBackup) uses, recording the attempt in
// store.BackupHistory exactly like a manual trigger does, since both
// paths call the identical Runner.RunBackup.
//
// Tick's own shape (list what to evaluate, act, persist any resulting
// state, join and return errors rather than stopping at the first one)
// deliberately mirrors internal/alerting.Engine.Tick: that is this
// codebase's own established shape for "evaluate every enabled X on an
// interval, one broken X must never block the rest," and a cron
// schedule check is exactly that same shape applied to a different
// resource. Run's own ticker loop is the identical pattern
// internal/alerting.Engine.Run, internal/telemetry.Collector.Run, and
// every other periodic loop in cmd/levelrail/main.go already use.
//
// Unlike alerting.Engine.Tick's own per-rule dispatch, a due backup runs
// synchronously inside Tick, not fired off into its own goroutine the
// way internal/api's handleTriggerBackup dispatches one: that handler's
// goroutine exists specifically because an HTTP response must return in
// milliseconds, a constraint that does not apply to a background ticker
// loop already running off the request path. Running sequentially here
// also means a database's backup finishing (or failing) is fully
// resolved, including its retention prune, before Tick moves on to
// evaluate the next database, which keeps this method's own control flow
// as a flat, easy-to-reason-about loop instead of needing a WaitGroup or
// a results channel to know when a tick's work is actually done.
//
// Missed-schedule handling is a known, deliberate, and documented gap,
// not an oversight: Scheduler tracks each database's next fire time
// purely in memory (nextRun below), armed fresh the first tick a
// schedule is observed rather than reconstructed from
// store.BackupHistory on startup. A schedule that should have fired
// while the control plane was down does not "catch up" the moment it
// restarts; it simply waits for its next naturally occurring occurrence.
// This mirrors the honesty this codebase already extends to a related
// gap on the record-keeping side (store.BackupHistory's own status
// constants doc comment: "nothing currently sweeps a stuck running row
// back to failed... a known, honest gap for the eventual scheduler to
// close alongside its own retry logic, not invented speculatively
// here"). Building real catch-up/retry semantics is a materially bigger
// feature (it needs to decide how many missed runs to catch up on, how
// to avoid a thundering herd of backups firing at once after a long
// outage, and how that interacts with retention) than this task's scope
// asked for, so it is left as a follow-up rather than guessed at here.
type Scheduler struct {
	Store  ScheduleStore
	Runner ScheduledBackupRunner
	// Deleter removes the bucket object behind each pruned backup_history
	// row after a successful retention prune. nil disables object
	// deletion entirely (store rows still get pruned), the same
	// optional-capability shape this codebase already uses elsewhere
	// (e.g. backupRunner itself being nil when no master key is
	// configured, cmd/levelrail/main.go).
	Deleter Deleter
	Logger  *slog.Logger
	// Now returns the current time, the same testable-clock field
	// Runner.Now already establishes in runner.go: production code
	// leaves it nil and Tick falls back to time.Now, tests set it for
	// deterministic schedule matching.
	Now func() time.Time

	mu      sync.Mutex
	nextRun map[string]time.Time
}

// NewScheduler builds a Scheduler ready to Tick or Run. logger defaults
// to slog.Default() if nil, the same convention alerting.NewEngine
// already establishes for its own logger parameter. deleter may be nil
// (see Scheduler.Deleter's own doc comment).
func NewScheduler(store ScheduleStore, runner ScheduledBackupRunner, deleter Deleter, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		Store:   store,
		Runner:  runner,
		Deleter: deleter,
		Logger:  logger,
		nextRun: make(map[string]time.Time),
	}
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Scheduler) log() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

// Tick evaluates every currently scheduled database once. See this
// struct's own doc comment for the overall shape and the missed-schedule
// gap this deliberately leaves open. Errors from an individual database
// (an invalid cron string, RunBackup itself failing, a retention prune
// failing) are collected and joined, never stopping evaluation of the
// remaining databases, the same "one broken resource must not block the
// rest" principle Tick's own doc comment traces back to
// alerting.Engine.Tick.
func (s *Scheduler) Tick(ctx context.Context) error {
	dbs, err := s.Store.ListScheduledDatabases(ctx)
	if err != nil {
		return fmt.Errorf("backup: scheduler: list scheduled databases: %w", err)
	}

	now := s.now()
	seen := make(map[string]struct{}, len(dbs))
	var errs []error

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, d := range dbs {
		seen[d.Name] = struct{}{}

		sched, perr := cronexpr.Parse(d.BackupSchedule)
		if perr != nil {
			// A schedule string that fails to parse here passed no
			// validation at write time (internal/api's own
			// handleSetBackupSchedule parses before saving), so this
			// should only actually happen if a row was written some
			// other way; logged and skipped rather than treated as a
			// Tick-ending failure, matching EvaluateThreshold's own
			// "warn and skip" handling of an unknown rule kind in
			// alerting.Engine.Tick.
			s.log().Warn("backup: scheduler: invalid schedule, skipping",
				slog.String("database", d.Name), slog.String("schedule", d.BackupSchedule), slog.String("error", perr.Error()))
			continue
		}

		next, known := s.nextRun[d.Name]
		if !known {
			// First tick this database has been observed as scheduled:
			// arm its next fire time without running immediately. See
			// this struct's own doc comment for why a freshly-armed
			// schedule never fires on sight, whether that is because the
			// schedule was just created or because the control plane
			// just started up.
			s.nextRun[d.Name] = sched.Next(now)
			continue
		}
		if now.Before(next) {
			continue
		}

		// Armed before running, not after: if RunBackup itself panics or
		// this process is killed mid-backup, the next tick still moves
		// forward to the schedule's next real occurrence instead of
		// retrying immediately in a tight loop against, most likely, the
		// same failure.
		s.nextRun[d.Name] = sched.Next(now)

		if rerr := s.runScheduled(ctx, d); rerr != nil {
			errs = append(errs, fmt.Errorf("database %q: %w", d.Name, rerr))
		}
	}

	// Forget any database no longer in the scheduled set (schedule
	// cleared, database deleted, or its backup target was removed out
	// from under it, see migrations/0023's own ON DELETE SET NULL
	// comment): otherwise nextRun would grow forever and a schedule
	// re-added later under the same database name would incorrectly
	// inherit a stale armed time from before it was cleared.
	for name := range s.nextRun {
		if _, ok := seen[name]; !ok {
			delete(s.nextRun, name)
		}
	}

	return errors.Join(errs...)
}

// runScheduled runs one database's due backup and, on success, prunes
// its backup_history rows back down to BackupRetain (0 means keep
// everything, see PruneBackupHistory's own doc comment), then
// best-effort deletes the pruned rows' bucket objects via Deleter. A
// pruning failure, or an individual object delete failure, is logged,
// not returned as this call's own error: the backup itself already
// succeeded and is already durably recorded, so losing the chance to
// clean up old rows or objects this one time is a materially smaller
// problem than a backup that ran fine somehow being reported as failed
// because bookkeeping cleanup afterward didn't go perfectly, the same
// "don't let a secondary concern's failure mask the primary result"
// reasoning Engine.dispatch's own doc comment applies to a notification
// failing after a rule's state was already persisted successfully.
func (s *Scheduler) runScheduled(ctx context.Context, d store.DesiredDatabase) error {
	historyID, err := randomScheduledBackupHistoryID()
	if err != nil {
		return fmt.Errorf("generate history id: %w", err)
	}

	containerName := scheduledBackupContainerName(d.Name)
	if runErr := s.Runner.RunBackup(ctx, historyID, d.Name, d.Engine, containerName, d.BackupTargetID); runErr != nil {
		s.log().Error("backup: scheduled run failed",
			slog.String("database", d.Name), slog.String("id", historyID), slog.String("error", runErr.Error()))
		return runErr
	}

	if d.BackupRetain > 0 {
		pruned, pruneErr := s.Store.PruneBackupHistory(ctx, d.Name, d.BackupRetain)
		if pruneErr != nil {
			s.log().Error("backup: scheduled retention prune failed",
				slog.String("database", d.Name), slog.String("error", pruneErr.Error()))
			return nil
		}
		if len(pruned) > 0 {
			s.log().Info("backup: scheduled retention pruned old backup history rows",
				slog.String("database", d.Name), slog.Int("deleted", len(pruned)))
			s.deletePrunedObjects(ctx, d.Name, pruned)
		}
	}

	return nil
}

// deletePrunedObjects best-effort deletes the bucket object behind every
// row PruneBackupHistory just removed: matching this method's own caller
// (runScheduled), a failure here is logged and never turns an
// already-successful backup+prune into a reported failure. Destinations
// are resolved once per distinct TargetID and reused across rows sharing
// it, since a database's schedule normally points at one target for its
// whole pruned batch.
func (s *Scheduler) deletePrunedObjects(ctx context.Context, databaseName string, pruned []store.PrunedBackup) {
	if s.Deleter == nil {
		return
	}

	destinations := make(map[string]Destination, 1)
	for _, p := range pruned {
		dest, ok := destinations[p.TargetID]
		if !ok {
			var err error
			dest, err = s.Runner.ResolveDestination(ctx, p.TargetID)
			if err != nil {
				s.log().Error("backup: scheduled retention delete: resolve destination failed",
					slog.String("database", databaseName), slog.String("target_id", p.TargetID), slog.String("error", err.Error()))
				continue
			}
			destinations[p.TargetID] = dest
		}

		if err := s.Deleter.Delete(ctx, dest, p.ObjectKey); err != nil {
			s.log().Error("backup: scheduled retention delete failed",
				slog.String("database", databaseName), slog.String("object_key", p.ObjectKey), slog.String("error", err.Error()))
		}
	}
}

// Run calls Tick on interval until ctx is done, matching the shape of
// every other periodic loop in this codebase (alerting.Engine.Run,
// telemetry.Collector.Run, and cmd/levelrail/main.go's own retention
// sweeps all follow this identical select-on-ticker-or-ctx-Done shape).
func (s *Scheduler) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil {
				s.log().Warn("backup: scheduler tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}

// scheduledBackupContainerName mirrors internal/api/backups.go's own
// databaseContainerName exactly ("db-" + name): the same
// duplicate-the-format-instead-of-importing-across-the-boundary tradeoff
// that function's own doc comment explains, applied to this package's
// symmetric situation (Scheduler needs the container name RunBackup
// itself expects, the same as handleTriggerBackup does, but importing
// internal/api from internal/backup to reuse the one function would
// invert this package's own dependency direction: internal/api already
// depends on internal/backup, not the other way around).
func scheduledBackupContainerName(dbName string) string {
	return "db-" + dbName
}

// randomScheduledBackupHistoryID mirrors internal/api/backups.go's own
// randomBackupHistoryID exactly (9 random bytes, URL-safe base64, the
// same "bkh_" prefix): a scheduled run and a manually triggered one
// write to the identical backup_history table and should be visually
// indistinguishable as backup history entries, since from
// store.BackupHistory's own perspective they are the same kind of row,
// only the trigger differs. Duplicated rather than imported, the same
// boundary reasoning scheduledBackupContainerName's own doc comment
// gives.
func randomScheduledBackupHistoryID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("backup: generate scheduled backup history id: %w", err)
	}
	return "bkh_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
