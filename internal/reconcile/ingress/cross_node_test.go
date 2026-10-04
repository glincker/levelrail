package ingress

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCrossNodeIngressCondition(t *testing.T) {
	isLocal := func(nodeID string) bool { return nodeID == "" || nodeID == "node-1" }

	tests := []struct {
		name     string
		services []store.DesiredService
		wantNil  bool
	}{
		{
			name:    "no services",
			wantNil: true,
		},
		{
			name: "local node, no domains",
			services: []store.DesiredService{
				{Name: "worker", NodeID: ""},
			},
			wantNil: true,
		},
		{
			name: "local node, with domains",
			services: []store.DesiredService{
				{Name: "web", NodeID: "node-1", Domains: []string{"web.example.com"}},
			},
			wantNil: true,
		},
		{
			name: "remote node, no domains configured",
			services: []store.DesiredService{
				{Name: "worker", NodeID: "node-2"},
			},
			wantNil: true,
		},
		{
			name: "remote node, with a domain: unreachable",
			services: []store.DesiredService{
				{Name: "static-test", NodeID: "node-2", Domains: []string{"levelrail-test-2.levelrail.com"}},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crossNodeIngressCondition(tt.services, isLocal)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("crossNodeIngressCondition() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("crossNodeIngressCondition() = nil, want a condition")
			}
			if got.Type != "CrossNodeIngress" || got.Status != reconcile.ConditionFalse || got.Reason != "NoMeshIngressPath" {
				t.Errorf("condition = %+v, want Type=CrossNodeIngress Status=False Reason=NoMeshIngressPath", got)
			}
			if got.Message == "" {
				t.Error("condition.Message is empty, want the affected service/domain named")
			}
		})
	}
}

// TestController_Reconcile_CrossNodeService_SurfacesCondition proves the
// condition actually reaches Reconcile's Result, not just the pure
// helper: a service placed on a node other than WithLocalNodeID's value,
// with a domain configured, must never look identical to the ordinary
// "mid-deploy, no container yet" skip path (see
// TestController_Reconcile_ContainerMissing_SkippedNotFailed), the exact
// silent-failure gap this condition exists to close.
func TestController_Reconcile_CrossNodeService_SurfacesCondition(t *testing.T) {
	desired := store.DesiredService{Name: "static-test", Image: "img:v1", Port: 80, NodeID: "node-2", Domains: []string{"levelrail-test-2.levelrail.com"}}
	// No container seeded locally: it genuinely only exists on node-2,
	// which this fakeRuntime (the control plane's own local runtime) can
	// never see.
	rt := newFakeRuntime()
	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithLocalNodeID("node-1"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}

	ready := conditionOf(t, result)
	if ready.Reason != "Routed0Services" {
		t.Errorf("Ready condition = %+v, want Reason=Routed0Services", ready)
	}

	var crossNode *reconcile.Condition
	for i := range result.Conditions {
		if result.Conditions[i].Type == "CrossNodeIngress" {
			crossNode = &result.Conditions[i]
		}
	}
	if crossNode == nil {
		t.Fatal("result.Conditions has no CrossNodeIngress condition, want one naming static-test's unreachable domain")
	}
	if crossNode.Status != reconcile.ConditionFalse {
		t.Errorf("CrossNodeIngress.Status = %q, want False", crossNode.Status)
	}
}

// TestController_Reconcile_LocalService_NoCrossNodeCondition is the
// control: WithLocalNodeID set, but every service with domains is placed
// locally, so the condition must stay absent, same as before
// WithLocalNodeID existed.
func TestController_Reconcile_LocalService_NoCrossNodeCondition(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(desired.Name, desired.Image, ""), 11111)
	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithLocalNodeID("node-1"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	for _, cond := range result.Conditions {
		if cond.Type == "CrossNodeIngress" {
			t.Fatalf("result.Conditions has a CrossNodeIngress condition, want none: %+v", cond)
		}
	}
}
