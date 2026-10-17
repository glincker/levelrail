package store

import (
	"context"
	"testing"
	"time"
)

func TestNodeSecurityEvents_CoalesceCapPruneAndSummary(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	ev := NodeSecurityEvent{NodeID: "n1", Kind: SecurityKindConnect, Outcome: SecurityOutcomeRejected, Reason: SecurityReasonExpired, SourceIP: "1.2.3.4"}
	pol := SecurityEventPolicy{Coalesce: true, Window: time.Minute, PerNodeCap: 3}

	for i := range 5 {
		if err := db.AddNodeSecurityEvent(ctx, ev, pol, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	evs, err := db.ListNodeSecurityEvents(ctx, "n1", NodeSecurityFilter{Limit: 10})
	if err != nil || len(evs) != 1 || evs[0].Count != 5 {
		t.Fatalf("coalesced events = %+v (%v), want one row with count 5", evs, err)
	}
	if err := db.AddNodeSecurityEvent(ctx, ev, pol, now.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if evs, _ = db.ListNodeSecurityEvents(ctx, "n1", NodeSecurityFilter{Limit: 10}); len(evs) != 2 {
		t.Fatalf("an event outside the window must start a new row, got %d", len(evs))
	}

	for i := range 10 {
		other := ev
		other.SourceIP = "9.9.9." + string(rune('0'+i))
		if err := db.AddNodeSecurityEvent(ctx, other, pol, now.Add(time.Hour+time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if evs, _ = db.ListNodeSecurityEvents(ctx, "n1", NodeSecurityFilter{Limit: 50}); len(evs) != 3 {
		t.Fatalf("per-node cap kept %d rows, want 3", len(evs))
	}

	if n, err := db.PruneNodeSecurityEvents(ctx, now.Add(2*time.Hour)); err != nil || n != 3 {
		t.Fatalf("prune removed %d (%v), want 3", n, err)
	}
	if err := db.AddNodeSecurityEvent(ctx, NodeSecurityEvent{NodeID: "n1", Kind: SecurityKindConnect, Outcome: SecurityOutcomeAccepted}, SecurityEventPolicy{}, now); err != nil {
		t.Fatal(err)
	}
	sum, err := db.SummarizeNodeConnections(ctx, "n1", now.Add(-time.Hour))
	if err != nil || sum.Accepted != 1 || sum.Rejected != 0 {
		t.Fatalf("summary = %+v (%v)", sum, err)
	}
}

func TestNodeTrustState_PinAndRenewalReportAreDeletedWithTheNode(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	if err := db.SetNodeCAPin(ctx, "n1", CAPinPinned, now); err != nil {
		t.Fatal(err)
	}
	next := now.Add(time.Hour)
	if err := db.SetNodeRenewalReport(ctx, NodeTrustState{NodeID: "n1", RenewNextAt: &next, RenewLastError: "down", RenewConsecutiveFails: 2}, now); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetNodeTrustState(ctx, "n1")
	if err != nil || got.CAPinState != CAPinPinned || got.RenewLastError != "down" || got.RenewConsecutiveFails != 2 || got.RenewNextAt == nil {
		t.Fatalf("trust state = %+v (%v): the report must not overwrite the pin", got, err)
	}
	if err := db.deleteNodeSecurityData(ctx, "n1"); err != nil {
		t.Fatal(err)
	}
	if got, _ = db.GetNodeTrustState(ctx, "n1"); got.CAPinState != "" {
		t.Fatalf("state survived node deletion: %+v", got)
	}
}

func TestListNodeRenewalStates_OnlyFailingNodes(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	_ = db.SetNodeCAPin(ctx, "pinned", CAPinPinned, now)
	if err := db.SetNodeRenewalReport(ctx, NodeTrustState{NodeID: "bad", RenewLastError: "down", RenewConsecutiveFails: 3}, now); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListNodeRenewalStates(ctx)
	if err != nil || len(got) != 1 || got["bad"].RenewLastError != "down" || got["bad"].RenewConsecutiveFails != 3 {
		t.Fatalf("states = %+v (%v)", got, err)
	}
}
