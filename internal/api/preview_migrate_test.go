package api

import (
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
