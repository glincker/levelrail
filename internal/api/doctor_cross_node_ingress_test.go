package api

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/meshpath"
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

type fakeMeshPaths struct{ path meshpath.Path }

func (f fakeMeshPaths) Path(context.Context, string) (meshpath.Path, error) { return f.path, nil }

func TestDoctorCheckCrossNodeIngress_MeshPath(t *testing.T) {
	tests := []struct {
		name     string
		path     meshpath.Path
		wantWarn bool
		wantWhy  string
	}{
		{name: "usable path clears the finding", path: meshpath.Path{Usable: true, Address: "10.181.0.2"}},
		{name: "unusable path warns with the reason", path: meshpath.Path{Reason: "no recent WireGuard handshake"}, wantWarn: true, wantWhy: "no recent WireGuard handshake"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newDoctorTestRouter(t)
			rt.SetMeshPaths(fakeMeshPaths{path: tt.path})
			saveServiceOnNode(t, db, "web", "node-2", []string{"web.example.com"})
			svc, err := db.GetDesiredService(context.Background(), "web")
			if err != nil {
				t.Fatal(err)
			}

			checks := rt.doctorCheckCrossNodeIngress(context.Background())
			cond := rt.crossNodeIngressAppCondition(context.Background(), *svc)
			if !tt.wantWarn {
				if len(checks) != 0 || cond != nil {
					t.Fatalf("checks = %+v, cond = %+v, want both cleared", checks, cond)
				}
				return
			}
			if len(checks) != 1 || !strings.Contains(checks[0].Message, tt.wantWhy) || checks[0].Fix == "" {
				t.Fatalf("checks = %+v, want one warning naming %q with a fix", checks, tt.wantWhy)
			}
			if cond == nil || cond.Reason != "NoMeshIngressPath" || !strings.Contains(cond.Message, tt.wantWhy) {
				t.Fatalf("cond = %+v, want NoMeshIngressPath naming %q", cond, tt.wantWhy)
			}
		})
	}
}

func TestDoctorCheckCrossNodeIngress_SslipFallbackHostIsChecked(t *testing.T) {
	rt, db := newDoctorTestRouter(t)
	rt.publicHost = "134.209.118.96"
	saveServiceOnNode(t, db, "nodes-web", "node-2", nil)

	checks := rt.doctorCheckCrossNodeIngress(context.Background())
	if len(checks) != 1 || !strings.Contains(checks[0].Message, "nodes-web.134-209-118-96.sslip.io") {
		t.Fatalf("checks = %+v, want the sslip.io host named", checks)
	}
}
