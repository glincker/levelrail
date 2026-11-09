package main

import (
	"encoding/json"
	"log/slog"
	"path/filepath"
	"testing"

	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestUnmatchedSettingsReachHardeningWithoutRestart(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rs := runtimesettings.New(db, func(string) (string, bool) { return "", false })
	edge := ingressEdge{hardening: &ingressdriver.Hardening{}}

	if h := edge.effectiveHardening(ctx, nil, slog.Default(), rs); h.UnmatchedMode != runtimesettings.UnmatchedDiagnostic {
		t.Fatalf("default mode = %q, want diagnostic", h.UnmatchedMode)
	}
	if _, err := rs.Set(ctx, runtimesettings.KeyIngressUnmatchedMode, json.RawMessage(`"redirect"`), "root"); err != nil {
		t.Fatal(err)
	}
	if _, err := rs.Set(ctx, runtimesettings.KeyIngressUnmatchedRedirectURL, json.RawMessage(`"https://example.com/"`), "root"); err != nil {
		t.Fatal(err)
	}
	h := edge.effectiveHardening(ctx, nil, slog.Default(), rs)
	if h.UnmatchedMode != runtimesettings.UnmatchedRedirect || h.UnmatchedRedirectURL != "https://example.com/" {
		t.Fatalf("after save mode=%q url=%q, want redirect to https://example.com/", h.UnmatchedMode, h.UnmatchedRedirectURL)
	}
}
