package api

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func saveServiceOnNode(t *testing.T, db *store.DB, name, nodeID string, domains []string) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "img:v1", Port: 80, Domains: domains}); err != nil {
		t.Fatalf("SaveDesiredService(%q) error = %v", name, err)
	}
	if nodeID == "" {
		return
	}
	if err := db.UpdateServiceNode(ctx, name, nodeID); err != nil {
		t.Fatalf("UpdateServiceNode(%q, %q) error = %v", name, nodeID, err)
	}
}

func TestDoctorCheckCrossNodeIngress_NothingPlacedRemotely(t *testing.T) {
	rt, db := newDoctorTestRouter(t)
	saveServiceOnNode(t, db, "web", "", []string{"web.example.com"})

	checks := rt.doctorCheckCrossNodeIngress(context.Background())
	if len(checks) != 0 {
		t.Fatalf("checks = %+v, want none: web is placed locally", checks)
	}
}

func TestDoctorCheckCrossNodeIngress_NoDomainsConfigured(t *testing.T) {
	rt, db := newDoctorTestRouter(t)
	saveServiceOnNode(t, db, "worker", "node-2", nil)

	checks := rt.doctorCheckCrossNodeIngress(context.Background())
	if len(checks) != 0 {
		t.Fatalf("checks = %+v, want none: worker has no domain to route", checks)
	}
}

func TestDoctorCheckCrossNodeIngress_RemotePlacementWithDomain_Warns(t *testing.T) {
	rt, db := newDoctorTestRouter(t)
	ctx := context.Background()
	if err := db.SaveNode(ctx, store.Node{ID: "node-2", Name: "worker-1", Address: "161.35.113.211", Status: store.NodeStatusOnline, Schedulable: true}); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}
	saveServiceOnNode(t, db, "static-test", "node-2", []string{"levelrail-test-2.levelrail.com"})

	checks := rt.doctorCheckCrossNodeIngress(ctx)
	if len(checks) != 1 {
		t.Fatalf("len(checks) = %d, want 1: %+v", len(checks), checks)
	}
	c := checks[0]
	if c.Status != doctorStatusWarn {
		t.Errorf("Status = %q, want %q", c.Status, doctorStatusWarn)
	}
	if c.Code != "cross_node_ingress:static-test" {
		t.Errorf("Code = %q, want cross_node_ingress:static-test", c.Code)
	}
	if c.Fix == "" {
		t.Error("Fix = \"\", want a concrete next step")
	}
	for _, want := range []string{"static-test", "worker-1", "levelrail-test-2.levelrail.com"} {
		if !strings.Contains(c.Message, want) {
			t.Errorf("Message = %q, want it to mention %q", c.Message, want)
		}
	}
}

func TestDoctorCheckCrossNodeIngress_LocalNodeIDMatches_NoWarning(t *testing.T) {
	rt, db := newDoctorTestRouter(t)
	rt.localNodeID = "node-1"
	// Explicit, non-empty NodeID that happens to equal this control
	// plane's own node ID: still local, must not warn (isLocalNode's own
	// "" or localNodeID convention).
	saveServiceOnNode(t, db, "web", "node-1", []string{"web.example.com"})

	checks := rt.doctorCheckCrossNodeIngress(context.Background())
	if len(checks) != 0 {
		t.Fatalf("checks = %+v, want none: node-1 is this control plane's own node", checks)
	}
}
