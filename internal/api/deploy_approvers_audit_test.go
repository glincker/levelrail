package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

// A two-approver release stays pending after one vote, ignores a repeat vote
// from the same person, releases on the second distinct approver, and the
// audit log keeps one row per voter.
func TestRequiredApprovers_AuditRecordsBothApprovers(t *testing.T) {
	rt, db, _, b, c, id := multiFixture(t)
	if got := postApproval(t, rt, b, id, "approve"); got != http.StatusAccepted {
		t.Fatalf("first vote = %d, want 202", got)
	}
	if got := postApproval(t, rt, b, id, "approve"); got == http.StatusOK {
		t.Fatalf("repeat vote by same approver released the deploy")
	}
	if s := approvalStatus(t, db, id); s != store.DeployApprovalStatusPending {
		t.Fatalf("status after one distinct approver = %s, want pending", s)
	}
	if got := postApproval(t, rt, c, id, "approve"); got != http.StatusOK {
		t.Fatalf("second distinct approver = %d, want 200", got)
	}
	if s := approvalStatus(t, db, id); s != store.DeployApprovalStatusApproved {
		t.Fatalf("status = %s, want approved", s)
	}
	rows, err := db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{Path: "/api/v1/deploy-approvals/" + id + "/approve"})
	if err != nil {
		t.Fatal(err)
	}
	actors := map[string]bool{}
	for _, r := range rows {
		actors[r.ActorID] = true
	}
	if len(actors) != 2 {
		t.Fatalf("distinct approvers in audit = %d (%d rows), want 2", len(actors), len(rows))
	}
}
