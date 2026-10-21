package alerting

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultsConfigFromEnv(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []int
	}{
		{"unset keeps defaults", "", []int{14, 7, 3, 1}},
		{"custom", "30, 10,2", []int{30, 10, 2}},
		{"garbage keeps defaults", "7,abc", []int{14, 7, 3, 1}},
		{"non positive keeps defaults", "7,0", []int{14, 7, 3, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultsConfigFromEnv(func(string) string { return tt.raw }).CertExpiryDays
			if len(got) != len(tt.want) {
				t.Fatalf("days = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("days = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestDefaultRuleSpecs_EscalatingCertRules(t *testing.T) {
	var certs []DefaultRuleSpec
	kinds := map[Kind]bool{}
	for _, s := range DefaultRuleSpecs(DefaultsConfig{CertExpiryDays: []int{1, 14, 3, 7}}) {
		kinds[s.Rule.Kind] = true
		if s.Rule.Kind == KindCertExpiry {
			certs = append(certs, s)
		}
	}
	wantSev := []string{SeverityInfo, SeverityWarning, SeverityCritical, SeverityCritical}
	wantDays := []time.Duration{14, 7, 3, 1}
	if len(certs) != 4 {
		t.Fatalf("cert rules = %d, want 4", len(certs))
	}
	for i, c := range certs {
		if c.Rule.ForDuration != wantDays[i]*24*time.Hour || c.Rule.Severity != wantSev[i] {
			t.Errorf("cert rule %d = %v/%s, want %v days/%s", i, c.Rule.ForDuration, c.Rule.Severity, wantDays[i], wantSev[i])
		}
	}
	if !kinds[KindRestoreDrillFailed] {
		t.Error("restore_drill_failed default missing")
	}
}

func TestSeedDefaults(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultsConfig{}
	specs := DefaultRuleSpecs(cfg)

	tests := []struct {
		name  string
		setup func(t *testing.T, db *DB)
		check func(t *testing.T, db *DB)
	}{
		{"first boot creates every enabled rule", func(*testing.T, *DB) {}, func(t *testing.T, db *DB) {
			got, err := db.ListDefaultRules(ctx)
			if err != nil || len(got) != len(specs) {
				t.Fatalf("rules = %d (err %v), want %d", len(got), err, len(specs))
			}
			for _, r := range got {
				if !r.Enabled || r.ResourceID != DefaultRuleResource {
					t.Errorf("rule %s enabled=%v resource=%q", r.ID, r.Enabled, r.ResourceID)
				}
			}
		}},
		{"operator edits survive", func(t *testing.T, db *DB) {
			if err := db.SeedDefaults(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			r, _ := db.GetRule(ctx, "default-restore-drill-failed")
			r.Enabled, r.Name = false, "mine"
			if err := db.SaveRule(ctx, *r); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, db *DB) {
			r, err := db.GetRule(ctx, "default-restore-drill-failed")
			if err != nil || r.Enabled || r.Name != "mine" {
				t.Fatalf("rule = %+v, err %v; edit was overwritten", r, err)
			}
		}},
		{"deleted rule stays deleted", func(t *testing.T, db *DB) {
			if err := db.SeedDefaults(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			if err := db.DeleteRule(ctx, "default-node-offline"); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, db *DB) {
			if _, err := db.GetRule(ctx, "default-node-offline"); !errors.Is(err, ErrRuleNotFound) {
				t.Fatalf("err = %v, want ErrRuleNotFound", err)
			}
		}},
		{"half-succeeded seed: rule saved, marker missing", func(t *testing.T, db *DB) {
			if err := db.SeedDefaults(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			r, _ := db.GetRule(ctx, "default-node-offline")
			r.Name = "edited before marker"
			if err := db.SaveRule(ctx, *r); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `DELETE FROM alert_default_seeds WHERE key = 'default-node-offline'`); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, db *DB) {
			r, err := db.GetRule(ctx, "default-node-offline")
			if err != nil || r.Name != "edited before marker" {
				t.Fatalf("rule = %+v, err %v; existing rule must be kept", r, err)
			}
			var n int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alert_default_seeds`).Scan(&n); err != nil || n != len(specs) {
				t.Fatalf("markers = %d (err %v), want %d", n, err, len(specs))
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			tt.setup(t, db)
			for i := 0; i < 2; i++ {
				if err := db.SeedDefaults(ctx, cfg); err != nil {
					t.Fatalf("SeedDefaults pass %d: %v", i, err)
				}
			}
			tt.check(t, db)
		})
	}
}

func TestResetDefaultRules(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	if err := db.SeedDefaults(ctx, DefaultsConfig{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetDefaultRuleEnabled(ctx, "default-node-offline", false); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteRule(ctx, "default-restore-drill-failed"); err != nil {
		t.Fatal(err)
	}
	rules, err := db.ResetDefaultRules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != len(DefaultRuleSpecs(DefaultsConfig{})) {
		t.Fatalf("got %d rules after reset", len(rules))
	}
	for _, r := range rules {
		if !r.Enabled {
			t.Fatalf("rule %s still disabled after reset", r.ID)
		}
	}
}

func TestSetDefaultRuleEnabled(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	if err := db.SeedDefaults(ctx, DefaultsConfig{}); err != nil {
		t.Fatal(err)
	}
	if r, err := db.SetDefaultRuleEnabled(ctx, "default-node-offline", false); err != nil || r.Enabled {
		t.Fatalf("disable = %+v, %v", r, err)
	}
	if _, err := db.SetDefaultRuleEnabled(ctx, "nope", true); !errors.Is(err, ErrRuleNotFound) {
		t.Fatalf("non-default id err = %v", err)
	}
	if err := db.SaveRule(ctx, Rule{ID: "default-app", ResourceID: "app-x", Kind: KindCrashloop}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetDefaultRuleEnabled(ctx, "default-app", true); !errors.Is(err, ErrRuleNotFound) {
		t.Fatalf("app rule err = %v", err)
	}
}
