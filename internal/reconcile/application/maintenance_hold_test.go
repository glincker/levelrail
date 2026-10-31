package application

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// A restore stops the mail container between swap steps; a reconcile pass
// in that window must not start it while the hold is set, even when suspended.
func TestController_Reconcile_MaintenanceHold_LeavesStoppedContainerAlone(t *testing.T) {
	for _, suspended := range []bool{false, true} {
		rt := newFakeRuntime(0)
		desired := &store.DesiredService{Name: "mail", Image: "mox:v1", Port: 80, MaintenanceHold: "mail-restore:20261030T120000Z", Suspended: suspended}
		target := ContainerName("mail", desired.Image, "")
		rt.seed(target, false)

		c := New("mail", &fakeStore{svc: desired}, rt)
		for pass := 0; pass < 3; pass++ {
			result, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if cond := conditionOf(t, result); cond.Status != reconcile.ConditionUnknown || cond.Reason != ReasonMaintenanceHold {
				t.Fatalf("condition = %+v, want Unknown %s", cond, ReasonMaintenanceHold)
			}
		}
		if rt.startCalls != 0 || rt.createCalls != 0 || rt.removeCalls != 0 || rt.stopCalls != 0 {
			t.Fatalf("held service touched: start=%d create=%d remove=%d stop=%d", rt.startCalls, rt.createCalls, rt.removeCalls, rt.stopCalls)
		}
		if cs := rt.containers[target]; cs == nil || cs.Running {
			t.Fatal("held container must stay present and stopped")
		}
	}

	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "mail", Image: "mox:v1", Port: 80}
	target := ContainerName("mail", desired.Image, "")
	rt.seed(target, false)
	if _, err := New("mail", &fakeStore{svc: desired}, rt).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.startCalls == 0 {
		t.Fatal("without a hold the reconciler starts the stopped container, which is what the hold guards against")
	}
}
