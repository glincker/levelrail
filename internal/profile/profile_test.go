package profile

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestResolveOrder(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		saved   string
		want    Profile
		wantSrc string
	}{
		{"nothing set", nil, "", Standard, SourceDefault},
		{"saved wins over default", nil, "minimal", Minimal, SourceSetting},
		{"env wins over saved", map[string]string{EnvVar: "suite"}, "minimal", Suite, SourceEnv},
		{"env is case insensitive", map[string]string{EnvVar: " SUITE "}, "", Suite, SourceEnv},
		{"bad env falls back to saved", map[string]string{EnvVar: "huge"}, "minimal", Minimal, SourceSetting},
		{"bad saved falls back to default", nil, "huge", Standard, SourceDefault},
	}
	for _, c := range cases {
		got, src := Resolve(env(c.env), c.saved)
		if got != c.want || src != c.wantSrc {
			t.Errorf("%s: got %s from %s, want %s from %s", c.name, got, src, c.want, c.wantSrc)
		}
	}
}

func TestMeshResolutionFollowsEnvThenSavedThenProfile(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		saved    bool
		savedSet bool
		profile  Profile
		want     bool
		wantSrc  string
	}{
		{"standard default off", nil, false, false, Standard, false, network.SettingSourceDefault},
		{"suite default on", nil, false, false, Suite, true, network.SettingSourceProfile},
		{"saved off beats suite", nil, false, true, Suite, false, network.SettingSourceSettings},
		{"saved on beats minimal", nil, true, true, Minimal, true, network.SettingSourceSettings},
		{"env off beats saved and suite", map[string]string{network.EnvMeshEnabled: "0"}, true, true, Suite, false, network.SettingSourceEnv},
	}
	for _, c := range cases {
		got, src := network.ResolveEnabledWithProfile(env(c.env), c.saved, c.savedSet, For(c.profile).Mesh)
		if got != c.want || src != c.wantSrc {
			t.Errorf("%s: got %v from %s, want %v from %s", c.name, got, src, c.want, c.wantSrc)
		}
	}
}

func TestRetention(t *testing.T) {
	const gib = int64(1) << 30
	cases := []struct {
		name string
		p    Profile
		disk int64
		env  map[string]string
		want time.Duration
	}{
		{"minimal is short", Minimal, 500 * gib, nil, 3 * 24 * time.Hour},
		{"standard on a big disk", Standard, 500 * gib, nil, 15 * 24 * time.Hour},
		{"standard on a small disk", Standard, 10 * gib, nil, 7 * 24 * time.Hour},
		{"suite on a small disk", Suite, 10 * gib, nil, 7 * 24 * time.Hour},
		{"unknown disk keeps the default", Standard, 0, nil, 15 * 24 * time.Hour},
		{"threshold from env", Standard, 30 * gib, map[string]string{EnvSmallDiskBytes: "42949672960"}, 7 * 24 * time.Hour},
		{"small disk retention from env", Standard, 10 * gib, map[string]string{EnvSmallDiskRetention: "48h"}, 48 * time.Hour},
		{"minimal retention from env", Minimal, 10 * gib, map[string]string{EnvMinimalRetention: "24h"}, 24 * time.Hour},
		{"bad env keeps the default", Minimal, 10 * gib, map[string]string{EnvMinimalRetention: "soon"}, 3 * 24 * time.Hour},
	}
	for _, c := range cases {
		if got := Retention(c.p, c.disk, env(c.env)); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestLookupFallsBackToProfileEnvDefaults(t *testing.T) {
	base := env(map[string]string{"APP_X": "1"})
	if v, ok := Lookup(base, Suite)("APP_BUILD_ATTEST"); !ok || v != "true" {
		t.Errorf("suite attest default = %q %v", v, ok)
	}
	if _, ok := Lookup(base, Standard)("APP_BUILD_ATTEST"); ok {
		t.Error("standard must not default attest on")
	}
	explicit := env(map[string]string{"APP_BUILD_ATTEST": "false"})
	if v, _ := Lookup(explicit, Suite)("APP_BUILD_ATTEST"); v != "false" {
		t.Errorf("explicit env must win, got %q", v)
	}
}

func TestCheckMoves(t *testing.T) {
	cases := []struct {
		name        string
		from, to    Profile
		facts       Facts
		wantCodes   []string
		wantBlocked bool
		wantConfirm bool
	}{
		{"up is free", Minimal, Suite, Facts{RemoteNodes: 3, EnabledPlugins: 2}, nil, false, false},
		{"same is free", Standard, Standard, Facts{RemoteNodes: 3}, nil, false, false},
		{"standard to minimal with nodes", Standard, Minimal, Facts{RemoteNodes: 1}, []string{GuardNodesEnrolled}, true, false},
		{"suite to standard stopping the mesh with nodes", Suite, Standard, Facts{RemoteNodes: 2, MeshStops: true}, []string{GuardNodesEnrolled}, true, false},
		{"suite to standard with the mesh pinned", Suite, Standard, Facts{RemoteNodes: 2}, nil, false, false},
		{"suite to standard with plugins", Suite, Standard, Facts{EnabledPlugins: 1}, []string{GuardPluginsRunning}, true, false},
		{"standard to minimal shortens retention", Standard, Minimal, Facts{Retention: 15 * 24 * time.Hour, NewRetention: 3 * 24 * time.Hour}, []string{GuardRetentionShorter}, false, true},
		{"equal retention needs no confirm", Suite, Standard, Facts{Retention: 15 * 24 * time.Hour, NewRetention: 15 * 24 * time.Hour}, nil, false, false},
	}
	for _, c := range cases {
		ps := Check(c.from, c.to, c.facts)
		var codes []string
		for _, p := range ps {
			codes = append(codes, p.Code)
		}
		if len(codes) != len(c.wantCodes) {
			t.Errorf("%s: codes %v want %v", c.name, codes, c.wantCodes)
			continue
		}
		for i := range codes {
			if codes[i] != c.wantCodes[i] {
				t.Errorf("%s: codes %v want %v", c.name, codes, c.wantCodes)
			}
		}
		if Blocked(ps) != c.wantBlocked || NeedsConfirm(ps) != c.wantConfirm {
			t.Errorf("%s: blocked %v confirm %v", c.name, Blocked(ps), NeedsConfirm(ps))
		}
	}
}

func TestSuiteEnablesEveryExperimentalFeatureAndMinimalNone(t *testing.T) {
	if len(For(Suite).Features) < 13 || !For(Suite).Mesh {
		t.Errorf("suite = %+v", For(Suite))
	}
	if m := For(Minimal); len(m.Features) != 0 || m.Mesh || !m.LoopbackAgent || !m.LazyLoops {
		t.Errorf("minimal = %+v", m)
	}
	if s := For(Standard); len(s.Features) != 0 || s.Mesh || s.LoopbackAgent || s.LazyLoops {
		t.Errorf("standard must keep today's defaults, got %+v", s)
	}
}
