package api

import "testing"

func TestClassifyInstall(t *testing.T) {
	cases := []struct {
		name string
		in   installStateInput
		want string
		migr int
	}{
		{"database ahead of release", installStateInput{Current: "v0.6.0", Latest: "v0.7.0", UpdateAvailable: true, DBSchema: 2690, LatestSchema: 2370}, UpdateStateAheadOfReleases, -1},
		{"schema equal, newer release", installStateInput{Current: "v0.6.0", Latest: "v0.7.0", UpdateAvailable: true, DBSchema: 2370, LatestSchema: 2370}, UpdateStateUpgradeAvailable, 0},
		{"database behind", installStateInput{Current: "v0.6.0", Latest: "v0.7.0", UpdateAvailable: true, DBSchema: 2360, LatestSchema: 2370}, UpdateStateUpgradeAvailable, 10},
		{"dev build string", installStateInput{Current: "v0.7.0-5-gabc1234", Latest: "v0.7.0", DBSchema: 2370, LatestSchema: 2370}, UpdateStateAheadOfReleases, -1},
		{"plain dev", installStateInput{Current: "dev", Latest: "v0.7.0", DBSchema: 2370, LatestSchema: 2370}, UpdateStateAheadOfReleases, -1},
		{"up to date", installStateInput{Current: "v0.7.0", Latest: "v0.7.0", DBSchema: 2370, LatestSchema: 2370}, UpdateStateUpToDate, 0},
		{"no release known", installStateInput{Current: "v0.7.0", DBSchema: 2370, LatestSchema: -1}, UpdateStateUnknown, -1},
		{"schema unknown, upgrade", installStateInput{Current: "v0.6.0", Latest: "v0.7.0", UpdateAvailable: true, DBSchema: 2370, LatestSchema: -1}, UpdateStateUpgradeAvailable, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyInstall(c.in)
			if got.State != c.want || got.Migrations != c.migr {
				t.Fatalf("got %s/%d, want %s/%d", got.State, got.Migrations, c.want, c.migr)
			}
		})
	}
}

func TestReleaseState(t *testing.T) {
	if releaseState(2690, 2370, false) != UpdateStateBlockedOlder {
		t.Fatal("older release must be blocked")
	}
	if releaseState(2690, 2690, false) == UpdateStateBlockedOlder {
		t.Fatal("equal schema must not be blocked")
	}
}
