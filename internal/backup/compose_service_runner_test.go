package backup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeComposeRuns struct {
	started  []store.ComposeServiceBackupRun
	finished []string
	lastErr  string
	size     int64
}

func (f *fakeComposeRuns) StartComposeServiceBackupRun(_ context.Context, r store.ComposeServiceBackupRun) error {
	f.started = append(f.started, r)
	return nil
}

func (f *fakeComposeRuns) FinishComposeServiceBackupRun(_ context.Context, _, status string, size int64, _, errMsg, _ string) error {
	f.finished = append(f.finished, status)
	f.lastErr, f.size = errMsg, size
	return nil
}

type fakeComposeContainers struct {
	name string
	err  error
	got  [2]string
}

func (f *fakeComposeContainers) ResolveComposeContainer(_ context.Context, app, service string) (string, error) {
	f.got = [2]string{app, service}
	return f.name, f.err
}

func newComposeRunnerForTest(t *testing.T, containers *fakeComposeContainers, up *fakeUploader) (*ComposeServiceRunner, *fakeComposeRuns) {
	t.Helper()
	t.Setenv("APP_DATA_DIR", "")
	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": {ID: "bkt_test", Provider: store.BackupProviderR2, Endpoint: "https://e.example", Bucket: "b"}}}
	runner := &Runner{Store: hs, Secrets: &fakeSecrets{values: map[string]string{
		store.BackupTargetSecretsKey("bkt_test") + "/access_key_id":     "k",
		store.BackupTargetSecretsKey("bkt_test") + "/secret_access_key": "s",
	}}, Uploader: up, Dumper: &fakeDumper{content: "should-not-be-used"}}
	runs := &fakeComposeRuns{}
	return &ComposeServiceRunner{
		Runner: runner, Runs: runs, Containers: containers, Dumper: &fakeDumper{content: "pg dump body"},
		Now: func() time.Time { return time.Date(2026, 11, 8, 3, 0, 0, 0, time.UTC) },
	}, runs
}

func TestComposeServiceRunner_Success(t *testing.T) {
	up := &fakeUploader{}
	cont := &fakeComposeContainers{name: "shop-db-abcd1234"}
	c, runs := newComposeRunnerForTest(t, cont, up)

	if err := c.RunComposeServiceBackup(context.Background(), "csr_1", "shop", "db", store.EnginePostgres, "bkt_test"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if cont.got != [2]string{"shop", "db"} {
		t.Errorf("resolved %v", cont.got)
	}
	if up.gotBody != "pg dump body" {
		t.Errorf("uploaded %q, want the compose dumper output", up.gotBody)
	}
	if !strings.HasPrefix(up.gotKey, "compose/shop/db/db-20261108T030000Z.dump") {
		t.Errorf("object key = %q", up.gotKey)
	}
	if len(runs.started) != 1 || runs.started[0].ObjectKey != up.gotKey || len(runs.finished) != 1 || runs.finished[0] != store.BackupStatusSucceeded {
		t.Errorf("run history = %+v / %v", runs.started, runs.finished)
	}
}

func TestComposeServiceRunner_ContainerMissingRecordsFailure(t *testing.T) {
	c, runs := newComposeRunnerForTest(t, &fakeComposeContainers{err: ErrComposeContainerNotRunning}, &fakeUploader{})

	err := c.RunComposeServiceBackup(context.Background(), "csr_2", "shop", "db", store.EnginePostgres, "bkt_test")
	if !errors.Is(err, ErrComposeContainerNotRunning) {
		t.Fatalf("err = %v, want ErrComposeContainerNotRunning", err)
	}
	if len(runs.finished) != 1 || runs.finished[0] != store.BackupStatusFailed || runs.lastErr == "" {
		t.Errorf("failed run not recorded: %v %q", runs.finished, runs.lastErr)
	}
}

type fakeComposeScheduleStore struct {
	*fakeScheduleStore
	rows   []store.ComposeServiceBackup
	pruned []string
}

func (f *fakeComposeScheduleStore) ListScheduledComposeServices(context.Context) ([]store.ComposeServiceBackup, error) {
	return f.rows, nil
}

func (f *fakeComposeScheduleStore) PruneComposeServiceBackupRuns(_ context.Context, app, service string, _ int, _ time.Time) ([]store.PrunedBackup, error) {
	f.pruned = append(f.pruned, app+"/"+service)
	return nil, nil
}

type fakeComposeScheduledRunner struct{ calls []string }

func (f *fakeComposeScheduledRunner) RunComposeServiceBackup(_ context.Context, _, app, service, engine, targetID string) error {
	f.calls = append(f.calls, app+"|"+service+"|"+engine+"|"+targetID)
	return nil
}

func TestScheduler_Tick_ComposeServices(t *testing.T) {
	row := store.ComposeServiceBackup{App: "shop", Service: "db", Engine: store.EnginePostgres, BackupTargetID: "bkt_a", BackupSchedule: "0 3 * * *", BackupRetain: 2, Enabled: true}
	st := &fakeComposeScheduleStore{fakeScheduleStore: &fakeScheduleStore{}, rows: []store.ComposeServiceBackup{row}}
	run := &fakeComposeScheduledRunner{}
	s := NewScheduler(st, &fakeScheduledRunner{}, nil, nil)
	s.ComposeServices = run

	baseTick(t, s, 1)
	if len(run.calls) != 0 {
		t.Fatalf("armed tick ran: %v", run.calls)
	}
	baseTick(t, s, 3)
	if len(run.calls) != 1 || run.calls[0] != "shop|db|postgres|bkt_a" {
		t.Fatalf("calls = %v", run.calls)
	}
	if len(st.pruned) != 1 || st.pruned[0] != "shop/db" {
		t.Errorf("retention prune = %v", st.pruned)
	}
	baseTick(t, s, 3)
	if len(run.calls) != 1 {
		t.Fatalf("same slot ran again: %v", run.calls)
	}
}

func TestScheduler_Tick_ComposeServicesDisabledWithoutRunner(t *testing.T) {
	st := &fakeComposeScheduleStore{fakeScheduleStore: &fakeScheduleStore{}, rows: []store.ComposeServiceBackup{{App: "a", Service: "db", BackupSchedule: "0 3 * * *", BackupTargetID: "t"}}}
	s := NewScheduler(st, &fakeScheduledRunner{}, nil, nil)
	baseTick(t, s, 1)
	baseTick(t, s, 3)
}
