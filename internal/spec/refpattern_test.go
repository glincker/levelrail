package spec

import (
	"strings"
	"testing"
	"time"
)

func TestMatchRefPatterns(t *testing.T) {
	tests := []struct {
		patterns, name string
		want           bool
	}{
		{"", "anything", true},
		{"v*", "v1.2.3", true},
		{"v*", "release-1", false},
		{"v*", "v1/hotfix", false},
		{"v**", "v1/hotfix", true},
		{"release/*", "release/2026.11", true},
		{"release/*", "release/a/b", false},
		{"release/**", "release/a/b", true},
		{"main,release/*", "main", true},
		{"main\nrelease/*", "release/x", true},
		{"main, release/*", "feature/x", false},
		{"release/*,!release/old", "release/old", false},
		{"!release/old,release/*", "release/old", false},
		{"!release/old,release/*", "release/new", true},
		{"!wip/*", "main", true},
		{"!wip/*", "wip/x", false},
		{"v?.?", "v1.2", true},
		{"v?.?", "v1/2", false},
		{"**", "a/b/c", true},
		{"*", "", true},
		{"v?", "vé", true},
		{"caf?/x", "café/x", true},
		{"v??", "vé", false},
	}
	for _, tt := range tests {
		if got := MatchRefPatterns(tt.patterns, tt.name); got != tt.want {
			t.Errorf("MatchRefPatterns(%q, %q) = %v, want %v", tt.patterns, tt.name, got, tt.want)
		}
	}
}

func TestShortRefNameHostileRefs(t *testing.T) {
	tests := []struct {
		ref      string
		wantKind string
		wantOK   bool
	}{
		{"refs/heads/main", RefKindBranch, true},
		{"refs/tags/v1", RefKindTag, true},
		{"refs/pull/1/head", "", false},
		{"refs/heads/", "", false},
		{"main", "", false},
		{"refs/heads/a\x00b", "", false},
		{"refs/heads/a\nb", "", false},
		{"refs/heads/\xff\xfe", "", false},
		{"refs/heads/" + strings.Repeat("a", 2000), "", false},
		{"refs/heads/café", RefKindBranch, true},
	}
	for _, tt := range tests {
		_, kind, ok := ShortRefName(tt.ref, 1024)
		if ok != tt.wantOK || kind != tt.wantKind {
			t.Errorf("ShortRefName(%.40q) = %q %v, want %q %v", tt.ref, kind, ok, tt.wantKind, tt.wantOK)
		}
	}
}

func TestGlobMatchStaysFastOnHostileInput(t *testing.T) {
	pattern := strings.Repeat("*a", 60) + "b"
	name := strings.Repeat("a", 64*1024)
	start := time.Now()
	if globMatch(pattern, name) {
		t.Fatal("hostile name matched")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("match took %v, want linear time", d)
	}
}

func TestValidateRefPatterns(t *testing.T) {
	lim := RefPatternLimits{MaxPatterns: 3, MaxPatternLen: 16, MaxListBytes: 30}
	ok := []string{"", "v*", "main,release/*,!release/old", "v1.2.+@_-"}
	for _, raw := range ok {
		if err := ValidateRefPatterns(raw, lim); err != nil {
			t.Errorf("ValidateRefPatterns(%q) = %v, want nil", raw, err)
		}
	}
	bad := []string{"!", "refs/heads/main", "v[0-9]", "a{b,c}", "a b", "a***", "a\x01", "a,b,c,d", strings.Repeat("a", 17), "aaaaaaaaaaaaaaa,bbbbbbbbbbbbbbb,c", "^v1$"}
	for _, raw := range bad {
		if err := ValidateRefPatterns(raw, lim); err == nil {
			t.Errorf("ValidateRefPatterns(%q) = nil, want an error", raw)
		}
	}
}

func TestNormalizeRefPatterns(t *testing.T) {
	if got := NormalizeRefPatterns(" main ,\nrelease/* ,,"); got != "main\nrelease/*" {
		t.Fatalf("NormalizeRefPatterns = %q", got)
	}
}
