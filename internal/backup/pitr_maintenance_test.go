package backup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeMaintenanceStore struct {
	rows    []store.BaseBackupHistory
	deleted []string
}

func (f *fakeMaintenanceStore) ListBaseBackupHistory(context.Context, string) ([]store.BaseBackupHistory, error) {
	return f.rows, nil
}

func (f *fakeMaintenanceStore) DeleteBaseBackupHistory(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func newestFirst() []store.BaseBackupHistory {
	return []store.BaseBackupHistory{
		{ID: "bb5", Status: store.BackupStatusSucceeded, LSN: "0/5000100", ObjectKey: "k5", TargetID: "t"},
		{ID: "bb4", Status: store.BackupStatusFailed, ObjectKey: "k4", TargetID: "t"},
		{ID: "bb3", Status: store.BackupStatusSucceeded, LSN: "0/3000100", ObjectKey: "k3", TargetID: "t"},
		{ID: "bb2", Status: store.BackupStatusSucceeded, LSN: "0/2000100", ObjectKey: "k2", TargetID: "t"},
		{ID: "bb1", Status: store.BackupStatusSucceeded, LSN: "0/1000100", ObjectKey: "k1", TargetID: "t"},
	}
}

func TestPITRMaintainer_Prune(t *testing.T) {
	tests := []struct {
		name        string
		keep        int
		rows        []store.BaseBackupHistory
		wantDeleted []string
		wantLSN     string
	}{
		{"keeps newest two, prunes WAL to the oldest kept", 2, newestFirst(), []string{"bb2", "bb1"}, "0/3000100"},
		{"nothing beyond keep", 5, newestFirst(), nil, "0/1000100"},
		{"no succeeded backups does nothing", 2, []store.BaseBackupHistory{{ID: "x", Status: store.BackupStatusFailed}}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeMaintenanceStore{rows: tt.rows}
			del := &fakeDeleter{}
			rt := &fakeExecRuntime{}
			m := &PITRMaintainer{
				Store:   st,
				Resolve: func(context.Context, string) (Destination, error) { return Destination{Bucket: "b"}, nil },
				Deleter: del,
				Runtime: rt,
				Keep:    tt.keep,
			}
			if err := m.Prune(context.Background(), "main", "db-main"); err != nil {
				t.Fatalf("Prune: %v", err)
			}
			if strings.Join(st.deleted, ",") != strings.Join(tt.wantDeleted, ",") {
				t.Errorf("deleted rows = %v, want %v", st.deleted, tt.wantDeleted)
			}
			if len(del.calls) != len(tt.wantDeleted) {
				t.Errorf("deleted %d objects, want %d", len(del.calls), len(tt.wantDeleted))
			}
			if tt.wantLSN == "" {
				if rt.execCalls != 0 {
					t.Errorf("exec called %d times, want 0", rt.execCalls)
				}
				return
			}
			if rt.execCalls != 1 || !strings.Contains(strings.Join(rt.gotCmd, " "), "pg_walfile_name('"+tt.wantLSN+"')") {
				t.Errorf("wal prune cmd = %v, want it to cut at %s", rt.gotCmd, tt.wantLSN)
			}
		})
	}
}

func TestPITRMaintainer_ObjectDeleteFailureKeepsRowAndWAL(t *testing.T) {
	st := &fakeMaintenanceStore{rows: newestFirst()}
	rt := &fakeExecRuntime{}
	m := &PITRMaintainer{
		Store:   st,
		Resolve: func(context.Context, string) (Destination, error) { return Destination{}, nil },
		Deleter: &fakeDeleter{err: errors.New("bucket down")},
		Runtime: rt,
		Keep:    2,
	}
	if err := m.Prune(context.Background(), "main", "db-main"); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(st.deleted) != 0 {
		t.Errorf("rows deleted despite object delete failure: %v", st.deleted)
	}
	if rt.execCalls != 0 {
		t.Error("WAL must not be pruned while an older base backup still exists")
	}
}

func TestPruneWALArchive_RejectsMalformedLSN(t *testing.T) {
	rt := &fakeExecRuntime{}
	for _, lsn := range []string{"", "0/5'; rm -rf /", "garbage", "0/"} {
		if err := PruneWALArchive(context.Background(), rt, "db-main", lsn); err == nil {
			t.Errorf("PruneWALArchive(%q) accepted a malformed LSN", lsn)
		}
	}
	if rt.execCalls != 0 {
		t.Errorf("exec called for malformed LSN")
	}
}

type fakeMaintainer struct{ calls int }

func (f *fakeMaintainer) Prune(context.Context, string, string) error { f.calls++; return nil }

func TestBaseBackupRunner_RunsMaintainerOnlyAfterSuccess(t *testing.T) {
	st := &fakeBaseBackupHistoryStore{targets: map[string]store.BackupTarget{"tgt_1": {ID: "tgt_1", Provider: store.BackupProviderCustom, Bucket: "b", Endpoint: "https://s3.example"}}}
	m := &fakeMaintainer{}
	ok := newTestBaseBackupRunner(st, &fakeBaseBackuper{content: "x"}, &fakeUploader{})
	ok.Maintainer = m
	if err := ok.RunBaseBackup(context.Background(), "bbh_1", "mydb", "db-mydb", "tgt_1"); err != nil {
		t.Fatalf("RunBaseBackup: %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("maintainer calls after success = %d, want 1", m.calls)
	}

	bad := newTestBaseBackupRunner(&fakeBaseBackupHistoryStore{targets: st.targets}, &fakeBaseBackuper{err: errors.New("boom")}, &fakeUploader{})
	bad.Maintainer = m
	_ = bad.RunBaseBackup(context.Background(), "bbh_2", "mydb", "db-mydb", "tgt_1")
	if m.calls != 1 {
		t.Fatalf("maintainer ran after a failed base backup (calls = %d)", m.calls)
	}
}
