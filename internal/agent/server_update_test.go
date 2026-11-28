package agent

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/glincker/levelrail/internal/agent/agentpb"
	"github.com/glincker/levelrail/internal/nodeinsight"
	"github.com/glincker/levelrail/internal/store"
)

func TestUpdateCheckIsScopedToTheCallingNode(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db)
	a := enrollNode(t, db, ts, "node-a")
	b := enrollNode(t, db, ts, "node-b")
	ts.srv.SetUpdateDecider(func(_ context.Context, n *store.Node) nodeinsight.UpdateDecision {
		return nodeinsight.UpdateDecision{Action: nodeinsight.UpdateInstall, Target: "v9.9.9", Reason: n.ID, AutoApply: true}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, id := range []*Identity{a, b} {
		d, err := UpdateCheckAt(ctx, ts.addr, id)
		if err != nil {
			t.Fatal(err)
		}
		// The request has no node id field, so the answer can only be the caller's own.
		if d.Reason != id.NodeID || d.Target != "v9.9.9" || !d.AutoApply {
			t.Fatalf("node %s got %+v", id.NodeID, d)
		}
	}
	if a.NodeID == b.NodeID {
		t.Fatal("nodes must differ")
	}
}

func TestUpdateCheckNeedsDeciderAndCertificate(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db)
	id := enrollNode(t, db, ts, "node-c")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := dialAs(t, ts.addr, id).UpdateCheck(ctx, &agentpb.UpdateCheckRequest{})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("err = %v, want Unavailable", err)
	}
	if _, err := ts.srv.UpdateCheck(ctx, &agentpb.UpdateCheckRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no peer: err = %v, want Unauthenticated", err)
	}
}

func TestUpdateCheckIsCachedPerNodeForAShortTime(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db)
	id := enrollNode(t, db, ts, "node-d")
	calls := 0
	ts.srv.SetUpdateDecider(func(context.Context, *store.Node) nodeinsight.UpdateDecision {
		calls++
		return nodeinsight.UpdateDecision{Action: nodeinsight.UpdateNone}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for i := 0; i < 3; i++ {
		if _, err := UpdateCheckAt(ctx, ts.addr, id); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("decider ran %d times, want 1", calls)
	}
}
