package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBranchDeployCoalesced(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	row := func(status, sha string, age time.Duration) *store.BranchDeploy {
		return &store.BranchDeploy{Status: status, HeadSHA: sha, UpdatedAt: now.Add(-age).Format(time.RFC3339Nano)}
	}
	window := 15 * time.Second
	tests := []struct {
		name string
		row  *store.BranchDeploy
		sha  string
		want bool
	}{
		{"same commit inside window", row(store.BranchDeployDeploying, "abc", 3*time.Second), "abc", true},
		{"same commit after window", row(store.BranchDeployDeploying, "abc", time.Minute), "abc", false},
		{"new commit", row(store.BranchDeployDeploying, "abc", time.Second), "def", false},
		{"finished deploy", row(store.BranchDeployFailed, "abc", time.Second), "abc", false},
		{"empty sha", row(store.BranchDeployDeploying, "", time.Second), "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := branchDeployCoalesced(tc.row, tc.sha, now, window); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
