package api

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cdn"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestTunablesEnvSeedsThenSavedWins(t *testing.T) {
	rt, _, _ := newMultiNodeRouter(t)
	ctx := context.Background()
	t.Setenv("APP_PROJECT_CLONE_MAX_APPS", "7")
	t.Setenv("APP_PROJECT_CLONE_PLACEHOLDER_DOMAIN", "seed.invalid")
	t.Setenv("APP_PREVIEW_MIGRATE_TIMEOUT", "30s")
	t.Setenv("APP_PREVIEW_MIGRATE_WAIT", "20s")
	t.Setenv("APP_DEPLOY_EXPLAIN_LOG_LINES", "3")
	t.Setenv("APP_UPDATE_CHANNELS_OFFERED", "stable,beta")

	if got := rt.cloneMaxApps(ctx); got != 7 {
		t.Fatalf("env seed clone max = %d", got)
	}
	if got := rt.clonePlaceholderSuffix(ctx); got != "seed.invalid" {
		t.Fatalf("env seed suffix = %q", got)
	}
	if got := rt.previewMigratePlatformTimeout(ctx); got != 30*time.Second {
		t.Fatalf("env seed migrate timeout = %s", got)
	}
	if got := rt.previewMigrateWait(ctx); got != 20*time.Second {
		t.Fatalf("env seed migrate wait = %s", got)
	}
	if got := rt.deployExplainLines(ctx); got != 3 {
		t.Fatalf("env seed explain lines = %d", got)
	}
	if got := rt.offeredUpdateChannels(ctx); !reflect.DeepEqual(got, []string{"stable", "beta"}) {
		t.Fatalf("env seed channels = %v", got)
	}

	set := func(key, raw string) {
		t.Helper()
		if _, err := rt.runtimeSettings.Set(ctx, key, json.RawMessage(raw), "test"); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
	set(runtimesettings.KeyProjectCloneMaxApps, `12`)
	set(runtimesettings.KeyProjectClonePlaceholderDomain, `"saved.invalid"`)
	set(runtimesettings.KeyPreviewMigrateTimeout, `"90s"`)
	set(runtimesettings.KeyPreviewMigrateWait, `"45s"`)
	set(runtimesettings.KeyDeployExplainLogLines, `5`)
	set(runtimesettings.KeyUpdateOfferedChannels, `["stable","edge"]`)

	if got := rt.cloneMaxApps(ctx); got != 12 {
		t.Fatalf("saved clone max = %d", got)
	}
	if got := rt.clonePlaceholderSuffix(ctx); got != "saved.invalid" {
		t.Fatalf("saved suffix = %q", got)
	}
	if got := rt.previewMigratePlatformTimeout(ctx); got != 90*time.Second {
		t.Fatalf("saved migrate timeout = %s", got)
	}
	if got := rt.previewMigrateWait(ctx); got != 45*time.Second {
		t.Fatalf("saved migrate wait = %s", got)
	}
	if got := rt.deployExplainLines(ctx); got != 5 {
		t.Fatalf("saved explain lines = %d", got)
	}
	if got := rt.offeredUpdateChannels(ctx); !reflect.DeepEqual(got, []string{"stable", "edge"}) {
		t.Fatalf("saved channels = %v", got)
	}
}

func TestTunablesRejectOutOfRange(t *testing.T) {
	rt, _, _ := newMultiNodeRouter(t)
	ctx := context.Background()
	bad := map[string]string{
		runtimesettings.KeyProjectCloneMaxApps:   `0`,
		runtimesettings.KeyPreviewMigrateTimeout: `"1s"`,
		runtimesettings.KeyCDNPurgeHistory:       `5000`,
		runtimesettings.KeyUpdateOfferedChannels: `["nightly"]`,
	}
	for key, raw := range bad {
		if _, err := rt.runtimeSettings.Set(ctx, key, json.RawMessage(raw), "test"); err == nil {
			t.Fatalf("%s accepted %s", key, raw)
		}
	}
}

func TestCDNPurgeHistoryCapFollowsSetting(t *testing.T) {
	rt, _, _ := newMultiNodeRouter(t)
	ctx := context.Background()
	if _, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyCDNPurgeHistory, json.RawMessage(`2`), "test"); err != nil {
		t.Fatal(err)
	}
	h := rt.cdnPurgeHistory(ctx)
	for i := 0; i < 4; i++ {
		h.Add("tunable-app", cdn.PurgeRecord{At: time.Now().UTC(), OK: true})
	}
	if got := len(h.List("tunable-app", 0)); got != 2 {
		t.Fatalf("kept %d records, want 2", got)
	}
}
