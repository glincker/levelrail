package proxyroutes

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/reconcile"
)

type fakeSyncer struct {
	rep proxyroutes.Report
	err error
}

func (f fakeSyncer) Sync(context.Context) (proxyroutes.Report, error) { return f.rep, f.err }

func TestReconcile(t *testing.T) {
	tests := []struct {
		name       string
		syncer     fakeSyncer
		wantStatus reconcile.ConditionStatus
		wantReason string
		wantErr    bool
	}{
		{"disabled", fakeSyncer{}, reconcile.ConditionTrue, ReasonDisabled, false},
		{"converged", fakeSyncer{rep: proxyroutes.Report{Enabled: true, Written: []string{"a"}}}, reconcile.ConditionTrue, ReasonConverged, false},
		{"write failed", fakeSyncer{rep: proxyroutes.Report{Enabled: true, Failed: map[string]string{"a": "x"}}, err: errors.New("1 of 1")}, reconcile.ConditionFalse, ReasonFailed, true},
		{"store error", fakeSyncer{err: errors.New("db")}, reconcile.ConditionFalse, ReasonStore, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := New(tc.syncer)
			res, err := c.Reconcile(context.Background())
			if (err != nil) != tc.wantErr || len(res.Conditions) != 1 {
				t.Fatalf("res = %+v err = %v", res, err)
			}
			if got := res.Conditions[0]; got.Status != tc.wantStatus || got.Reason != tc.wantReason {
				t.Errorf("condition = %+v", got)
			}
			if c.Name() != Name {
				t.Errorf("Name() = %q", c.Name())
			}
		})
	}
}
