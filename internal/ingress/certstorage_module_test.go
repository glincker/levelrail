package ingress

import (
	"testing"
)

func TestSqliteCertStorageModule_CertMagicStorage_NoActiveBackend(t *testing.T) {
	SetActiveCertStorage(nil)

	_, err := (sqliteCertStorageModule{}).CertMagicStorage()
	if err == nil {
		t.Fatal("CertMagicStorage() error = nil, want an error when no backend is registered")
	}
}

func TestSqliteCertStorageModule_CertMagicStorage_ReturnsActiveBackend(t *testing.T) {
	fc := newFakeCertStore()
	want := NewSQLiteStorage(fc, testLogger(t))
	SetActiveCertStorage(want)
	t.Cleanup(func() { SetActiveCertStorage(nil) })

	got, err := (sqliteCertStorageModule{}).CertMagicStorage()
	if err != nil {
		t.Fatalf("CertMagicStorage() error = %v", err)
	}
	if got != want {
		t.Errorf("CertMagicStorage() = %v, want the registered *SQLiteStorage instance %v", got, want)
	}
}

func TestSqliteCertStorageModule_CaddyModule(t *testing.T) {
	info := (sqliteCertStorageModule{}).CaddyModule()
	if info.ID != "caddy.storage.sqlite" {
		t.Errorf("CaddyModule().ID = %q, want caddy.storage.sqlite", info.ID)
	}
	if info.New() == nil {
		t.Error("CaddyModule().New() = nil")
	}
}

func TestNewSQLiteStorageRef(t *testing.T) {
	ref := NewSQLiteStorageRef()
	if ref.Module != "sqlite" {
		t.Errorf("NewSQLiteStorageRef().Module = %q, want sqlite", ref.Module)
	}
}
