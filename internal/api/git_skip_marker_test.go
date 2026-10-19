package api

import (
	"strings"
	"testing"
)

func TestSkipPushForMarker(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, msg string
		skipped   bool
	}{
		{"plain", "fix: bug", false},
		{"skip ci", "docs: typo [skip ci]", true},
		{"ci skip upper", "docs: typo [CI SKIP]", true},
		{"skip deploy", "wip [skip deploy]\n\nbody", true},
		{"no deploy", "[no deploy] chore", true},
		{"empty", "", false},
		{"lookalike", "skip ci without brackets", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := skipPushForMarker(tc.msg)
			if (got != "") != tc.skipped {
				t.Fatalf("skipPushForMarker(%q) = %q, skipped want %v", tc.msg, got, tc.skipped)
			}
			if tc.skipped && !strings.HasPrefix(got, "ignored:") {
				t.Fatalf("message %q should start with ignored:", got)
			}
		})
	}
}
