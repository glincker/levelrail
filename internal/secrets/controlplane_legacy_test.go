package secrets

import (
	"context"
	"testing"
)

var legacyCases = []struct{ legacy, key, val string }{
	{"webpush", "vapid_private_key", "priv"},
	{"heartbeat", "url", "https://h.example/ping"},
	{"heartbeat-jobs", "task_t1_url", "https://j.example"},
	{"token-lifecycle", "expiry_notices", `{"a":1}`},
	{"secretconnections", "TOKEN_GH", "tok"},
	{"secretproviders", "CFG_VAULT", `{"x":1}`},
	{"ddns", "settings", `{"p":"x"}`},
	{"ddns", "target", "https://t.example"},
	{"monitor-abc", "hdr:X-Auth", "bearer"},
	{"model/llama", "HF_TOKEN", "hf"},
}

func TestLegacyMigration_EachFamilyMovesOnReadAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	for _, c := range legacyCases {
		t.Run(c.legacy, func(t *testing.T) {
			m, _ := testManager(t)
			nw := ControlPlaneService(c.legacy)
			if err := m.setValue(ctx, c.legacy, c.key, c.val); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				got, err := m.Resolve(ctx, nw, c.key)
				if err != nil || got != c.val {
					t.Fatalf("read %d: got %q, %v", i, got, err)
				}
			}
			if ok, _ := m.exists(ctx, c.legacy, c.key); ok {
				t.Fatal("legacy entry should be gone")
			}
		})
	}
}

func TestLegacyMigration_ExistsListAndDelete(t *testing.T) {
	ctx := context.Background()
	m, _ := testManager(t)
	nw := ControlPlaneService("secretproviders")
	_ = m.setValue(ctx, "secretproviders", "TOKEN_A", "a")
	_ = m.setValue(ctx, "secretproviders", "CFG_A", "c")
	keys, err := m.ListKeys(ctx, nw)
	if err != nil || len(keys) != 2 {
		t.Fatalf("list: %v %v", keys, err)
	}
	if ok, _ := m.Exists(ctx, nw, "TOKEN_A"); !ok {
		t.Fatal("exists after migration")
	}
	if err := m.DeleteValue(ctx, nw, "TOKEN_A"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Resolve(ctx, nw, "TOKEN_A"); err == nil {
		t.Fatal("deleted value must not come back")
	}
}

func TestLegacyMigration_NeverOverwritesAndFallsBack(t *testing.T) {
	ctx := context.Background()
	m, _ := testManager(t)
	nw := ControlPlaneService("heartbeat")
	_ = m.setValue(ctx, "heartbeat", "url", "old")
	_ = m.setValue(ctx, nw, "url", "new")
	if got, _ := m.Resolve(ctx, nw, "url"); got != "new" {
		t.Fatalf("new entry must win, got %q", got)
	}
	if got, _ := m.resolve(ctx, "heartbeat", "url"); got != "old" {
		t.Fatal("differing legacy entry must stay")
	}
}

func TestLegacyMigration_AppSlotsAreNotSwept(t *testing.T) {
	ctx := context.Background()
	m, _ := testManager(t)
	_ = m.setValue(ctx, "heartbeat", "DATABASE_URL", "appvalue")
	if _, err := m.Resolve(ctx, ControlPlaneService("heartbeat"), "DATABASE_URL"); err == nil {
		t.Fatal("an app key must not be read as a control plane key")
	}
	if got, _ := m.resolve(ctx, "heartbeat", "DATABASE_URL"); got != "appvalue" {
		t.Fatal("app value must be untouched")
	}
}

func TestLegacyMigration_DDNSKeepsAppSlotAndExistingValue(t *testing.T) {
	ctx := context.Background()
	m, _ := testManager(t)
	nw := ControlPlaneService("ddns")
	_ = m.setValue(ctx, "ddns", "API_KEY", "appvalue")
	_ = m.setValue(ctx, "ddns", "settings", "old")
	_ = m.setValue(ctx, nw, "settings", "new")
	if got, _ := m.Resolve(ctx, nw, "settings"); got != "new" {
		t.Fatalf("new entry must win, got %q", got)
	}
	if _, err := m.Resolve(ctx, nw, "API_KEY"); err == nil {
		t.Fatal("app key must not be read as a control plane key")
	}
	if got, _ := m.resolve(ctx, "ddns", "API_KEY"); got != "appvalue" {
		t.Fatal("app value must be untouched")
	}
}
