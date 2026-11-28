package database

import (
	"context"
	"testing"

	"github.com/glincker/levelrail/internal/reconcile"
	"github.com/glincker/levelrail/internal/store"
)

func TestController_Reconcile_DocumentDB_CredentialsBlocked(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineDocumentDB, Version: "pg17-1.2.0"}
	c := New("main", &fakeStore{db: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "CredentialsNotConfigured" {
		t.Errorf("reason = %q, want CredentialsNotConfigured", cond.Reason)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

func TestController_Reconcile_DocumentDB_Spec(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineDocumentDB, Version: "pg17-1.2.0"}
	c := New("main", &fakeStore{db: desired}, rt, WithMongoDBCredentials(&MongoDBCredentials{Username: "main", Password: "s3cret"}))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("condition = %+v, want True", cond)
	}
	if rt.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", rt.createCalls)
	}
	if env := rt.lastCreateEnv; env["USERNAME"] != "main" || env["PASSWORD"] != "s3cret" {
		t.Errorf("env = %v, want USERNAME and PASSWORD set", env)
	}
	if env := rt.lastCreateEnv; env["ENABLE_TELEMETRY"] != "false" || env["DOCUMENTDB_USAGE_TELEMETRY"] != "false" {
		t.Errorf("env = %v, want both telemetry switches false", env)
	}
	if rt.lastCreateEnv["TLS_MODE"] != "" {
		t.Errorf("TLS_MODE = %q, want unset for a private database", rt.lastCreateEnv["TLS_MODE"])
	}
}

func TestImageRef_DocumentDB(t *testing.T) {
	want := "ghcr.io/documentdb/documentdb/documentdb-local:pg17-1.2.0"
	if got := ImageRef(store.EngineDocumentDB, "pg17-1.2.0"); got != want {
		t.Errorf("ImageRef = %q, want %q", got, want)
	}
	if store.EngineSupportsBackups(store.EngineDocumentDB) {
		t.Error("documentdb backups must be unsupported")
	}
}

func TestController_Reconcile_DocumentDB_PublicRequiresTLS(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineDocumentDB, Version: "pg17-1.2.0", PubliclyAccessible: true, PublicPort: 40010, PublicBindAddress: "0.0.0.0"}
	c := New("main", &fakeStore{db: desired}, rt, WithMongoDBCredentials(&MongoDBCredentials{Username: "main", Password: "s3cret"}))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got := rt.lastCreateEnv["TLS_MODE"]; got != "requireTLS" {
		t.Errorf("TLS_MODE = %q, want requireTLS", got)
	}
}

func TestDocumentDBVersionRules(t *testing.T) {
	if err := ValidateEngineVersion(store.EngineDocumentDB, "latest"); err == nil {
		t.Error("latest must be rejected")
	}
	if err := ValidateEngineVersion(store.EngineDocumentDB, "pg17-1.2.0"); err != nil {
		t.Errorf("pinned tag rejected: %v", err)
	}
	if err := CheckInPlaceVersionChange(store.EngineDocumentDB, "pg17-1.2.0", "pg17-1.3.0"); err != nil {
		t.Errorf("same pg major should be allowed: %v", err)
	}
	if err := CheckInPlaceVersionChange(store.EngineDocumentDB, "pg17-1.2.0", "pg18-1.2.0"); err == nil {
		t.Error("pg major change must be refused")
	}
	if got := ClientScheme(store.EngineDocumentDB); got != "mongodb" {
		t.Errorf("ClientScheme = %q", got)
	}
}
