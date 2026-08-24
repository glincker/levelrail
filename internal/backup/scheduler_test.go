package backup

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeScheduleStore is Scheduler's own fake, the same shape
// fakeHistoryStore already establishes for Runner's tests: a struct
// literal a test configures directly, no mocking framework.
type fakeScheduleStore struct {
	mu      sync.Mutex
	dbs     []store.DesiredDatabase
	listErr error

	pruned []struct {
		database  string
		keep      int
		olderThan time.Time
	}
	pruneReturn []store.PrunedBackup
	pruneErr    error
}

func (f *fakeScheduleStore) ListScheduledDatabases(context.Context) ([]store.DesiredDatabase, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.dbs, nil
}

func (f *fakeScheduleStore) PruneBackupHistory(_ context.Context, databaseName string, keep int, olderThan time.Time) ([]store.PrunedBackup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pruned = append(f.pruned, struct {
		database  string
		keep      int
		olderThan time.Time
	}{databaseName, keep, olderThan})
	if f.pruneErr != nil {
		return nil, f.pruneErr
	}
	return f.pruneReturn, nil
}

// fakeScheduledRunner is Scheduler's own fake for ScheduledBackupRunner:
// records every call, and can be told to fail either always or for
// specific database names.
type fakeScheduledRunner struct {
	mu    sync.Mutex
	calls []struct {
		historyID, database, engine, container, targetID string
	}
	failFor map[string]error

	resolveCalls  []string
	resolveErr    error
	resolveResult Destination
}

func (f *fakeScheduledRunner) RunBackup(_ context.Context, historyID, databaseName, engine, containerName, targetID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, struct {
		historyID, database, engine, container, targetID string
	}{historyID, databaseName, engine, containerName, targetID})
	if f.failFor != nil {
		if err, ok := f.failFor[databaseName]; ok {
			return err
		}
	}
	return nil
}

func (f *fakeScheduledRunner) ResolveDestination(_ context.Context, targetID string) (Destination, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolveCalls = append(f.resolveCalls, targetID)
	if f.resolveErr != nil {
		return Destination{}, f.resolveErr
	}
	return f.resolveResult, nil
}

// fakeDeleter is Scheduler's own fake for Deleter: records every call,
// and can be told to fail so tests can prove a delete failure is logged
// rather than turning a successful run into a reported failure.
type fakeDeleter struct {
	mu    sync.Mutex
	calls []struct {
		dest Destination
		key  string
	}
	err error
}

func (f *fakeDeleter) Delete(_ context.Context, dest Destination, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, struct {
		dest Destination
		key  string
	}{dest, key})
	return f.err
}

func scheduledDB(name, schedule string, retain int) store.DesiredDatabase {
	return store.DesiredDatabase{
		Name: name, Engine: store.EnginePostgres, Version: "16",
		BackupTargetID: "bkt_1", BackupSchedule: schedule, BackupRetain: retain,
	}
}

func scheduledDBWithRetainDays(name, schedule string, retain, retainDays int) store.DesiredDatabase {
	d := scheduledDB(name, schedule, retain)
	d.BackupRetainDays = retainDays
	return d
}

// TestScheduler_Tick_FirstSightingArmsWithoutFiring is the "no catch-up
// on control-plane startup" behavior this package's own Scheduler doc
// comment documents as deliberate: the very first tick a schedule is
// observed must never itself count as a due fire.
func TestScheduler_Tick_FirstSightingArmsWithoutFiring(t *testing.T) {
	// "0 3 * * *" due at 03:00 UTC daily; now is exactly 03:00, which
	// would be due on a second tick, but must not fire on the first.
	now := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	fakeStore := &fakeScheduleStore{dbs: []store.DesiredDatabase{scheduledDB("main", "0 3 * * *", 0)}}
	runner := &fakeScheduledRunner{}
	s := NewScheduler(fakeStore, runner, nil, nil)
	s.Now = func() time.Time { return now }

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("first tick fired %d backups, want 0 (arm only)", len(runner.calls))
	}
}

