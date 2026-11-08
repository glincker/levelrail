package runtimesettings

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestSettingsUpgrade_UnknownSavedKeyIgnored(t *testing.T) {
	fs := newFakeStore()
	fs.rows["retired.setting.from.old.release"] = store.RuntimeSettingRow{Key: "retired.setting.from.old.release", Value: "1"}
	svc := New(fs, envOf(nil))
	all, err := svc.All(context.Background())
	if err != nil {
		t.Fatalf("All with a retired saved key: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("expected registered settings")
	}
}

func TestSettingsUpgrade_LegacyKeysTargetRegistered(t *testing.T) {
	reg := map[string]bool{}
	for _, d := range Definitions() {
		reg[d.Key] = true
	}
	for old, cur := range legacyKeys {
		if !reg[cur] {
			t.Errorf("legacy key %q maps to unregistered %q", old, cur)
		}
		if reg[old] {
			t.Errorf("legacy key %q is still registered", old)
		}
	}
}

func TestSettingsUpgrade_RenameKeepsValue(t *testing.T) {
	prev := legacyKeys
	legacyKeys = map[string]string{"old.key": "new.key", "old.shadowed": "new.shadowed"}
	t.Cleanup(func() { legacyKeys = prev })
	now := time.Now()
	rows := map[string]store.RuntimeSettingRow{
		"old.key":      {Key: "old.key", Value: "a", UpdatedAt: now},
		"old.shadowed": {Key: "old.shadowed", Value: "x"},
		"new.shadowed": {Key: "new.shadowed", Value: "y"},
	}
	got := applyLegacyKeys(rows)
	if got["new.key"].Value != "a" || got["new.key"].Key != "new.key" {
		t.Errorf("rename lost value: %+v", got["new.key"])
	}
	if got["new.shadowed"].Value != "y" {
		t.Errorf("explicit new row must win: %+v", got["new.shadowed"])
	}
	if _, ok := got["old.key"]; ok {
		t.Error("old key should be dropped")
	}
}
