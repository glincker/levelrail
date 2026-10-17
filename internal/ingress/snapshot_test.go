package ingress

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testKey(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func newTestSnapshot(t *testing.T) *SnapshotStore {
	t.Helper()
	s, err := NewSnapshotStore(filepath.Join(t.TempDir(), SnapshotFilename), testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSnapshotStoreRoundTrip(t *testing.T) {
	s := newTestSnapshot(t)
	want := SnapshotData{SavedAt: time.Unix(1700000000, 0).UTC(), Fingerprint: "fd:3:4", Ports: []int{80, 443}, Config: []byte(`{"apps":{}}`)}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("fd:3:4")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Config) != string(want.Config) || len(got.Ports) != 2 || !got.SavedAt.Equal(want.SavedAt) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if info, err := os.Stat(s.Path()); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot mode = %v, err %v, want 0600", info.Mode().Perm(), err)
	}
}

func TestSnapshotStoreLoadFailures(t *testing.T) {
	valid := SnapshotData{Fingerprint: "bound", Config: []byte(`{}`)}
	tests := []struct {
		name   string
		mutate func(t *testing.T, s *SnapshotStore)
		fp     string
		want   error
	}{
		{name: "missing file", mutate: func(*testing.T, *SnapshotStore) {}, fp: "bound", want: ErrNoSnapshot},
		{name: "truncated mid-write", fp: "bound", want: ErrSnapshotCorrupt, mutate: func(t *testing.T, s *SnapshotStore) {
			mustSave(t, s, valid)
			raw, _ := os.ReadFile(s.Path())
			mustWrite(t, s.Path(), raw[:len(raw)/2])
		}},
		{name: "empty file", fp: "bound", want: ErrSnapshotCorrupt, mutate: func(t *testing.T, s *SnapshotStore) { mustWrite(t, s.Path(), nil) }},
		{name: "garbage", fp: "bound", want: ErrSnapshotCorrupt, mutate: func(t *testing.T, s *SnapshotStore) {
			mustWrite(t, s.Path(), []byte("not a snapshot at all, definitely"))
		}},
		{name: "flipped byte", fp: "bound", want: ErrSnapshotCorrupt, mutate: func(t *testing.T, s *SnapshotStore) {
			mustSave(t, s, valid)
			raw, _ := os.ReadFile(s.Path())
			raw[len(raw)-3] ^= 0xff
			mustWrite(t, s.Path(), raw)
		}},
		{name: "sealed with another key", fp: "bound", want: ErrSnapshotCorrupt, mutate: func(t *testing.T, s *SnapshotStore) {
			other, _ := NewSnapshotStore(s.Path(), testKey(9))
			mustSave(t, other, valid)
		}},
		{name: "different listeners", fp: "fd:3:4", want: ErrSnapshotStale, mutate: func(t *testing.T, s *SnapshotStore) { mustSave(t, s, valid) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestSnapshot(t)
			tc.mutate(t, s)
			if _, err := s.Load(tc.fp); !errors.Is(err, tc.want) {
				t.Fatalf("Load error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSnapshotSaveKeepsPreviousOnFailure(t *testing.T) {
	s := newTestSnapshot(t)
	mustSave(t, s, SnapshotData{Fingerprint: "bound", Config: []byte(`{"v":1}`)})
	bad := &SnapshotStore{path: filepath.Join(filepath.Dir(s.Path()), "missing-dir", SnapshotFilename), aead: s.aead}
	if err := bad.Save(SnapshotData{Fingerprint: "bound", Config: []byte(`{"v":2}`)}); err == nil {
		t.Fatal("Save into a missing directory succeeded")
	}
	got, err := s.Load("bound")
	if err != nil || string(got.Config) != `{"v":1}` {
		t.Fatalf("previous snapshot lost: %v %s", err, got.Config)
	}
}

func TestSnapshotFingerprint(t *testing.T) {
	tests := []struct {
		name string
		in   *InheritedSockets
		want string
	}{
		{"nil", nil, "bound"},
		{"inactive", &InheritedSockets{}, "bound"},
		{"active", &InheritedSockets{HTTPS: 3, HTTP: 4}, "fd:3:4"},
	}
	for _, tc := range tests {
		if got := tc.in.Fingerprint(); got != tc.want {
			t.Fatalf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDriverRestoreSnapshotBeforeFirstReconcile(t *testing.T) {
	orig := loadConfig
	t.Cleanup(func() { loadConfig = orig })
	var loaded [][]byte
	var failLoad error
	loadConfig = func(b []byte, _ bool) error {
		loaded = append(loaded, append([]byte(nil), b...))
		return failLoad
	}

	store := newTestSnapshot(t)
	first := New(nil)
	first.SetSnapshot(store, "bound")
	cfg := &Config{Admin: &AdminConfig{Listen: "127.0.0.1:2999"}, Apps: Apps{}}
	if err := first.Apply(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if !first.SnapshotStatus().SavedAt.After(time.Time{}) {
		t.Fatal("Apply did not record a snapshot")
	}

	loaded = nil
	second := New(nil)
	second.SetSnapshot(store, "bound")
	ok, err := second.RestoreSnapshot(context.Background())
	if err != nil || !ok {
		t.Fatalf("RestoreSnapshot = %v, %v", ok, err)
	}
	if len(loaded) != 1 {
		t.Fatalf("restore loaded %d configs, want 1 before any reconcile", len(loaded))
	}
	if !second.SnapshotStatus().Restored {
		t.Fatal("status does not report the restore")
	}
	if err := second.Apply(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("identical first reconcile reloaded Caddy (%d loads)", len(loaded))
	}

	failLoad = errors.New("unknown module")
	third := New(nil)
	third.SetSnapshot(store, "bound")
	if ok, err := third.RestoreSnapshot(context.Background()); ok || err == nil {
		t.Fatalf("restore with a config Caddy rejects = %v, %v", ok, err)
	}
	if third.SnapshotStatus().RestoreError == "" || third.SnapshotStatus().Restored {
		t.Fatalf("failed restore not reported: %+v", third.SnapshotStatus())
	}
}

func TestDriverRestoreSnapshotNoopCases(t *testing.T) {
	d := New(nil)
	if ok, err := d.RestoreSnapshot(context.Background()); ok || err != nil {
		t.Fatalf("no store configured = %v, %v", ok, err)
	}
	d.SetSnapshot(newTestSnapshot(t), "bound")
	if ok, err := d.RestoreSnapshot(context.Background()); ok || err != nil {
		t.Fatalf("no snapshot file yet = %v, %v", ok, err)
	}
	if d.SnapshotStatus().RestoreError != "" {
		t.Fatal("a first boot must not report a restore error")
	}
}

func mustSave(t *testing.T, s *SnapshotStore, d SnapshotData) {
	t.Helper()
	if err := s.Save(d); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0o600); err != nil { //nolint:gosec // test path from t.TempDir
		t.Fatal(err)
	}
}
