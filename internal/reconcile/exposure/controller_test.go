package exposure

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/exposure"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeStore struct {
	rows []store.ExposureRestriction
	err  error
}

func (f fakeStore) ListExposureRestrictions(context.Context) ([]store.ExposureRestriction, error) {
	return f.rows, f.err
}

type fakeSyncer struct {
	got []exposure.Restriction
	res exposure.SyncResult
}

func (f *fakeSyncer) Sync(_ context.Context, want []exposure.Restriction) exposure.SyncResult {
	f.got = want
	return f.res
}

func TestReconcileAssertsStoredRestrictions(t *testing.T) {
	syn := &fakeSyncer{res: exposure.SyncResult{Applied: 1}}
	c := New(fakeStore{rows: []store.ExposureRestriction{{Port: 8108, Protocol: "tcp", Allow: []string{"10.0.0.0/8"}}}}, syn, nil)
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(syn.got) != 1 || syn.got[0].Port != 8108 {
		t.Fatalf("synced = %+v", syn.got)
	}
	if res.Conditions[0].Status != reconcile.ConditionTrue {
		t.Fatalf("condition = %+v", res.Conditions[0])
	}
}

func TestReconcileReportsFailureAndUnreadableChain(t *testing.T) {
	failing := &fakeSyncer{res: exposure.SyncResult{Errors: []string{"8108/tcp: boom"}}}
	rows := []store.ExposureRestriction{{Port: 8108, Protocol: "tcp", Allow: []string{"10.0.0.0/8"}}}
	res, _ := New(fakeStore{rows: rows}, failing, nil).Reconcile(context.Background())
	if res.Conditions[0].Status != reconcile.ConditionFalse {
		t.Fatalf("condition = %+v", res.Conditions[0])
	}
	res, _ = New(fakeStore{}, failing, nil).Reconcile(context.Background())
	if res.Conditions[0].Status != reconcile.ConditionUnknown {
		t.Fatalf("no restrictions and unreadable chain must be informational: %+v", res.Conditions[0])
	}
}

func TestReconcileStoreError(t *testing.T) {
	_, err := New(fakeStore{err: errors.New("db down")}, &fakeSyncer{}, nil).Reconcile(context.Background())
	if err == nil {
		t.Fatal("want error")
	}
}
