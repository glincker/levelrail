package store

import (
	"context"
	"errors"
	"testing"
)

func TestNewDeployApprovalID(t *testing.T) {
	seen := make(map[string]bool)
	for range 20 {
		id, err := NewDeployApprovalID()
		if err != nil {
			t.Fatalf("NewDeployApprovalID() error = %v", err)
		}
		if id == "" {
			t.Fatal("NewDeployApprovalID() returned empty string")
		}
		if id[:len(deployApprovalIDPrefix)] != deployApprovalIDPrefix {
			t.Errorf("NewDeployApprovalID() = %q, want prefix %q", id, deployApprovalIDPrefix)
		}
		if seen[id] {
			t.Fatalf("NewDeployApprovalID() produced a duplicate: %q", id)
		}
		seen[id] = true
	}
}

func testDeployApproval(id string) DeployApproval {
	return DeployApproval{
		ID: id, ServiceName: "web", EnvironmentID: "env_prod",
		Action: DeployApprovalActionDeploy, Image: "levelrail/web:2",
		Status:          DeployApprovalStatusPending,
		RequestedByType: PrincipalTypeUser, RequestedBy: "user_requester", RequestedByName: "Requester",
		CreatedAt: "2026-09-22T00:00:00.000000000Z",
		ExpiresAt: "2026-09-23T00:00:00.000000000Z",
	}
}

func TestSaveAndGetDeployApproval(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := testDeployApproval("apr_test1")
	if err := db.SaveDeployApproval(ctx, want); err != nil {
		t.Fatalf("SaveDeployApproval() error = %v", err)
	}

	got, err := db.GetDeployApproval(ctx, "apr_test1")
	if err != nil {
		t.Fatalf("GetDeployApproval() error = %v", err)
	}
	if got != want {
		t.Errorf("GetDeployApproval() = %+v, want %+v", got, want)
	}

	if _, err := db.GetDeployApproval(ctx, "apr_missing"); !errors.Is(err, ErrDeployApprovalNotFound) {
		t.Errorf("GetDeployApproval(missing) error = %v, want ErrDeployApprovalNotFound", err)
	}
}

func TestListDeployApprovals(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	pendingWeb := testDeployApproval("apr_1")
	pendingWeb.ServiceName = "web"
	if err := db.SaveDeployApproval(ctx, pendingWeb); err != nil {
		t.Fatalf("save apr_1: %v", err)
	}

	pendingAPI := testDeployApproval("apr_2")
	pendingAPI.ServiceName = "api"
	if err := db.SaveDeployApproval(ctx, pendingAPI); err != nil {
		t.Fatalf("save apr_2: %v", err)
	}

	decidedWeb := testDeployApproval("apr_3")
	decidedWeb.ServiceName = "web"
	decidedWeb.Status = DeployApprovalStatusApproved
	if err := db.SaveDeployApproval(ctx, decidedWeb); err != nil {
		t.Fatalf("save apr_3: %v", err)
	}

	all, err := db.ListDeployApprovals(ctx, "", "")
	if err != nil {
		t.Fatalf("ListDeployApprovals(all): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("len(all) = %d, want 3", len(all))
	}

	pending, err := db.ListDeployApprovals(ctx, DeployApprovalStatusPending, "")
	if err != nil {
		t.Fatalf("ListDeployApprovals(pending): %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("len(pending) = %d, want 2", len(pending))
	}

	webOnly, err := db.ListDeployApprovals(ctx, "", "web")
	if err != nil {
		t.Fatalf("ListDeployApprovals(web): %v", err)
	}
	if len(webOnly) != 2 {
		t.Fatalf("len(webOnly) = %d, want 2", len(webOnly))
	}

	pendingWebOnly, err := db.ListDeployApprovals(ctx, DeployApprovalStatusPending, "web")
	if err != nil {
		t.Fatalf("ListDeployApprovals(pending, web): %v", err)
	}
	if len(pendingWebOnly) != 1 || pendingWebOnly[0].ID != "apr_1" {
		t.Fatalf("ListDeployApprovals(pending, web) = %+v, want just apr_1", pendingWebOnly)
	}
}

// TestDecideDeployApproval_OnlyFromPending is the state machine's core
// invariant: a decision only ever applies from status = 'pending', so a
// caller racing to decide an already-decided (or expired) row can never
// silently overwrite an earlier decision.
func TestDecideDeployApproval_OnlyFromPending(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	a := testDeployApproval("apr_1")
	if err := db.SaveDeployApproval(ctx, a); err != nil {
		t.Fatalf("save: %v", err)
	}

	ok, err := db.DecideDeployApproval(ctx, "apr_1", DeployApprovalStatusApproved, PrincipalTypeUser, "user_approver", "Approver", "", "2026-09-22T01:00:00.000000000Z")
	if err != nil {
		t.Fatalf("DecideDeployApproval() error = %v", err)
	}
	if !ok {
		t.Fatal("DecideDeployApproval() from pending = false, want true")
	}

	got, err := db.GetDeployApproval(ctx, "apr_1")
	if err != nil {
		t.Fatalf("GetDeployApproval: %v", err)
	}
	if got.Status != DeployApprovalStatusApproved {
		t.Errorf("Status = %q, want %q", got.Status, DeployApprovalStatusApproved)
	}
	if got.ApprovedBy != "user_approver" {
		t.Errorf("ApprovedBy = %q, want %q", got.ApprovedBy, "user_approver")
	}

	// A second decision attempt on an already-decided row must be a
	// no-op, not silently overwrite the first decision.
	ok2, err := db.DecideDeployApproval(ctx, "apr_1", DeployApprovalStatusRejected, PrincipalTypeUser, "user_other", "Other", "too late", "2026-09-22T02:00:00.000000000Z")
	if err != nil {
		t.Fatalf("DecideDeployApproval() (second) error = %v", err)
	}
	if ok2 {
		t.Fatal("DecideDeployApproval() on an already-decided row = true, want false")
	}

	got2, err := db.GetDeployApproval(ctx, "apr_1")
	if err != nil {
		t.Fatalf("GetDeployApproval: %v", err)
	}
	if got2.Status != DeployApprovalStatusApproved || got2.ApprovedBy != "user_approver" {
		t.Errorf("a no-op second decision must not change the row, got %+v", got2)
	}

	// Deciding a nonexistent row is also a no-op, not an error.
	ok3, err := db.DecideDeployApproval(ctx, "apr_missing", DeployApprovalStatusApproved, PrincipalTypeUser, "x", "X", "", "2026-09-22T03:00:00.000000000Z")
	if err != nil {
		t.Fatalf("DecideDeployApproval(missing) error = %v", err)
	}
	if ok3 {
		t.Fatal("DecideDeployApproval(missing) = true, want false")
	}
}
