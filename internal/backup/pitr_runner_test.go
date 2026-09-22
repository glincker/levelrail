package backup

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakePITRHistoryStore backs PITRHistoryStore for pitr_runner_test.go.
// suspendCalls records every UpdateDatabaseSuspended call in order, so
// tests can assert the destructive phase actually bracketed itself with
// suspend-true then suspend-false.
type fakePITRHistoryStore struct {
	baseBackups map[string]store.BaseBackupHistory
	targets     map[string]store.BackupTarget
	oldest      store.BaseBackupHistory
	hasOldest   bool

	started  []store.PITRRestoreHistory
	finished []struct {
		id, status, errMsg, finishedAt string
	}
	suspendCalls []bool

	getBaseBackupErr error
	suspendErr       error
}

func (f *fakePITRHistoryStore) GetOldestSucceededBaseBackup(_ context.Context, _ string) (store.BaseBackupHistory, bool, error) {
	return f.oldest, f.hasOldest, nil
}

func (f *fakePITRHistoryStore) GetBackupTarget(_ context.Context, id string) (store.BackupTarget, error) {
	t, ok := f.targets[id]
	if !ok {
		return store.BackupTarget{}, store.ErrBackupTargetNotFound
	}
	return t, nil
}

func (f *fakePITRHistoryStore) GetBaseBackupHistory(_ context.Context, id string) (store.BaseBackupHistory, error) {
	if f.getBaseBackupErr != nil {
		return store.BaseBackupHistory{}, f.getBaseBackupErr
	}
	b, ok := f.baseBackups[id]
	if !ok {
		return store.BaseBackupHistory{}, store.ErrBaseBackupHistoryNotFound
	}
	return b, nil
}

func (f *fakePITRHistoryStore) StartPITRRestoreHistory(_ context.Context, h store.PITRRestoreHistory) error {
	f.started = append(f.started, h)
	return nil
}

func (f *fakePITRHistoryStore) FinishPITRRestoreHistory(_ context.Context, id, status, errMsg, finishedAt string) error {
	f.finished = append(f.finished, struct {
		id, status, errMsg, finishedAt string
	}{id, status, errMsg, finishedAt})
	return nil
}

func (f *fakePITRHistoryStore) UpdateDatabaseSuspended(_ context.Context, _ string, suspended bool) error {
	if f.suspendErr != nil {
		return f.suspendErr
	}
	f.suspendCalls = append(f.suspendCalls, suspended)
	return nil
}

// fakePITRRestorer records the arguments Restore was called with rather
// than doing anything real to a volume.
type fakePITRRestorer struct {
	gotVolume string
	gotBody   string
	gotTime   time.Time
	err       error
}

func (f *fakePITRRestorer) Restore(_ context.Context, dataVolumeName string, baseBackup io.Reader, targetTime time.Time) error {
	f.gotVolume = dataVolumeName
	b, _ := io.ReadAll(baseBackup)
	f.gotBody = string(b)
	f.gotTime = targetTime
	return f.err
}

// fakePITRRuntime is a minimal docker.Runtime standing in for the
// container-lifecycle side of a restore: InspectByName reports "gone"
// until goneAfter calls, then "running" from then on; Exec answers the
// promotion probe with recoveringFor calls of "t" before switching to
// "f", the same "eventually converges" shape a real container's own
// recovery-then-promote sequence has.
type fakePITRRuntime struct {
	inspectCalls int
	goneAfter    int // InspectByName returns nil (gone) for calls <= goneAfter
	inspectErr   error

	execCalls     int
	recoveringFor int // Exec (the promotion probe) answers "t" for calls <= recoveringFor
}

func (f *fakePITRRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	f.inspectCalls++
	if f.inspectErr != nil {
		return nil, f.inspectErr
	}
	if f.inspectCalls <= f.goneAfter {
		return nil, nil
	}
	return &docker.ContainerState{ID: "c1", Name: name, Running: true}, nil
}

func (f *fakePITRRuntime) Exec(_ context.Context, _ string, _ []string) (io.ReadCloser, error) {
	f.execCalls++
	if f.execCalls <= f.recoveringFor {
		return io.NopCloser(strings.NewReader("t")), nil
	}
	return io.NopCloser(strings.NewReader("f")), nil
}

