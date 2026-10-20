package dbupgrade

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func newTestManager(t *testing.T, now time.Time, dbs ...store.DesiredDatabase) (*Manager, *fakeStore) {
	t.Helper()
	s := newFakeStore(dbs...)
	ad := testAdvisor(t)
	return &Manager{
		Store: s, Advisor: ad, Now: func() time.Time { return now },
		Runner: &Runner{Store: s, Runtime: newFakeRuntime(s, ""), Catalog: ad.Catalog},
	}, s
}

func windowPolicy(name, level string) store.DBUpgradePolicy {
	return store.DBUpgradePolicy{DatabaseName: name, AutoUpgrade: level, WindowCron: "0 3 * * *", WindowDuration: 2 * time.Hour,
		WindowTimezone: "UTC", BackupBefore: true, VerifyAfter: true, RevertOnFailure: true}
}

func TestManagerSchedules(t *testing.T) {
	inWindow := time.Date(2026, 10, 10, 3, 15, 0, 0, time.UTC)
	outside := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	redis := store.DesiredDatabase{Name: "cache", Engine: "redis", Version: "7.2.10", BackupTargetID: "tgt"}
	tests := []struct {
		name     string
		now      time.Time
		db       store.DesiredDatabase
		policies []store.DBUpgradePolicy
		prior    []store.DBUpgradeRun
		want     string
	}{
		{"patch in window", inWindow, redis, []store.DBUpgradePolicy{windowPolicy("cache", store.DBAutoUpgradePatch)}, nil, "7.2.11"},
		{"minor in window", inWindow, redis, []store.DBUpgradePolicy{windowPolicy("cache", store.DBAutoUpgradeMinor)}, nil, "7.4.6"},
		{"inherits the platform default", inWindow, redis, []store.DBUpgradePolicy{windowPolicy(store.DBUpgradePlatformDefault, store.DBAutoUpgradePatch)}, nil, "7.2.11"},
		{"own off beats default on", inWindow, redis, []store.DBUpgradePolicy{
			windowPolicy(store.DBUpgradePlatformDefault, store.DBAutoUpgradePatch), windowPolicy("cache", store.DBAutoUpgradeOff)}, nil, ""},
		{"outside the window", outside, redis, []store.DBUpgradePolicy{windowPolicy("cache", store.DBAutoUpgradePatch)}, nil, ""},
		{"no backup target", inWindow, store.DesiredDatabase{Name: "cache", Engine: "redis", Version: "7.2.10"},
			[]store.DBUpgradePolicy{windowPolicy("cache", store.DBAutoUpgradePatch)}, nil, ""},
		{"stopped database", inWindow, store.DesiredDatabase{Name: "cache", Engine: "redis", Version: "7.2.10", BackupTargetID: "tgt", Suspended: true},
			[]store.DBUpgradePolicy{windowPolicy("cache", store.DBAutoUpgradePatch)}, nil, ""},
		{"manual engine never scheduled", inWindow, store.DesiredDatabase{Name: "cache", Engine: "mysql", Version: "8.0.40", BackupTargetID: "tgt"},
			[]store.DBUpgradePolicy{windowPolicy("cache", store.DBAutoUpgradeMinor)}, nil, ""},
		{"failed target is parked", inWindow, redis, []store.DBUpgradePolicy{windowPolicy("cache", store.DBAutoUpgradePatch)},
			[]store.DBUpgradeRun{{ID: "dbu_old", DatabaseName: "cache", ToVersion: "7.2.11", State: store.DBUpgradeStateReverted, CreatedAt: "2026-10-03T03:10:00Z"}}, ""},
		{"active run blocks a second", inWindow, redis, []store.DBUpgradePolicy{windowPolicy("cache", store.DBAutoUpgradePatch)},
			[]store.DBUpgradeRun{{ID: "dbu_old", DatabaseName: "cache", ToVersion: "7.2.11", State: store.DBUpgradeStateVerifying, CreatedAt: "2026-10-10T03:00:00Z"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, s := newTestManager(t, tt.now, tt.db)
			for _, p := range tt.policies {
				s.policies[p.DatabaseName] = p
			}
			for _, r := range tt.prior {
				s.runs[r.ID] = r
			}
			if err := m.Tick(context.Background()); err != nil {
				t.Fatalf("Tick() error = %v", err)
			}
			var created []store.DBUpgradeRun
			for _, r := range s.runs {
				if r.ID != "dbu_old" {
					created = append(created, r)
				}
			}
			if tt.want == "" {
				if len(created) != 0 {
					t.Fatalf("created %+v, want nothing", created)
				}
				return
			}
			if len(created) != 1 || created[0].ToVersion != tt.want || created[0].Source != store.DBUpgradeSourceAuto || created[0].State != store.DBUpgradeStatePending {
				t.Fatalf("created %+v, want one pending auto run to %s", created, tt.want)
			}
		})
	}
}

func TestManagerUpgradeNow(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	pg := store.DesiredDatabase{Name: "main", Engine: "postgres", Version: "16.9", BackupTargetID: "tgt"}
	tests := []struct {
		name    string
		db      store.DesiredDatabase
		version string
		active  bool
		wantErr error
		kind    string
	}{
		{"advised patch", pg, "16.11", false, nil, KindPatch},
		{"unadvised newer patch", pg, "16.12", false, nil, KindPatch},
		{"major refused", pg, "17.7", false, ErrInvalid, ""},
		{"downgrade refused", pg, "16.4", false, ErrInvalid, ""},
		{"garbage refused", pg, "latest", false, ErrInvalid, ""},
		{"already running", pg, "16.11", true, ErrConflict, ""},
		{"no backup target", store.DesiredDatabase{Name: "main", Engine: "postgres", Version: "16.9"}, "16.11", false, ErrConflict, ""},
		{"stopped", store.DesiredDatabase{Name: "main", Engine: "postgres", Version: "16.9", BackupTargetID: "tgt", Suspended: true}, "16.11", false, ErrConflict, ""},
		{"mysql manual patch allowed", store.DesiredDatabase{Name: "main", Engine: "mysql", Version: "8.0.40", BackupTargetID: "tgt"}, "8.0.43", false, nil, KindPatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, s := newTestManager(t, now, tt.db)
			if tt.active {
				s.runs["dbu_busy"] = store.DBUpgradeRun{ID: "dbu_busy", DatabaseName: "main", State: store.DBUpgradeStateUpgrading}
			}
			run, err := m.UpgradeNow(context.Background(), "main", tt.version, "admin")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("UpgradeNow() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpgradeNow() error = %v", err)
			}
			if run.Kind != tt.kind || run.Source != store.DBUpgradeSourceManual || run.RequestedBy != "admin" || !run.VerifyAfter || !run.RevertOnFailure {
				t.Errorf("run = %+v", run)
			}
		})
	}
	m, _ := newTestManager(t, now)
	if _, err := m.UpgradeNow(context.Background(), "missing", "16.11", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing database error = %v, want ErrNotFound", err)
	}
}

