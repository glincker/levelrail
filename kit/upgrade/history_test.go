package upgrade

import "testing"

func TestVerdictFor(t *testing.T) {
	tests := []struct {
		name       string
		db, target int
		want       SchemaVerdict
	}{
		{"equal", 50, 50, VerdictBinaryOnly},
		{"older target", 50, 40, VerdictRestoreRequired},
		{"newer target", 50, 60, VerdictForward},
		{"unknown target", 50, -1, VerdictUnknown},
		{"unknown database", -1, 50, VerdictUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := VerdictFor(tc.db, tc.target); got != tc.want {
				t.Fatalf("VerdictFor(%d, %d) = %s, want %s", tc.db, tc.target, got, tc.want)
			}
		})
	}
}

func TestSelectHistory(t *testing.T) {
	rrs := []rawHistory{
		{TagName: "v0.2.0-beta.9", PublishedAt: "2026-09-01T00:00:00Z", Prerelease: true},
		{TagName: "v0.2.0-beta.14", PublishedAt: "2026-09-10T00:00:00Z", Prerelease: true},
		{TagName: "v0.1.0", PublishedAt: "2026-08-01T00:00:00Z"},
		{TagName: "v0.1.1", PublishedAt: "2026-08-20T00:00:00Z"},
		{TagName: "v0.3.0", PublishedAt: "2026-10-01T00:00:00Z", Draft: true},
		{TagName: "v0.0.1", PublishedAt: "not a date"},
	}
	tests := []struct {
		name    string
		channel string
		limit   int
		want    []string
	}{
		{"stable only", ChannelStable, 5, []string{"v0.1.1", "v0.1.0"}},
		{"beta only, publish order not list order", ChannelBeta, 5, []string{"v0.2.0-beta.14", "v0.2.0-beta.9"}},
		{"all, limited", ChannelAll, 3, []string{"v0.2.0-beta.14", "v0.2.0-beta.9", "v0.1.1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := selectHistory(rrs, tc.channel, tc.limit)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d releases, want %d", len(got), len(tc.want))
			}
			for i, w := range tc.want {
				if got[i].Tag != w {
					t.Errorf("[%d] = %s, want %s", i, got[i].Tag, w)
				}
			}
		})
	}
}

func TestFetchManifestRejectsNonGitHub(t *testing.T) {
	for _, u := range []string{"http://github.com/x", "https://evil.test/manifest.json", "file:///etc/passwd"} {
		if _, err := FetchManifest(t.Context(), u); err == nil {
			t.Errorf("FetchManifest(%q) succeeded", u)
		}
	}
}
