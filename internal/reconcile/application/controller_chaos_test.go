package application

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestController_Reconcile_DockerUnreachable_ReportsInspectFailedAndChangesNothing(t *testing.T) {
	strategies := []string{"", "recreate"}
	for _, strategy := range strategies {
		t.Run("strategy="+strategy, func(t *testing.T) {
			rt := newFakeRuntime(0)
			rt.inspectErr = errors.New("Cannot connect to the Docker daemon")
			desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Strategy: strategy}

			c := New("web", &fakeStore{svc: desired}, rt)
			result, err := c.Reconcile(context.Background())
			if err == nil {
				t.Fatal("Reconcile() error = nil, want the daemon failure surfaced")
			}
			cond := conditionOf(t, result)
			if cond.Status != reconcile.ConditionFalse || cond.Reason != "InspectFailed" {
				t.Errorf("condition = %+v, want Status=False Reason=InspectFailed", cond)
			}
			if rt.createCalls != 0 || rt.removeCalls != 0 || rt.stopCalls != 0 || rt.startCalls != 0 {
				t.Errorf("mutations with daemon down: create=%d remove=%d stop=%d start=%d, want all 0", rt.createCalls, rt.removeCalls, rt.stopCalls, rt.startCalls)
			}

			rt.inspectErr = nil
			result, err = c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("Reconcile() after daemon recovery error = %v", err)
			}
			if cond := conditionOf(t, result); cond.Reason != "Deployed" {
				t.Errorf("condition after recovery = %+v, want Reason=Deployed", cond)
			}
		})
	}
}

func TestController_Reconcile_Redeploy_TargetImageMissing_OldContainerSurvives(t *testing.T) {
	tests := []struct {
		name      string
		createErr string
	}{
		{"image garbage collected", "Error response from daemon: No such image: img:v2"},
		{"registry lost the tag", "Error response from daemon: manifest for img:v2 not found: manifest unknown"},
		{"pull denied", "Error response from daemon: pull access denied for img, repository does not exist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(0)
			oldTarget := ContainerName("web", "img:v1", "")
			rt.seed(oldTarget, true)
			rt.createErr = errors.New(tt.createErr)
			desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80}

			c := New("web", &fakeStore{svc: desired}, rt)
			result, err := c.Reconcile(context.Background())
			if err == nil {
				t.Fatal("Reconcile() error = nil, want the missing-image failure")
			}
			cond := conditionOf(t, result)
			if cond.Status != reconcile.ConditionFalse || cond.Reason != "CreateFailed" {
				t.Errorf("condition = %+v, want Status=False Reason=CreateFailed", cond)
			}
			if rt.removeCalls != 0 {
				t.Errorf("removeCalls = %d, want 0: a rollback to a missing image must not take down the serving container", rt.removeCalls)
			}
			names := rt.names()
			if len(names) != 1 || names[0] != oldTarget {
				t.Errorf("containers = %v, want only the old %q still present", names, oldTarget)
			}
		})
	}
}