// TestScheduler_Tick_FiresOnceDue: after the first arming tick, a
// subsequent tick at or after the armed next-run time must fire exactly
// once, calling Runner.RunBackup with the database's own engine,
// container name, and target ID.
func TestScheduler_Tick_FiresOnceDue(t *testing.T) {
	fakeStore := &fakeScheduleStore{dbs: []store.DesiredDatabase{scheduledDB("main", "0 3 * * *", 0)}}
	runner := &fakeScheduledRunner{}
	s := NewScheduler(fakeStore, runner, nil, nil)

	armAt := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return armAt }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("arm tick error = %v", err)
	}

	dueAt := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return dueAt }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("due tick error = %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("due tick fired %d backups, want 1", len(runner.calls))
	}
	call := runner.calls[0]
	if call.database != "main" || call.engine != store.EnginePostgres || call.container != "db-main" || call.targetID != "bkt_1" {
		t.Errorf("RunBackup call = %+v, want database=main engine=postgres container=db-main targetID=bkt_1", call)
	}

	// A third tick at the same due time must not fire again: Tick armed
	// the next occurrence (2026-08-16 03:00) before running, so "now"
	// staying at 2026-08-15 03:00 is before that next occurrence.
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("repeat tick error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("repeat tick at the same due time fired again, calls = %d, want 1", len(runner.calls))
	}
}

// TestScheduler_Tick_NotYetDueDoesNotFire covers the ordinary "armed but
// the clock hasn't reached it yet" case between the two ticks
// TestScheduler_Tick_FiresOnceDue exercises at their boundaries.
func TestScheduler_Tick_NotYetDueDoesNotFire(t *testing.T) {
	fakeStore := &fakeScheduleStore{dbs: []store.DesiredDatabase{scheduledDB("main", "0 3 * * *", 0)}}
	runner := &fakeScheduledRunner{}
	s := NewScheduler(fakeStore, runner, nil, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("arm tick error = %v", err)
	}

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 2, 59, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("not-yet-due tick error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("fired before the schedule was due, calls = %d, want 0", len(runner.calls))
	}
}

