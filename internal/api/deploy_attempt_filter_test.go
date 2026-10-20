package api

import (
	"net/http/httptest"
	"testing"
)

func TestDeployAttemptFilter(t *testing.T) {
	in := []deployAttemptResource{
		{ID: "a", Branch: "main", Author: "Ada", CommitSHA: "abc123", Status: "succeeded", CommitMessage: "fix login"},
		{ID: "b", Branch: "dev", Author: "Bob", CommitSHA: "def456", Status: "failed", CommitMessage: "wip"},
		{ID: "c", Branch: "main", Author: "Bob", CommitSHA: "abd789", Status: "succeeded"},
	}
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"none", "", "abc"},
		{"branch", "branch=main", "ac"},
		{"author substring", "author=bo", "bc"},
		{"commit prefix", "commit=ab", "ac"},
		{"status", "status=failed", "b"},
		{"text", "q=login", "a"},
		{"limit", "limit=1", "a"},
		{"combined", "branch=main&author=bob", "c"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/x?"+tc.query, nil)
			got := ""
			for _, a := range parseDeployAttemptFilter(r).apply(in) {
				got += a.ID
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
