package store

import (
	"testing"
	"time"
)

func TestFindSilence(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	tests := []struct {
		name    string
		silence Silence
		key     string
		scope   string
		want    bool
	}{
		{"active", Silence{IssueKey: "doctor:disk", Until: &future}, "doctor:disk", "", true},
		{"expired", Silence{IssueKey: "doctor:disk", Until: &past}, "doctor:disk", "", false},
		{"forever", Silence{IssueKey: "doctor:disk"}, "doctor:disk", "", true},
		{"other key", Silence{IssueKey: "doctor:disk"}, "doctor:tls", "", false},
		{"scope match", Silence{IssueKey: "node:a", Scope: "n1"}, "node:a", "n1", true},
		{"scope mismatch", Silence{IssueKey: "node:a", Scope: "n1"}, "node:a", "n2", false},
		{"unscoped covers any scope", Silence{IssueKey: "node:a"}, "node:a", "n2", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := FindSilence([]Silence{tt.silence}, tt.key, tt.scope, now)
			if got != tt.want {
				t.Fatalf("FindSilence = %v, want %v", got, tt.want)
			}
		})
	}
}
