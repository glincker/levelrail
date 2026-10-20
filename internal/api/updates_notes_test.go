package api

import "testing"

func TestCleanReleaseNotes(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"markers removed", "<!-- release-notes:start -->\n> [!NOTE]\n> hi\n<!-- release-notes:end -->", "> [!NOTE]\n> hi"},
		{"no comments", "  plain  ", "plain"},
		{"unterminated comment dropped", "keep <!-- never closed", "keep"},
		{"multiple", "a<!-- x -->b<!-- y -->c", "abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanReleaseNotes(tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
