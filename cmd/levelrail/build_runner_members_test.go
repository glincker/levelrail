package main

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDeveloperMemberIDsSkipsViewers(t *testing.T) {
	got := developerMemberIDs([]store.TeamMember{{UserID: "u1", Role: store.TeamRoleViewer}, {UserID: "u2", Role: store.TeamRoleDeveloper}, {UserID: "u3", Role: store.TeamRoleOwner}})
	if len(got) != 2 || got[0] != "u2" || got[1] != "u3" {
		t.Fatalf("members = %v, want developer and above only", got)
	}
}
