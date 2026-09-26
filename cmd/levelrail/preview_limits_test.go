package main

import (
	"io"
	"log/slog"
	"testing"
)

func TestPreviewLimits_EnvParsing(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tests := []struct {
		name          string
		perApp, total string
		wantPer       int
		wantTotal     int
	}{
		{name: "defaults", wantPer: defaultPreviewMaxPerApp, wantTotal: defaultPreviewMaxTotal},
		{name: "explicit", perApp: "3", total: "7", wantPer: 3, wantTotal: 7},
		{name: "zero disables a cap", perApp: "0", total: "0", wantPer: 0, wantTotal: 0},
		{name: "invalid falls back", perApp: "abc", total: "-4", wantPer: defaultPreviewMaxPerApp, wantTotal: defaultPreviewMaxTotal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_PREVIEW_ENV_MAX_PER_APP", tt.perApp)
			t.Setenv("APP_PREVIEW_ENV_MAX_TOTAL", tt.total)
			got := previewLimits(logger)
			if got.MaxPerApp != tt.wantPer || got.MaxTotal != tt.wantTotal {
				t.Errorf("previewLimits() = %+v, want per-app %d total %d", got, tt.wantPer, tt.wantTotal)
			}
		})
	}
}