func (f *fakePITRRuntime) ExecWithInput(context.Context, string, []string, io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (f *fakePITRRuntime) Create(context.Context, docker.ContainerSpec) (string, error) {
	return "", nil
}
func (f *fakePITRRuntime) Start(context.Context, string) error { return nil }
func (f *fakePITRRuntime) Events(context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}
func (f *fakePITRRuntime) ListImages(context.Context, string) ([]docker.ImageInfo, error) {
	return nil, nil
}
func (f *fakePITRRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return nil, nil
}
func (f *fakePITRRuntime) Stop(context.Context, string, time.Duration) error { return nil }
func (f *fakePITRRuntime) Remove(context.Context, string, bool) error        { return nil }
func (f *fakePITRRuntime) UpdateResources(context.Context, string, docker.Resources) error {
	return nil
}
func (f *fakePITRRuntime) EnsureVolume(context.Context, string) error { return nil }
func (f *fakePITRRuntime) EnsureNetwork(context.Context, string) (string, error) {
	return "", nil
}
func (f *fakePITRRuntime) RemoveNetwork(context.Context, string) error { return nil }
func (f *fakePITRRuntime) ListNetworksByPrefix(context.Context, string) ([]docker.NetworkInfo, error) {
	return nil, nil
}

func newTestPITRRunner(st *fakePITRHistoryStore, restorer *fakePITRRestorer, rt *fakePITRRuntime, downloaderErr error) *PITRRunner {
	return &PITRRunner{
		Store:           st,
		Secrets:         stubSecrets{},
		Downloader:      &fakeDownloader{content: "base-tar-bytes", err: downloaderErr},
		Restorer:        restorer,
		Runtime:         rt,
		Now:             func() time.Time { return time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC) },
		PollInterval:    time.Millisecond,
		TeardownTimeout: 200 * time.Millisecond,
		StartupTimeout:  200 * time.Millisecond,
		PromoteTimeout:  200 * time.Millisecond,
	}
}

func TestPITRRunner_RunPITRRestore_Success(t *testing.T) {
	st := &fakePITRHistoryStore{
		baseBackups: map[string]store.BaseBackupHistory{
			"bbh_1": {ID: "bbh_1", TargetID: "tgt_1", ObjectKey: "mydb/base-1.tar", Status: store.BackupStatusSucceeded},
		},
		targets: map[string]store.BackupTarget{"tgt_1": {ID: "tgt_1", Provider: store.BackupProviderCustom}},
	}
	restorer := &fakePITRRestorer{}
	rt := &fakePITRRuntime{goneAfter: 1, recoveringFor: 2}
	r := newTestPITRRunner(st, restorer, rt, nil)

	target := time.Date(2026, 8, 14, 6, 0, 0, 0, time.UTC)
	err := r.RunPITRRestore(context.Background(), "pitr_1", "mydb", "db-mydb", "db-mydb-data", "bbh_1", target)
	if err != nil {
		t.Fatalf("RunPITRRestore() error = %v", err)
	}

	if restorer.gotVolume != "db-mydb-data" || restorer.gotBody != "base-tar-bytes" || !restorer.gotTime.Equal(target) {
		t.Errorf("Restore called with (%q, %q, %v), want (db-mydb-data, base-tar-bytes, %v)", restorer.gotVolume, restorer.gotBody, restorer.gotTime, target)
	}
	if len(st.suspendCalls) < 2 || st.suspendCalls[0] != true || st.suspendCalls[len(st.suspendCalls)-1] != false {
		t.Fatalf("suspendCalls = %v, want starting true, ending false", st.suspendCalls)
	}
	if len(st.finished) != 1 || st.finished[0].status != store.BackupStatusSucceeded {
		t.Fatalf("finished = %+v, want one succeeded row", st.finished)
	}
	if len(st.started) != 1 || st.started[0].BaseBackupHistoryID != "bbh_1" {
		t.Fatalf("started = %+v", st.started)
	}
}

