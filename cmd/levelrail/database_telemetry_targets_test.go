package main

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeInspectRuntime is a minimal docker.Runtime fake for
// databaseTelemetryTargets/databaseLogTargets: only InspectByName is
// exercised by either function, so every other method panics if called,
// making an unexpected dependency on them a loud test failure rather
// than a silent nil-return.
type fakeInspectRuntime struct {
	byName map[string]*docker.ContainerState
}

func (f *fakeInspectRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	return f.byName[name], nil
}

func (f *fakeInspectRuntime) Create(context.Context, docker.ContainerSpec) (string, error) {
	panic("not used by databaseTelemetryTargets/databaseLogTargets")
}
func (f *fakeInspectRuntime) Start(context.Context, string) error {
	panic("not used by databaseTelemetryTargets/databaseLogTargets")
}
func (f *fakeInspectRuntime) Events(context.Context) (<-chan docker.Event, <-chan error) {
	panic("not used by databaseTelemetryTargets/databaseLogTargets")
}
func (f *fakeInspectRuntime) ListImages(context.Context, string) ([]docker.ImageInfo, error) {
	panic("not used by databaseTelemetryTargets/databaseLogTargets")
}
func (f *fakeInspectRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	panic("not used by databaseTelemetryTargets/databaseLogTargets")
}
func (f *fakeInspectRuntime) Stop(context.Context, string, time.Duration) error {
	panic("not used by databaseTelemetryTargets/databaseLogTargets")
}
func (f *fakeInspectRuntime) Remove(context.Context, string, bool) error {
	panic("not used by databaseTelemetryTargets/databaseLogTargets")
}
func (f *fakeInspectRuntime) UpdateResources(context.Context, string, docker.Resources) error {
	panic("not used by databaseTelemetryTargets/databaseLogTargets")
}
func (f *fakeInspectRuntime) EnsureVolume(context.Context, string) error {
	panic("not used by databaseTelemetryTargets/databaseLogTargets")
}
func (f *fakeInspectRuntime) EnsureNetwork(context.Context, string) (string, error) {
	panic("not used by databaseTelemetryTargets/databaseLogTargets")
}
func (f *fakeInspectRuntime) RemoveNetwork(context.Context, string) error {
	panic("not used by databaseTelemetryTargets/databaseLogTargets")
}
func (f *fakeInspectRuntime) ListNetworksByPrefix(context.Context, string) ([]docker.NetworkInfo, error) {
	panic("not used by databaseTelemetryTargets/databaseLogTargets")
}
func (f *fakeInspectRuntime) Exec(context.Context, string, []string) (io.ReadCloser, error) {
	panic("not used by databaseTelemetryTargets/databaseLogTargets")
}
func (f *fakeInspectRuntime) ExecWithInput(context.Context, string, []string, io.Reader) (io.ReadCloser, error) {
	panic("not used by databaseTelemetryTargets/databaseLogTargets")
}

func TestDatabaseTelemetryTargets_OnlyRunningContainersIncluded(t *testing.T) {
	db := openCredentialsTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed database main: %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "stopped", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database stopped: %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "never-started", Engine: store.EngineMySQL, Version: "8"}); err != nil {
		t.Fatalf("seed database never-started: %v", err)
	}

	runtime := &fakeInspectRuntime{byName: map[string]*docker.ContainerState{
		"db-main":    {ID: "container-main", Name: "db-main", Running: true},
		"db-stopped": {ID: "container-stopped", Name: "db-stopped", Running: false},
		// "db-never-started" deliberately absent: InspectByName's
		// documented (nil, nil) "no such container" contract.
	}}

	targets, err := databaseTelemetryTargets(ctx, db, runtime)
	if err != nil {
		t.Fatalf("databaseTelemetryTargets() error = %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %+v, want exactly 1 (only the running container)", targets)
	}
	if targets[0].ResourceID != "database:main" || targets[0].ContainerID != "container-main" {
		t.Errorf("targets[0] = %+v, want ResourceID=database:main ContainerID=container-main", targets[0])
	}
}

func TestDatabaseLogTargets_OnlyRunningContainersIncluded(t *testing.T) {
	db := openCredentialsTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "cache", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database cache: %v", err)
	}

	runtime := &fakeInspectRuntime{byName: map[string]*docker.ContainerState{
		"db-cache": {ID: "container-cache", Name: "db-cache", Running: true},
	}}

	targets, err := databaseLogTargets(ctx, db, runtime)
	if err != nil {
		t.Fatalf("databaseLogTargets() error = %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %+v, want exactly 1", targets)
	}
	if targets[0].ResourceID != "database:cache" || targets[0].ContainerID != "container-cache" {
		t.Errorf("targets[0] = %+v, want ResourceID=database:cache ContainerID=container-cache", targets[0])
	}
}
