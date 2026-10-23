package branchdeploy

import (
	"strings"
	"testing"
)

func TestSlug(t *testing.T) {
	tests := []struct {
		name, branch string
		maxLen       int
		want         string
		exact        bool
	}{
		{"plain keeps name", "staging", 40, "staging", true},
		{"dashes and digits kept", "release-1-2", 40, "release-1-2", true},
		{"slash gets hash", "feature/login", 40, "feature-login-", false},
		{"uppercase gets hash", "Feature", 40, "feature-", false},
		{"symbols only", "///", 40, "branch-", false},
		{"unicode", "fix/über", 40, "fix-ber-", false},
		{"long is capped", strings.Repeat("a", 90), 20, "aaaaaaaaaaaaa-", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Slug(tc.branch, tc.maxLen)
			if tc.exact && got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
			if !tc.exact && !strings.HasPrefix(got, tc.want) {
				t.Fatalf("got %q want prefix %q", got, tc.want)
			}
			if len(got) > tc.maxLen {
				t.Fatalf("slug %q longer than %d", got, tc.maxLen)
			}
			if strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") || strings.Contains(got, "--") {
				t.Fatalf("slug %q has bad dashes", got)
			}
		})
	}
}

func TestSlugCollisionSafe(t *testing.T) {
	branches := []string{"feature/x", "feature-x", "Feature-X", "feature_x", "feature.x", strings.Repeat("a", 80), strings.Repeat("a", 81)}
	seen := map[string]string{}
	for _, b := range branches {
		s := Slug(b, 30)
		if other, dup := seen[s]; dup {
			t.Fatalf("%q and %q share slug %q", b, other, s)
		}
		seen[s] = b
		if Slug(b, 30) != s {
			t.Fatalf("slug for %q is not deterministic", b)
		}
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"staging", "staging", true},
		{"staging", "staging2", false},
		{"feature/*", "feature/a/b", true},
		{"feature/*", "feature", false},
		{"*", "anything/at/all", true},
		{"*-hotfix", "x-hotfix", true},
		{"*-hotfix", "x-hotfix-2", false},
		{"re*ase/*", "release/1", true},
		{"a*b*c", "axxbxxc", true},
		{"a*b*c", "axxbxx", false},
		{"", "x", false},
	}
	for _, tc := range tests {
		if got := MatchGlob(tc.pattern, tc.name); got != tc.want {
			t.Errorf("MatchGlob(%q,%q)=%v want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

func TestEligible(t *testing.T) {
	tests := []struct {
		name   string
		s      Settings
		branch string
		want   bool
	}{
		{"off", Settings{Mode: ModeOff}, "dev", false},
		{"zero value", Settings{}, "dev", false},
		{"all", Settings{Mode: ModeAll}, "dev", true},
		{"all skips production", Settings{Mode: ModeAll}, "main", false},
		{"patterns hit", Settings{Mode: ModePatterns, Patterns: []string{"staging", "feat/*"}}, "feat/x", true},
		{"patterns miss", Settings{Mode: ModePatterns, Patterns: []string{"staging"}}, "dev", false},
		{"empty branch", Settings{Mode: ModeAll}, "", false},
	}
	for _, tc := range tests {
		if got := tc.s.Eligible(tc.branch, "main"); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestMaxSlugLenAndAppName(t *testing.T) {
	if got := MaxSlugLen("web"); got != 63-3-4 {
		t.Fatalf("got %d", got)
	}
	if got := MaxSlugLen(strings.Repeat("a", 70)); got != minSlugLen {
		t.Fatalf("floor not applied: %d", got)
	}
	if got := AppName("web", "dev"); got != "web-br-dev" {
		t.Fatalf("got %q", got)
	}
}
