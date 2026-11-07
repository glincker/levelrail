package api

import (
	"context"
	"strings"
	"testing"

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

func TestRunPreviewMigrateRefusesUnverifiableDatabase(t *testing.T) {
	rt := &Router{}
	s := store.DefaultPreviewAppSettings("a")
	if note, err := rt.runPreviewMigrate(context.Background(), "a-pr-1", s, true); note != "" || err != nil {
		t.Fatalf("no command must be a no-op, got %q %v", note, err)
	}
	s.MigrateCommand = "npm run migrate"
	for _, strategy := range []string{store.PreviewDatabaseStrategyShared, store.PreviewDatabaseStrategyFresh, store.PreviewDatabaseStrategySeed} {
		s.DatabaseStrategy = strategy
		if _, err := rt.runPreviewMigrate(context.Background(), "a-pr-1", s, false); err == nil || !strings.Contains(err.Error(), "cannot yet verify") {
			t.Fatalf("%s: err = %v", strategy, err)
		}
	}
	s.DatabaseStrategy = store.PreviewDatabaseStrategyNone
	if _, err := rt.runPreviewMigrate(context.Background(), "a-pr-1", s, true); err == nil {
		t.Fatal("app.yaml databases must be refused")
	}
}
