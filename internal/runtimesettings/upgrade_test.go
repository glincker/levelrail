package runtimesettings

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestSettingsUpgrade(t *testing.T) {
	ctx := context.Background()
	fs := newFakeStore()
	now := time.Now()
	for k, v := range map[string]string{
		KeyTemplatesImportMaxBytes: "5",       // below the floor
		"retired.old_name":         "7s",      // renamed below
		"removed.in_later_release": "keep-me", // unknown, kept
		KeyTemplatesImportTimeout:  "30s",     // valid, untouched
	} {
		fs.rows[k] = store.RuntimeSettingRow{Key: k, Value: v, UpdatedAt: now}
	}
	legacyKeys["retired.old_name"] = "templates.import_timeout_renamed"
	t.Cleanup(func() { delete(legacyKeys, "retired.old_name") })

	svc := New(fs, envOf(nil))
	if got := svc.Limit(ctx, KeyTemplatesImportMaxBytes); got != 256<<10 {
		t.Fatalf("out of range value should fall back to the default, got %d", got)
	}
	if got := svc.Duration(ctx, KeyTemplatesImportTimeout); got != 30*time.Second {
		t.Fatalf("valid value changed: %v", got)
	}
	if r, ok := fs.rows["removed.in_later_release"]; !ok || r.Value != "keep-me" {
		t.Fatal("unknown key must stay untouched")
	}
	if _, ok := fs.rows["retired.old_name"]; ok {
		t.Fatal("renamed key should leave the old name")
	}
	if r, ok := fs.rows["templates.import_timeout_renamed"]; !ok || !strings.HasPrefix(r.Value, "7s") {
		t.Fatalf("renamed key should carry the value, got %+v", r)
	}
}