// TestScheduler_Tick_RetentionPrunedAfterSuccess: a successful scheduled
// run with BackupRetain > 0 must call PruneBackupHistory with that exact
// retain count.
func TestScheduler_Tick_RetentionPrunedAfterSuccess(t *testing.T) {
	fakeStore := &fakeScheduleStore{dbs: []store.DesiredDatabase{scheduledDB("main", "0 3 * * *", 7)}}
	runner := &fakeScheduledRunner{}
	s := NewScheduler(fakeStore, runner, nil, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	_ = s.Tick(context.Background())
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	if len(fakeStore.pruned) != 1 {
		t.Fatalf("prune calls = %d, want 1", len(fakeStore.pruned))
	}
	if fakeStore.pruned[0].database != "main" || fakeStore.pruned[0].keep != 7 {
		t.Errorf("prune call = %+v, want database=main keep=7", fakeStore.pruned[0])
	}
}

// TestScheduler_Tick_DeletesPrunedObjects proves the actual gap this
// feature closes: every row PruneBackupHistory returns must have its
// bucket object deleted too, via Runner.ResolveDestination + Deleter,
// not just the backup_history row.
func TestScheduler_Tick_DeletesPrunedObjects(t *testing.T) {
	fakeStore := &fakeScheduleStore{
		dbs: []store.DesiredDatabase{scheduledDB("main", "0 3 * * *", 7)},
		pruneReturn: []store.PrunedBackup{
			{TargetID: "bkt_1", ObjectKey: "main/main-1.dump"},
			{TargetID: "bkt_1", ObjectKey: "main/main-2.dump"},
		},
	}
	runner := &fakeScheduledRunner{resolveResult: Destination{Bucket: "levelrail-backups"}}
	deleter := &fakeDeleter{}
	s := NewScheduler(fakeStore, runner, deleter, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	_ = s.Tick(context.Background())
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	if len(deleter.calls) != 2 {
		t.Fatalf("delete calls = %d, want 2", len(deleter.calls))
	}
	if deleter.calls[0].key != "main/main-1.dump" || deleter.calls[1].key != "main/main-2.dump" {
		t.Errorf("delete calls = %+v, want keys main/main-1.dump then main/main-2.dump", deleter.calls)
	}
	if deleter.calls[0].dest.Bucket != "levelrail-backups" {
		t.Errorf("delete dest = %+v, want the resolved Destination, not a zero value", deleter.calls[0].dest)
	}
	// Both pruned rows share targetID bkt_1, so ResolveDestination
	// should be resolved once and reused, not called per row.
	if len(runner.resolveCalls) != 1 || runner.resolveCalls[0] != "bkt_1" {
		t.Errorf("resolveCalls = %+v, want exactly one call for bkt_1 (cached across rows sharing a target)", runner.resolveCalls)
	}
}

// TestScheduler_Tick_DeleteFailureDoesNotFailTick is the half-succeeded
// case this feature must handle: the backup ran, the DB rows were
// pruned, but deleting the bucket object failed. That failure must be
// logged, never turn an already-successful backup+prune into a reported
// Tick error.
func TestScheduler_Tick_DeleteFailureDoesNotFailTick(t *testing.T) {
	fakeStore := &fakeScheduleStore{
		dbs:         []store.DesiredDatabase{scheduledDB("main", "0 3 * * *", 7)},
		pruneReturn: []store.PrunedBackup{{TargetID: "bkt_1", ObjectKey: "main/main-1.dump"}},
	}
	runner := &fakeScheduledRunner{}
	deleter := &fakeDeleter{err: errors.New("bucket unreachable")}
	s := NewScheduler(fakeStore, runner, deleter, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	_ = s.Tick(context.Background())
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil (a delete failure must not fail Tick)", err)
	}

	if len(deleter.calls) != 1 {
		t.Fatalf("delete calls = %d, want 1 (attempted despite the eventual failure)", len(deleter.calls))
	}
}

// TestScheduler_Tick_ResolveDestinationFailureDoesNotFailTick covers the
// other half of resolving a target before deleting: a failure to resolve
// credentials for a pruned row's target must also be logged and
// swallowed, not propagated as a Tick error.
func TestScheduler_Tick_ResolveDestinationFailureDoesNotFailTick(t *testing.T) {
	fakeStore := &fakeScheduleStore{
		dbs:         []store.DesiredDatabase{scheduledDB("main", "0 3 * * *", 7)},
		pruneReturn: []store.PrunedBackup{{TargetID: "bkt_1", ObjectKey: "main/main-1.dump"}},
	}
	runner := &fakeScheduledRunner{resolveErr: errors.New("target not found")}
	deleter := &fakeDeleter{}
	s := NewScheduler(fakeStore, runner, deleter, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	_ = s.Tick(context.Background())
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil (a resolve failure must not fail Tick)", err)
	}
	if len(deleter.calls) != 0 {
		t.Fatalf("delete calls = %d, want 0 (never called once its destination failed to resolve)", len(deleter.calls))
	}
}

// TestScheduler_Tick_NilDeleterSkipsDeleteWithoutFailing: Deleter is an
// optional capability (see Scheduler.Deleter's own doc comment); a
// Scheduler with none configured must still prune store rows fine and
// must not panic or fail Tick trying to delete objects.
func TestScheduler_Tick_NilDeleterSkipsDeleteWithoutFailing(t *testing.T) {
	fakeStore := &fakeScheduleStore{
		dbs:         []store.DesiredDatabase{scheduledDB("main", "0 3 * * *", 7)},
		pruneReturn: []store.PrunedBackup{{TargetID: "bkt_1", ObjectKey: "main/main-1.dump"}},
	}
	s := NewScheduler(fakeStore, &fakeScheduledRunner{}, nil, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	_ = s.Tick(context.Background())
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil", err)
	}
	if len(fakeStore.pruned) != 1 {
		t.Fatalf("prune calls = %d, want 1 (store pruning must still happen with no Deleter configured)", len(fakeStore.pruned))
	}
}

// TestScheduler_Tick_RetainZeroSkipsPrune: BackupRetain 0 means "keep
// everything," so a successful run must never call PruneBackupHistory at
// all, not call it with keep=0 and rely on that method's own no-op
// behavior; Scheduler owns this decision at its own layer too, the same
// belt-and-suspenders shape PruneBackupHistory's own "keep <= 0 deletes
// nothing" guard already provides.
func TestScheduler_Tick_RetainZeroSkipsPrune(t *testing.T) {
	fakeStore := &fakeScheduleStore{dbs: []store.DesiredDatabase{scheduledDB("main", "0 3 * * *", 0)}}
	runner := &fakeScheduledRunner{}
	s := NewScheduler(fakeStore, runner, nil, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	_ = s.Tick(context.Background())
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	if len(fakeStore.pruned) != 0 {
		t.Fatalf("prune calls = %d, want 0 (retain=0 means keep everything)", len(fakeStore.pruned))
	}
}

// TestScheduler_Tick_RetainDaysOnlyPrunesByAge: BackupRetain 0 with
// BackupRetainDays > 0 must still trigger a prune call, keep=0 (no count
// limit) and olderThan set to now minus BackupRetainDays.
func TestScheduler_Tick_RetainDaysOnlyPrunesByAge(t *testing.T) {
	fakeStore := &fakeScheduleStore{dbs: []store.DesiredDatabase{scheduledDBWithRetainDays("main", "0 3 * * *", 0, 30)}}
	runner := &fakeScheduledRunner{}
	s := NewScheduler(fakeStore, runner, nil, nil)

	runAt := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	_ = s.Tick(context.Background())
	s.Now = func() time.Time { return runAt }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	if len(fakeStore.pruned) != 1 {
		t.Fatalf("prune calls = %d, want 1", len(fakeStore.pruned))
	}
	want := runAt.AddDate(0, 0, -30)
	if fakeStore.pruned[0].keep != 0 || !fakeStore.pruned[0].olderThan.Equal(want) {
		t.Errorf("prune call = %+v, want keep=0 olderThan=%v", fakeStore.pruned[0], want)
	}
}

// TestScheduler_Tick_RetainAndRetainDaysBothSet: both dimensions set must
// both reach PruneBackupHistory in the same call, letting the store
// layer enforce "violates either limit."
func TestScheduler_Tick_RetainAndRetainDaysBothSet(t *testing.T) {
	fakeStore := &fakeScheduleStore{dbs: []store.DesiredDatabase{scheduledDBWithRetainDays("main", "0 3 * * *", 7, 30)}}
	runner := &fakeScheduledRunner{}
	s := NewScheduler(fakeStore, runner, nil, nil)

	runAt := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	_ = s.Tick(context.Background())
	s.Now = func() time.Time { return runAt }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	if len(fakeStore.pruned) != 1 {
		t.Fatalf("prune calls = %d, want 1", len(fakeStore.pruned))
	}
	want := runAt.AddDate(0, 0, -30)
	if fakeStore.pruned[0].keep != 7 || !fakeStore.pruned[0].olderThan.Equal(want) {
		t.Errorf("prune call = %+v, want keep=7 olderThan=%v", fakeStore.pruned[0], want)
	}
}

// TestScheduler_Tick_FailedRunSkipsPrune: RunBackup failing must not
// trigger a retention prune, and must surface as a Tick error rather
// than being silently swallowed.
func TestScheduler_Tick_FailedRunSkipsPrune(t *testing.T) {
	fakeStore := &fakeScheduleStore{dbs: []store.DesiredDatabase{scheduledDB("main", "0 3 * * *", 7)}}
	runner := &fakeScheduledRunner{failFor: map[string]error{"main": errors.New("dump failed")}}
	s := NewScheduler(fakeStore, runner, nil, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	_ = s.Tick(context.Background())
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	err := s.Tick(context.Background())
	if err == nil {
		t.Fatal("Tick() error = nil, want a wrapped RunBackup failure")
	}

	if len(fakeStore.pruned) != 0 {
		t.Fatalf("prune calls = %d, want 0 after a failed run", len(fakeStore.pruned))
	}
}

// TestScheduler_Tick_InvalidScheduleSkipsWithoutFailingTick: an
// unparseable cron string must be logged and skipped, and must not stop
// evaluation of other, valid, scheduled databases in the same tick.
func TestScheduler_Tick_InvalidScheduleSkipsWithoutFailingTick(t *testing.T) {
	fakeStore := &fakeScheduleStore{dbs: []store.DesiredDatabase{
		scheduledDB("broken", "not a cron string", 0),
		scheduledDB("ok", "0 3 * * *", 0),
	}}
	runner := &fakeScheduledRunner{}
	s := NewScheduler(fakeStore, runner, nil, nil)
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil (invalid schedules are skipped, not fatal)", err)
	}
	// Neither has fired yet (both just armed on this first sighting),
	// but the "ok" database must have a nextRun entry recorded, proving
	// its evaluation wasn't short-circuited by "broken" earlier in the
	// list.
	s.mu.Lock()
	_, armed := s.nextRun["ok"]
	_, brokenArmed := s.nextRun["broken"]
	s.mu.Unlock()
	if !armed {
		t.Error("valid schedule 'ok' was not armed after a tick containing an invalid schedule earlier in the list")
	}
	if brokenArmed {
		t.Error("invalid schedule 'broken' should never be armed")
	}
}

// TestScheduler_Tick_ForgetsUnscheduledDatabase: once a database drops
// out of ListScheduledDatabases (schedule cleared, database deleted, or
// its target removed), Scheduler must forget its armed nextRun, not
// leak memory or resurrect a stale time if the same name is scheduled
// again later.
func TestScheduler_Tick_ForgetsUnscheduledDatabase(t *testing.T) {
	fakeStore := &fakeScheduleStore{dbs: []store.DesiredDatabase{scheduledDB("main", "0 3 * * *", 0)}}
	runner := &fakeScheduledRunner{}
	s := NewScheduler(fakeStore, runner, nil, nil)
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	s.mu.Lock()
	_, armed := s.nextRun["main"]
	s.mu.Unlock()
	if !armed {
		t.Fatal("expected 'main' to be armed after its first tick")
	}

	fakeStore.dbs = nil
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	s.mu.Lock()
	_, stillArmed := s.nextRun["main"]
	s.mu.Unlock()
	if stillArmed {
		t.Error("'main' should have been forgotten once it dropped out of ListScheduledDatabases")
	}
}

// TestScheduler_Tick_ListErrorPropagates: a store failure to even list
// scheduled databases must surface as Tick's own error, not be silently
// swallowed the way an individual database's invalid schedule is.
func TestScheduler_Tick_ListErrorPropagates(t *testing.T) {
	fakeStore := &fakeScheduleStore{listErr: errors.New("db unavailable")}
	s := NewScheduler(fakeStore, &fakeScheduledRunner{}, nil, nil)

	if err := s.Tick(context.Background()); err == nil {
		t.Fatal("Tick() error = nil, want the store's list error wrapped")
	}
}

// TestScheduler_Run_StopsOnContextCancel confirms Run's ticker loop
// exits cleanly with ctx.Err() once its context is cancelled, the same
// shape alerting.Engine.Run and telemetry.Collector.Run's own tests (if
// any) would expect from this codebase's shared periodic-loop
// convention.
func TestScheduler_Run_StopsOnContextCancel(t *testing.T) {
	fakeStore := &fakeScheduleStore{}
	s := NewScheduler(fakeStore, &fakeScheduledRunner{}, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx, time.Millisecond) }()

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}
