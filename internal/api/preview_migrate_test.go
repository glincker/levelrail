package api

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPreviewImageTagAndMigrateValidation(t *testing.T) {
	if got := previewImageTag("ghcr.io/org/app:pr-{number}-{short_sha}-{branch}", 42, "abcdef1234567", "feat/x y"); got != "ghcr.io/org/app:pr-42-abcdef1-feat-x-y" {
		t.Fatalf("tag = %q", got)
	}
	if previewImageTag("", 1, "a", "b") != "" {
		t.Fatal("empty template must stay empty")
	}
	s := store.DefaultPreviewAppSettings("a")
	bad := "ghcr.io/org/app:latest"
	if msg := applyPreviewMigrateFields(&s, setPreviewPolicyRequest{ImageTagTemplate: &bad}); !strings.Contains(msg, "{number}") {
		t.Fatalf("template without placeholder accepted: %q", msg)
	}
	cmd, ok := " npm run migrate ", "ghcr.io/org/app:pr-{number}"
	timeout := 90
	if msg := applyPreviewMigrateFields(&s, setPreviewPolicyRequest{MigrateCommand: &cmd, ImageTagTemplate: &ok, MigrateTimeoutSeconds: &timeout}); msg != "" {
		t.Fatalf("valid fields rejected: %q", msg)
	}
	if s.MigrateCommand != "npm run migrate" || previewMigrateTimeout(s).Seconds() != 90 {
		t.Fatalf("settings = %+v", s)
	}
}

func TestRunPreviewMigrateDatabaseReadiness(t *testing.T) {
	rt := &Router{}
	s := store.DefaultPreviewAppSettings("a")
	if note, err := rt.runPreviewMigrate(context.Background(), "a-pr-1", s, true); note != "" || err != nil {
		t.Fatalf("no command must be a no-op, got %q %v", note, err)
	}
	s.MigrateCommand = "npm run migrate"
	for _, strategy := range []string{store.PreviewDatabaseStrategyShared} {
		s.DatabaseStrategy = strategy
		if _, err := rt.runPreviewMigrate(context.Background(), "a-pr-1", s, false); err == nil || !strings.Contains(err.Error(), "cannot be verified") {
			t.Fatalf("%s: err = %v", strategy, err)
		}
	}
	for _, strategy := range []string{store.PreviewDatabaseStrategySeed, store.PreviewDatabaseStrategyFresh} {
		s.DatabaseStrategy = strategy
		if err := previewMigrateDatabaseGuard(s, false); err != nil {
			t.Fatalf("%s must be allowed: %v", strategy, err)
		}
	}
	s.DatabaseStrategy = store.PreviewDatabaseStrategyNone
	if _, err := rt.runPreviewMigrate(context.Background(), "a-pr-1", s, true); err == nil {
		t.Fatal("app.yaml databases must be refused")
	}
	// A seed that cannot run must stop the migration, not leave an empty database.
	_, err := rt.seedPreviewDatabase(context.Background(), "main", store.PreviewEphemeralDatabase{DatabaseName: "a-pr-1-db"}, "postgres", true)
	if err == nil || !strings.Contains(err.Error(), "migration skipped") {
		t.Fatalf("seed without runner under migrate: %v", err)
	}
	if notes, err := rt.seedPreviewDatabase(context.Background(), "main", store.PreviewEphemeralDatabase{DatabaseName: "a-pr-1-db"}, "postgres", false); err != nil || len(notes) != 1 {
		t.Fatalf("seed without runner, no migrate: %v %v", notes, err)
	}
}

func TestWaitPreviewDatabaseReady(t *testing.T) {
	ctx := context.Background()
	up := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}, execReader: io.NopCloser(strings.NewReader(""))}
	if err := waitPreviewDatabaseReady(ctx, up, "db-x", "postgres", time.Second); err != nil || up.gotCmd[0] != "pg_isready" {
		t.Fatalf("ready database: err=%v cmd=%v", err, up.gotCmd)
	}
	down := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}, execErr: errors.New("refused")}
	if err := waitPreviewDatabaseReady(ctx, down, "db-x", "postgres", time.Millisecond); err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("unready database must time out: %v", err)
	}
	stopped := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1"}}
	if err := waitPreviewDatabaseReady(ctx, stopped, "db-x", "postgres", time.Millisecond); err == nil || stopped.execCalls != 0 {
		t.Fatalf("stopped container must not be probed: %v", err)
	}
	if err := waitPreviewDatabaseReady(ctx, up, "db-x", "redis", time.Second); err == nil || !strings.Contains(err.Error(), "cannot be verified") {
		t.Fatalf("unprobed engine must fail closed: %v", err)
	}
}
