package api

import "testing"

func TestRedactURLCredentials(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no credentials", in: "https://github.com/org/repo.git", want: "https://github.com/org/repo.git"},
		{name: "token as username", in: "https://ghp_secret@github.com/org/repo.git", want: "https://redacted@github.com/org/repo.git"},
		{name: "user and password", in: "https://user:hunter2@gitlab.example.com/repo", want: "https://redacted@gitlab.example.com/repo"}, //nolint:gosec // test fixture, not a real credential
		{name: "scp style ssh", in: "git@github.com:org/repo.git", want: "<unparseable url>"},
		{name: "ssh url", in: "ssh://git@github.com/org/repo.git", want: "ssh://redacted@github.com/org/repo.git"},
		{name: "empty", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactURLCredentials(tt.in); got != tt.want {
				t.Errorf("redactURLCredentials(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
