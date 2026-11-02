package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestApplySavedAuthRateLimits(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	t.Setenv("APP_AUTH_ENGINE_RATE_LIMIT_PER_IP", "3")
	t.Setenv("APP_AUTH_ENGINE_RATE_LIMIT_PER_EMAIL", "200")

	var cfg authengine.Config
	applySavedAuthRateLimits(ctx, db.DB, &cfg)
	if cfg.RateLimitPerIP != runtimesettings.AuthRateLimitFloor || cfg.RateLimitPerEmail != 200 {
		t.Fatalf("env seeds = %d/%d, want floor/200", cfg.RateLimitPerIP, cfg.RateLimitPerEmail)
	}

	now := time.Now()
	if err := db.SetRuntimeSetting(ctx, runtimesettings.KeyAuthRateLimitPerIP, "75", "root", now); err != nil {
		t.Fatal(err)
	}
	if err := db.SetRuntimeSetting(ctx, runtimesettings.KeyAuthRateLimitPerEmail, "30", "root", now); err != nil {
		t.Fatal(err)
	}
	applySavedAuthRateLimits(ctx, db.DB, &cfg)
	if cfg.RateLimitPerIP != 75 || cfg.RateLimitPerEmail != 30 {
		t.Fatalf("saved = %d/%d, want 75/30 (saved beats env)", cfg.RateLimitPerIP, cfg.RateLimitPerEmail)
	}

	t.Setenv("APP_AUTH_ENGINE_RATE_LIMIT_PER_IP", "")
	t.Setenv("APP_AUTH_ENGINE_RATE_LIMIT_PER_EMAIL", "")
	if err := db.DeleteRuntimeSetting(ctx, runtimesettings.KeyAuthRateLimitPerIP); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteRuntimeSetting(ctx, runtimesettings.KeyAuthRateLimitPerEmail); err != nil {
		t.Fatal(err)
	}
	applySavedAuthRateLimits(ctx, db.DB, &cfg)
	if cfg.RateLimitPerIP != 60 || cfg.RateLimitPerEmail != 60 {
		t.Fatalf("defaults = %d/%d, want 60/60", cfg.RateLimitPerIP, cfg.RateLimitPerEmail)
	}
}
