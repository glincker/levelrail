package upgradesched

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDefaultsResolveOnlyUnsavedPolicies(t *testing.T) {
	d := DefaultsFromEnv(func(string) (string, bool) { return "", false })
	unsaved := store.UpgradePolicy{Mode: ModeOff, MinReleaseAgeDays: 3}
	got, applied := d.Resolve(unsaved)
	if !applied || got.Mode != ModeAny || got.WindowCron == "" || got.WindowDurationSeconds <= 0 {
		t.Fatalf("unsaved policy must get the default: %+v applied=%t", got, applied)
	}
	saved := store.UpgradePolicy{Mode: ModeOff, UpdatedAt: "2026-11-01T00:00:00Z"}
	if got, applied := d.Resolve(saved); applied || got.Mode != ModeOff {
		t.Fatalf("an explicit off must be kept: %+v", got)
	}
	optOut := DefaultsFromEnv(func(k string) (string, bool) { return ModeOff, k == EnvDefaultMode })
	if _, applied := optOut.Resolve(unsaved); applied {
		t.Fatal("an env-seeded off default must leave unsaved installs off")
	}
	if _, applied := (Defaults{}).Resolve(unsaved); applied {
		t.Fatal("a zero Defaults must not turn anything on")
	}
}

func TestEligibleHonoursTimedPause(t *testing.T) {
	now := time.Date(2026, 11, 7, 3, 0, 0, 0, time.UTC)
	p := Policy{Mode: ModeAny, Window: Window{Cron: "0 3 * * *", Duration: 2 * time.Hour, Timezone: "UTC"}, PausedUntil: now.Add(48 * time.Hour)}
	cand := Candidate{Tag: "v0.9.5", PublishedAt: now.Add(-30 * 24 * time.Hour)}
	if v := Eligible(p, "v0.9.4", cand, 0, true, now); v.OK || v.Code != VerdictPaused || !v.WaitUntil.Equal(p.PausedUntil) {
		t.Fatalf("a timed pause must hold the release: %+v", v)
	}
	p.PausedUntil = now.Add(-time.Hour)
	if v := Eligible(p, "v0.9.4", cand, 0, true, now); !v.OK {
		t.Fatalf("an expired pause must not hold the release: %+v", v)
	}
	if v := Eligible(p, "v0.9.5", Candidate{Tag: "v0.9.3"}, 0, true, now); v.OK || v.Code != VerdictNotNewer {
		t.Fatalf("a downgrade must never be eligible: %+v", v)
	}
}
