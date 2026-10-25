package application

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
)

type fakeInner struct{ res reconcile.Result }

func (f fakeInner) Name() string { return "app/web" }
func (f fakeInner) Reconcile(context.Context) (reconcile.Result, error) {
	return f.res, nil
}

type fakePending struct{ calls []string }

func (f *fakePending) ResolvePendingDeployAttempts(_ context.Context, name string, _ time.Time) (int, error) {
	f.calls = append(f.calls, name)
	return 1, nil
}

func TestPendingResolverSettlesOnReadyOrSuspendedOnly(t *testing.T) {
	cases := []struct {
		name  string
		cond  reconcile.Condition
		wantN int
	}{
		{"ready", reconcile.Condition{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionTrue, Reason: "AlreadyRunning"}, 1},
		{"suspended", reconcile.Condition{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionUnknown, Reason: "Suspended"}, 1},
		{"node offline", reconcile.Condition{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionFalse, Reason: ReasonNodeOffline}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &fakePending{}
			c := WithPendingAttemptResolution(fakeInner{reconcile.Result{Conditions: []reconcile.Condition{tc.cond}}}, "web", st)
			if _, err := c.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(st.calls) != tc.wantN {
				t.Fatalf("calls = %v, want %d", st.calls, tc.wantN)
			}
		})
	}
}
