package alerting

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

type fakeVersionSkewSettings struct {
	settings store.UpdateSettings
	err      error
}

func (f fakeVersionSkewSettings) GetUpdateSettings(context.Context) (store.UpdateSettings, error) {
	return f.settings, f.err
}

func fakeVersionSkewFetchers(release *upgrade.Release, err error) upgrade.Fetchers {
	fn := func(context.Context) (*upgrade.Release, error) { return release, err }
	return upgrade.Fetchers{Stable: fn, Beta: fn, Edge: fn}
}

func TestEvaluateVersionSkew(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		settings       store.UpdateSettings
		release        *upgrade.Release
		currentVersion string
		wantFiring     bool
		wantNotice     string
	}{
		{name: "up to date", settings: store.UpdateSettings{Channel: "stable"}, release: &upgrade.Release{Tag: "v1.0.0"}, currentVersion: "v1.0.0", wantFiring: false},
		{name: "behind stable", settings: store.UpdateSettings{Channel: "stable"}, release: &upgrade.Release{Tag: "v1.1.0"}, currentVersion: "v1.0.0", wantFiring: true, wantNotice: "stable channel's latest is v1.1.0"},
		{name: "behind beta", settings: store.UpdateSettings{Channel: "beta"}, release: &upgrade.Release{Tag: "v1.1.0-beta.1"}, currentVersion: "v1.0.0", wantFiring: true, wantNotice: "beta channel's latest is v1.1.0-beta.1"},
		{name: "invalid stored channel falls back to stable", settings: store.UpdateSettings{Channel: "nightly"}, release: &upgrade.Release{Tag: "v1.1.0"}, currentVersion: "v1.0.0", wantFiring: true, wantNotice: "stable channel's latest is v1.1.0"},
		{name: "dev build never fires", settings: store.UpdateSettings{Channel: "stable"}, release: &upgrade.Release{Tag: "v1.1.0"}, currentVersion: "dev", wantFiring: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Rule{ID: "r1", Kind: KindVersionSkew}
			next, notice, err := EvaluateVersionSkew(context.Background(),
				fakeVersionSkewSettings{settings: tt.settings}, fakeVersionSkewFetchers(tt.release, nil), upgrade.NewCache(),
				r, tt.currentVersion, now)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if next.Firing != tt.wantFiring {
				t.Fatalf("firing = %v, want %v (notice %q)", next.Firing, tt.wantFiring, notice)
			}
			if tt.wantNotice != "" && !strings.Contains(notice, tt.wantNotice) {
				t.Fatalf("notice %q missing %q", notice, tt.wantNotice)
			}
		})
	}
}

func TestEvaluateVersionSkew_SettingsErrorPropagates(t *testing.T) {
	r := Rule{ID: "r1", Kind: KindVersionSkew}
	_, _, err := EvaluateVersionSkew(context.Background(),
		fakeVersionSkewSettings{err: errors.New("db unavailable")}, fakeVersionSkewFetchers(nil, nil), upgrade.NewCache(),
		r, "v1.0.0", time.Now())
	if err == nil {
		t.Fatal("err = nil, want the store error surfaced")
	}
}

func TestEvaluateVersionSkew_FetchErrorFallsBackToStaleCache(t *testing.T) {
	now := time.Now()
	cache := upgrade.NewCache()
	cache.Set(upgrade.ChannelStable, &upgrade.Release{Tag: "v1.1.0"})

	r := Rule{ID: "r1", Kind: KindVersionSkew}
	next, notice, err := EvaluateVersionSkew(context.Background(),
		fakeVersionSkewSettings{settings: store.UpdateSettings{Channel: "stable"}},
		fakeVersionSkewFetchers(nil, errors.New("github unreachable")), cache,
		r, "v1.0.0", now)
	if err != nil {
		t.Fatalf("err: %v, want the stale cache entry to cover a failed fetch", err)
	}
	if !next.Firing {
		t.Errorf("firing = false, want true (stale cache still shows v1.1.0 available), notice = %q", notice)
	}
}

func TestEvaluateVersionSkew_FetchErrorNoCacheReturnsError(t *testing.T) {
	r := Rule{ID: "r1", Kind: KindVersionSkew}
	_, _, err := EvaluateVersionSkew(context.Background(),
		fakeVersionSkewSettings{settings: store.UpdateSettings{Channel: "stable"}},
		fakeVersionSkewFetchers(nil, errors.New("github unreachable")), upgrade.NewCache(),
		r, "v1.0.0", time.Now())
	if err == nil {
		t.Fatal("err = nil, want the fetch failure surfaced when there is no stale cache to fall back on")
	}
}
