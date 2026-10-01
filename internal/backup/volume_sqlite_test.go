package backup

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestContainerSqliteSnapshotter_Snapshot(t *testing.T) {
	rt := &fakeVolumeRuntime{execContent: "consistent-db-bytes"}
	s := &ContainerSqliteSnapshotter{Runtime: rt}

	rc, err := s.Snapshot(context.Background(), "app-web-data", "app/data.db")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "consistent-db-bytes" {
		t.Errorf("content = %q, want %q", got, "consistent-db-bytes")
	}

	if len(rt.gotExecCmd) < 1 || rt.gotExecCmd[0] != "sh" {
		t.Fatalf("exec cmd = %v, want a sh -c invocation", rt.gotExecCmd)
	}
	script := rt.gotExecCmd[len(rt.gotExecCmd)-1]
	for _, want := range []string{"sqlite3", "/vol/app/data.db", ".backup", sqliteSnapshotPath} {
		if !strings.Contains(script, want) {
			t.Errorf("exec script %q does not contain %q", script, want)
		}
	}

	if rt.removeCalls != 0 {
		t.Fatal("Remove called before Close, want 0 calls until the caller is done reading")
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if rt.removeCalls != 1 {
		t.Errorf("Remove calls after Close = %d, want 1 (helper container cleaned up)", rt.removeCalls)
	}
}

func TestContainerSqliteSnapshotter_Snapshot_InvalidPath(t *testing.T) {
	rt := &fakeVolumeRuntime{}
	s := &ContainerSqliteSnapshotter{Runtime: rt}

	if _, err := s.Snapshot(context.Background(), "app-web-data", "../escape.db"); err == nil {
		t.Fatal("Snapshot() error = nil, want a path validation error")
	}
	if rt.createCalls != 0 {
		t.Error("Create was called despite an invalid path, want validation to short-circuit before touching Docker")
	}
}

func TestContainerSqliteSnapshotter_Snapshot_ExecFailure_RemovesHelper(t *testing.T) {
	rt := &fakeVolumeRuntime{execErr: errors.New("exec failed")}
	s := &ContainerSqliteSnapshotter{Runtime: rt}

	_, err := s.Snapshot(context.Background(), "app-web-data", "data.db")
	if err == nil {
		t.Fatal("Snapshot() error = nil, want the Exec failure")
	}
	if rt.removeCalls != 1 {
		t.Errorf("Remove calls after an Exec failure = %d, want 1 (no leaked helper container)", rt.removeCalls)
	}
}

func TestContainerSqliteSnapshotter_Snapshot_CreateFailure(t *testing.T) {
	rt := &fakeVolumeRuntime{createErr: errors.New("create failed")}
	s := &ContainerSqliteSnapshotter{Runtime: rt}

	if _, err := s.Snapshot(context.Background(), "app-web-data", "data.db"); err == nil {
		t.Fatal("Snapshot() error = nil, want the Create failure")
	}
}

func TestValidateSqlitePath(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"app/data.db", false},
		{"data.db", false},
		{"", true},
		{"   ", true},
		{"/etc/passwd", true},
		{"../escape.db", true},
		{"app/../../escape.db", true},
	}
	for _, tt := range tests {
		err := ValidateSqlitePath(tt.path)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateSqlitePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
		}
	}
}
