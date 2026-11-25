package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDBStatsSourceLocalRunningPostgresOnly(t *testing.T) {
	db := openCredentialsTestDB(t)
	ctx := context.Background()
	for _, d := range []store.DesiredDatabase{
		{Name: "main", Engine: store.EnginePostgres, Version: "16"},
		{Name: "stopped", Engine: store.EnginePostgres, Version: "16"},
		{Name: "remote", Engine: store.EnginePostgres, Version: "16"},
		{Name: "cache", Engine: store.EngineValkey, Version: "8"},
	} {
		if err := db.SaveDesiredDatabase(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.UpdateDatabaseNode(ctx, "remote", "node-2"); err != nil {
		t.Fatal(err)
	}
	rt := &fakeInspectRuntime{byName: map[string]*docker.ContainerState{
		"db-main":    {ID: "c-main", Running: true},
		"db-stopped": {ID: "c-stopped", Running: false},
		"db-remote":  {ID: "c-remote", Running: true},
		"db-cache":   {ID: "c-cache", Running: true},
	}}
	got, err := dbStatsSource{db: db, runtime: rt, logger: slog.Default()}.RunningDatabases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "main" || got[0].ContainerID != "c-main" {
		t.Fatalf("targets = %+v", got)
	}
}

func TestLookbackWithDBInterval(t *testing.T) {
	if got := lookbackWithDBInterval(time.Minute, 90*time.Second); got != 3*time.Minute {
		t.Fatalf("got %v", got)
	}
	if got := lookbackWithDBInterval(10*time.Minute, 30*time.Second); got != 10*time.Minute {
		t.Fatalf("got %v", got)
	}
}
