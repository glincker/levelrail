package vault

import "testing"

func TestLegacyPathAllowed(t *testing.T) {
	prefixes := []string{"secret/data/team-a/", "kv/shared"}
	cases := []struct {
		path string
		want bool
	}{
		{"secret/data/team-a/db", true},
		{"secret/data/team-a", true},
		{"kv/shared/x", true},
		{"secret/data/team-ab/db", false},
		{"secret/data/team-b/db", false},
		{"secret/data/team-a/../team-b", false},
		{"", false},
	}
	for _, c := range cases {
		if got := LegacyPathAllowed(c.path, prefixes); got != c.want {
			t.Errorf("LegacyPathAllowed(%q) = %v, want %v", c.path, got, c.want)
		}
	}
	if LegacyPathAllowed("secret/x", nil) {
		t.Error("an empty allowlist must allow nothing")
	}
}
