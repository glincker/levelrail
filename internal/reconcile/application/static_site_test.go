package application

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeStaticStore struct {
	*fakeStore
	site *store.StaticSite
	err  error
}

func (f *fakeStaticStore) GetStaticSite(context.Context, string) (*store.StaticSite, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.site == nil {
		return nil, store.ErrServiceNotFound
	}
	return f.site, nil
}

func TestController_Reconcile_PendingImage_StaticSite(t *testing.T) {
	tests := []struct {
		name       string
		site       *store.StaticSite
		err        error
		wantStatus reconcile.ConditionStatus
		wantReason string
		wantErr    bool
	}{
		{"published site is ready", &store.StaticSite{Name: "web"}, nil, reconcile.ConditionTrue, ReasonStaticSitePublished, false},
		{"no site awaits first build", nil, nil, reconcile.ConditionUnknown, "AwaitingFirstBuild", false},
		{"lookup failure is not ready", nil, errors.New("db locked"), reconcile.ConditionFalse, "StoreError", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := &store.DesiredService{Name: "web", Image: "local/web:pending", Port: 3000}
			st := &fakeStaticStore{fakeStore: &fakeStore{svc: desired}, site: tt.site, err: tt.err}
			rt := newFakeRuntime(0)
			res, err := New("web", st, rt).Reconcile(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			cond := conditionOf(t, res)
			if cond.Status != tt.wantStatus || cond.Reason != tt.wantReason {
				t.Errorf("condition = %+v, want %s/%s", cond, tt.wantStatus, tt.wantReason)
			}
			if names := rt.names(); len(names) != 0 {
				t.Errorf("containers = %v, a static site must never start one", names)
			}
		})
	}
}
