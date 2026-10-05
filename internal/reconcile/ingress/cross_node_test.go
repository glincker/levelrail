package ingress

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCrossNodeIngressCondition(t *testing.T) {
	if got := crossNodeIngressCondition(nil); got != nil {
		t.Fatalf("crossNodeIngressCondition(nil) = %+v, want nil", got)
	}
	got := crossNodeIngressCondition([]remoteBlock{{Service: "static-test", NodeID: "node-2", Hosts: []string{"a.example.com"}, Why: "no handshake"}})
	if got == nil {
		t.Fatal("crossNodeIngressCondition() = nil, want a condition")
	}
	if got.Type != "CrossNodeIngress" || got.Status != reconcile.ConditionFalse || got.Reason != "NoMeshIngressPath" {
		t.Errorf("condition = %+v, want Type=CrossNodeIngress Status=False Reason=NoMeshIngressPath", got)
	}
	for _, want := range []string{"static-test", "node-2", "a.example.com", "no handshake"} {
		if !strings.Contains(got.Message, want) {
			t.Errorf("Message = %q, want it to mention %q", got.Message, want)
		}
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