func TestManagerPolicyAndOverview(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	m, s := newTestManager(t, now, store.DesiredDatabase{Name: "main", Engine: "postgres", Version: "16.9"})
	s.policies[store.DBUpgradePlatformDefault] = windowPolicy(store.DBUpgradePlatformDefault, store.DBAutoUpgradeOff)
	ctx := context.Background()

	ov, err := m.Overview(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !ov.PolicyInherited || ov.NextTarget != nil || len(ov.Blockers) == 0 || ov.NextWindow.IsZero() {
		t.Errorf("inherited overview = %+v", ov)
	}

	bad := Policy{AutoUpgrade: KindMajor, BackupBefore: true}
	if err := m.SetPolicy(ctx, "main", bad, false, "admin"); !errors.Is(err, ErrInvalid) {
		t.Errorf("SetPolicy(major) error = %v, want ErrInvalid", err)
	}
	good := Policy{AutoUpgrade: store.DBAutoUpgradePatch, WindowCron: "0 4 * * *", WindowDuration: time.Hour, WindowTimezone: "UTC", BackupBefore: true, VerifyAfter: true, RevertOnFailure: true}
	if err := m.SetPolicy(ctx, "main", good, false, "admin"); err != nil {
		t.Fatalf("SetPolicy() error = %v", err)
	}
	ov, _ = m.Overview(ctx, "main")
	if ov.PolicyInherited || ov.NextTarget == nil || ov.NextTarget.Version != "16.11" {
		t.Errorf("own policy overview = %+v", ov)
	}
	if err := m.SetPolicy(ctx, "main", Policy{}, true, "admin"); err != nil {
		t.Fatalf("SetPolicy(inherit) error = %v", err)
	}
	if _, ok := s.policies["main"]; ok {
		t.Error("inherit did not drop the database's own policy")
	}
	if err := m.SetPolicy(ctx, "nope", good, false, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetPolicy(missing) error = %v", err)
	}

	sum, err := m.Summary(ctx, nil)
	if err != nil || len(sum) != 1 || !sum[0].Security {
		t.Errorf("Summary() = %+v, %v, want one database with a security update", sum, err)
	}
	if hidden, _ := m.Summary(ctx, func(string) bool { return false }); len(hidden) != 0 {
		t.Errorf("Summary(hidden) = %d items", len(hidden))
	}
}
