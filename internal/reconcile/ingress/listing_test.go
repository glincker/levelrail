package ingress

import (
	"context"
	"fmt"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

type listCountRuntime struct {
	*fakeRuntime
	lists int
}

func (r *listCountRuntime) ListByPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error) {
	r.lists++
	return r.fakeRuntime.ListByPrefix(ctx, prefix)
}

func TestController_PreviousReleaseLookups_ShareOneListingPerReconcile(t *testing.T) {
	const apps = 12
	rt := &listCountRuntime{fakeRuntime: newFakeRuntime()}
	st := &fakeStore{notReadyServices: map[string]bool{}}
	for i := 0; i < apps; i++ {
		name := fmt.Sprintf("web%02d", i)
		desired := store.DesiredService{Name: name, Image: "img:v2", Port: 80, Domains: []string{name + ".example.com"}}
		rt.seedRunning(application.ContainerName(name, desired.Image, ""), 30000+i)
		rt.seedRunning(application.ContainerName(name, "img:v1", ""), 31000+i)
		st.services = append(st.services, desired)
		st.notReadyServices[name] = true
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.lists != 1 {
		t.Errorf("ListByPrefix calls for %d services = %d, want 1 shared listing", apps, rt.lists)
	}

	routes := applier.routes(t)
	if len(routes) != apps {
		t.Fatalf("routes = %d, want %d", len(routes), apps)
	}
	for _, r := range routes {
		h, ok := r.Handle[0].(ingress.ReverseProxyHandler)
		if !ok || len(h.Upstreams) != 1 {
			t.Fatalf("route %+v has no single upstream", r)
		}
		if d := h.Upstreams[0].Dial; d < "127.0.0.1:31000" {
			t.Errorf("upstream %s is not a previous-release port: the unready new release must not take traffic", d)
		}
	}

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if rt.lists != 2 {
		t.Errorf("after a second pass ListByPrefix calls = %d, want 2: a listing must never outlive its pass", rt.lists)
	}
}

func TestController_ListLocal_OutsideReconcileGoesToTheRuntime(t *testing.T) {
	rt := &listCountRuntime{fakeRuntime: newFakeRuntime()}
	rt.seedRunning("web-abc", 30000)
	c := New(&fakeStore{}, rt, &fakeApplier{}, WithLogger(discardLogger()))

	for i := 0; i < 3; i++ {
		got, err := c.listLocal(context.Background(), "web-")
		if err != nil || len(got) != 1 {
			t.Fatalf("listLocal = %v, %v", got, err)
		}
	}
	if rt.lists != 3 {
		t.Errorf("direct calls = %d, want 3: nothing is cached outside a Reconcile", rt.lists)
	}
}

type batchCountRuntime struct {
	*fakeRuntime
	inspects, batches int
}

func (r *batchCountRuntime) InspectByName(ctx context.Context, name string) (*docker.ContainerState, error) {
	r.inspects++
	return r.fakeRuntime.InspectByName(ctx, name)
}

func (r *batchCountRuntime) InspectManyByName(ctx context.Context, names []string) (map[string]*docker.ContainerState, error) {
	r.batches++
	out := make(map[string]*docker.ContainerState, len(names))
	for _, n := range names {
		cs, _ := r.fakeRuntime.InspectByName(ctx, n)
		out[n] = cs
	}
	return out, nil
}

func TestController_LocalDials_ComeFromOneBatchedRead(t *testing.T) {
	const apps = 12
	rt := &batchCountRuntime{fakeRuntime: newFakeRuntime()}
	st := &fakeStore{}
	for i := 0; i < apps; i++ {
		name := fmt.Sprintf("web%02d", i)
		desired := store.DesiredService{Name: name, Image: "img:v1", Port: 80, Domains: []string{name + ".example.com"}}
		if i%2 == 0 {
			rt.seedRunning(application.ContainerName(name, desired.Image, ""), 30000+i)
		}
		st.services = append(st.services, desired)
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.batches != 1 || rt.inspects != 0 {
		t.Errorf("batched reads = %d, per-service inspects = %d, want 1 and 0", rt.batches, rt.inspects)
	}
	if routes := applier.routes(t); len(routes) != apps/2 {
		t.Errorf("routes = %d, want %d: only the running services have a backend", len(routes), apps/2)
	}
}
