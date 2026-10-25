package exposure

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
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

func TestReconcileWarnsOncePerDistinctError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	syn := &fakeSyncer{res: exposure.SyncResult{Errors: []string{"iptables is not installed here"}}}
	c := New(fakeStore{}, syn, logger)
	for i := 0; i < 3; i++ {
		if _, err := c.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if n := strings.Count(buf.String(), "sync reported errors"); n != 1 {
		t.Fatalf("logged %d times, want 1", n)
	}
}