func TestPITRRunner_RunPITRRestore_BaseBackupNotSucceeded_NeverSuspends(t *testing.T) {
	st := &fakePITRHistoryStore{
		baseBackups: map[string]store.BaseBackupHistory{
			"bbh_1": {ID: "bbh_1", TargetID: "tgt_1", Status: store.BackupStatusFailed},
		},
		targets: map[string]store.BackupTarget{"tgt_1": {ID: "tgt_1"}},
	}
	restorer := &fakePITRRestorer{}
	rt := &fakePITRRuntime{}
	r := newTestPITRRunner(st, restorer, rt, nil)

	err := r.RunPITRRestore(context.Background(), "pitr_1", "mydb", "db-mydb", "db-mydb-data", "bbh_1", time.Now())
	if err == nil {
		t.Fatal("RunPITRRestore() error = nil, want a refusal to restore from a non-succeeded base backup")
	}
	if len(st.suspendCalls) != 0 {
		t.Errorf("suspendCalls = %v, want none: a failed base backup must never touch the live container", st.suspendCalls)
	}
	if len(st.finished) != 1 || st.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finished = %+v, want one failed row", st.finished)
	}
}

func TestPITRRunner_RunPITRRestore_DownloadFails_NeverSuspends(t *testing.T) {
	st := &fakePITRHistoryStore{
		baseBackups: map[string]store.BaseBackupHistory{
			"bbh_1": {ID: "bbh_1", TargetID: "tgt_1", Status: store.BackupStatusSucceeded},
		},
		targets: map[string]store.BackupTarget{"tgt_1": {ID: "tgt_1"}},
	}
	restorer := &fakePITRRestorer{}
	rt := &fakePITRRuntime{}
	r := newTestPITRRunner(st, restorer, rt, errors.New("network unreachable"))

	err := r.RunPITRRestore(context.Background(), "pitr_1", "mydb", "db-mydb", "db-mydb-data", "bbh_1", time.Now())
	if err == nil {
		t.Fatal("RunPITRRestore() error = nil, want the download failure")
	}
	if len(st.suspendCalls) != 0 {
		t.Errorf("suspendCalls = %v, want none: a download failure never touches the live container", st.suspendCalls)
	}
}

func TestPITRRunner_RunPITRRestore_RestoreFails_StillUnsuspends(t *testing.T) {
	st := &fakePITRHistoryStore{
		baseBackups: map[string]store.BaseBackupHistory{
			"bbh_1": {ID: "bbh_1", TargetID: "tgt_1", Status: store.BackupStatusSucceeded},
		},
		targets: map[string]store.BackupTarget{"tgt_1": {ID: "tgt_1"}},
	}
	restorer := &fakePITRRestorer{err: errors.New("volume extract failed: disk full")}
	rt := &fakePITRRuntime{goneAfter: 0}
	r := newTestPITRRunner(st, restorer, rt, nil)

	err := r.RunPITRRestore(context.Background(), "pitr_1", "mydb", "db-mydb", "db-mydb-data", "bbh_1", time.Now())
	if err == nil {
		t.Fatal("RunPITRRestore() error = nil, want the restore failure")
	}
	if len(st.suspendCalls) != 2 || st.suspendCalls[0] != true || st.suspendCalls[1] != false {
		t.Fatalf("suspendCalls = %v, want [true, false]: a failed restore must still leave the database unsuspended", st.suspendCalls)
	}
	if len(st.finished) != 1 || st.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finished = %+v, want one failed row", st.finished)
	}
}

func TestPITRRunner_RunPITRRestore_NeverPromotes_TimesOutAsFailure(t *testing.T) {
	st := &fakePITRHistoryStore{
		baseBackups: map[string]store.BaseBackupHistory{
			"bbh_1": {ID: "bbh_1", TargetID: "tgt_1", Status: store.BackupStatusSucceeded},
		},
		targets: map[string]store.BackupTarget{"tgt_1": {ID: "tgt_1"}},
	}
	restorer := &fakePITRRestorer{}
	// recoveringFor is huge: the promotion probe always answers "t"
	// (still recovering), simulating a target timestamp that can never be
	// reached because the archived WAL ran out first.
	rt := &fakePITRRuntime{goneAfter: 1, recoveringFor: 1_000_000}
	r := newTestPITRRunner(st, restorer, rt, nil)

	err := r.RunPITRRestore(context.Background(), "pitr_1", "mydb", "db-mydb", "db-mydb-data", "bbh_1", time.Now())
	if err == nil {
		t.Fatal("RunPITRRestore() error = nil, want a timeout: recovery that never promotes must be reported as a failure, never silently treated as success")
	}
	if len(st.finished) != 1 || st.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finished = %+v, want one failed row", st.finished)
	}
}
