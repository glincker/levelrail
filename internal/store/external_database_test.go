package store

import (
	"errors"
	"testing"
	"time"
)

func TestExternalDatabaseLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	d := ExternalDatabase{Name: "main", Engine: "postgres", Host: "pg", Port: 5432, Username: "app", DatabaseName: "app", TLSMode: "prefer", Network: "coolify"}
	if err := db.CreateExternalDatabase(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateExternalDatabase(ctx, d); !errors.Is(err, ErrExternalDatabaseExists) {
		t.Fatalf("duplicate = %v", err)
	}
	got, err := db.GetExternalDatabase(ctx, "main")
	if err != nil || got.Host != "pg" || got.Network != "coolify" || got.ProjectID != "" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	d.Host, d.Port = "pg2", 5433
	if err := db.UpdateExternalDatabase(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := db.SetExternalDatabaseHealth(ctx, "main", ExternalHealthAuthFailed, "bad pw", 12, time.Now()); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListExternalDatabases(ctx)
	if err != nil || len(list) != 1 || list[0].Host != "pg2" || list[0].HealthStatus != ExternalHealthAuthFailed || list[0].HealthCheckedAt == "" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if err := db.DeleteExternalDatabase(ctx, "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetExternalDatabase(ctx, "main"); !errors.Is(err, ErrExternalDatabaseNotFound) {
		t.Fatalf("after delete = %v", err)
	}
	if err := db.DeleteExternalDatabase(ctx, "main"); !errors.Is(err, ErrExternalDatabaseNotFound) {
		t.Fatalf("second delete = %v", err)
	}
}
