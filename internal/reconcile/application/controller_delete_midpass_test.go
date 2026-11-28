package application

import (
	"context"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

// goneAfterFirstStore serves the desired row once, then reports it deleted, like a delete landing mid-deploy.
type goneAfterFirstStore struct {
	*fakeStore
	reads int
}

func (g *goneAfterFirstStore) GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error) {
	g.reads++
	if g.reads > 1 {
		return nil, store.ErrServiceNotFound
	}
	return g.fakeStore.GetDesiredService(ctx, name)
}

func TestController_Reconcile_DeletedMidPass_RemovesContainersItCreated(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &goneAfterFirstStore{fakeStore: &fakeStore{svc: desired}}, rt)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1 (the pass did deploy)", rt.createCalls)
	}
	if names := rt.names(); len(names) != 0 {
		t.Errorf("containers after a mid-pass delete = %v, want none", names)
	}
}
