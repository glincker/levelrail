package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/reconcile/application"
	"github.com/glincker/levelrail/internal/store"
)

// inertRuntime embeds a nil Runtime: controllers are only built here, never reconciled.
type inertRuntime struct{ docker.Runtime }

func spreadBootDeps(t *testing.T) dynamicSourceDeps {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return dynamicSourceDeps{db: db, runtime: inertRuntime{}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestSpreadBootSettingOffBuildsPlainController(t *testing.T) {
	deps := spreadBootDeps(t)
	svcs := []store.DesiredService{{Name: "web", Image: "img:1", Port: 80, Replicas: 3}}
	ctls := appControllersFor(context.Background(), deps, svcs)
	if _, ok := ctls[0].(*application.Controller); !ok {
		t.Fatalf("controller = %T, want the plain application controller with the setting off", ctls[0])
	}
}

func TestSpreadBootLeftoverSpreadGetsWrapper(t *testing.T) {
	deps := spreadBootDeps(t)
	svcs := []store.DesiredService{{Name: "web", Image: "img:1", Port: 80, Replicas: 2, ReplicaNodes: []string{"", ""}}}
	ctls := appControllersFor(context.Background(), deps, svcs)
	if _, ok := ctls[0].(*application.SpreadController); !ok {
		t.Fatalf("controller = %T, want the spread controller to unwind a leftover spread", ctls[0])
	}
	if ctls[0].Name() != application.ControllerName("web") {
		t.Fatalf("name = %q, want the plain application identity", ctls[0].Name())
	}
}
