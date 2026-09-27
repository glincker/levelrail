package agentinit

import "testing"

func TestDiff(t *testing.T) {
	tests := []struct{ name, old, next, want string }{
		{"equal", "a\nb\n", "a\nb\n", ""},
		{"add", "a\n", "a\nb\n", "+ b\n"},
		{"remove", "a\nb\n", "a\n", "- b\n"},
		{"change", "a\nb\nc\n", "a\nX\nc\n", "+ X\n- b\n"},
		{"from empty", "", "a\n", "+ a\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Diff(tc.old, tc.next); got != tc.want {
				t.Errorf("Diff = %q, want %q", got, tc.want)
			}
		})
	}
}
