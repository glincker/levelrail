package database

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
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
