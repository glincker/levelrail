package ingress

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// layer4Servers returns the layer4 app's servers from the last applied
// Config, or nil if no layer4 app was built this pass. Mirrors
// fakeApplier.routes' own "read straight off lastCfg" shape for the
// layer4 app instead of the http app.
func (f *fakeApplier) layer4Servers(t *testing.T) map[string]*ingress.Layer4Server {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastCfg == nil || f.lastCfg.Apps.Layer4 == nil {
		return nil
	}
	return f.lastCfg.Apps.Layer4.Servers
}

// TestController_Reconcile_AppStream_RoutedToRunningContainer proves a
// stream whose owning service has a running container publishing its
// container port gets one dedicated layer4 server, proxying to the
// container's actual published host port for that specific port (not
// the service's own main HTTP port).
func TestController_Reconcile_AppStream_RoutedToRunningContainer(t *testing.T) {
	desired := store.DesiredService{Name: "pg", Image: "postgres:16", Port: 5432}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.containers[target] = &docker.ContainerState{
		ID: target, Name: target, Running: true,
		Ports: []docker.PortBinding{
			{ContainerPort: 5432, HostPort: 34567},
			// The stream's own extra published port: a Docker-assigned
			// ephemeral host port, deliberately different from the
			// stream's HostPort (15432) below, which Caddy's layer4
			// listener binds instead (see streamPortBindings's doc
			// comment in internal/reconcile/application).
			{ContainerPort: 5433, HostPort: 55433},
		},
	}

	st := &fakeStore{
		services: []store.DesiredService{desired},
		streams: []store.AppStream{
			{ID: "stream_a", ServiceName: "pg", ContainerPort: 5433, HostPort: 15432, Protocol: store.AppStreamProtocolTCP},
		},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	servers := applier.layer4Servers(t)
	if len(servers) != 1 {
		t.Fatalf("layer4 servers = %+v, want exactly one", servers)
	}
	for _, srv := range servers {
		if len(srv.Listen) != 1 || srv.Listen[0] != ":15432" {
			t.Errorf("listen = %v, want [:15432]", srv.Listen)
		}
		if len(srv.Routes) != 1 {
			t.Fatalf("routes = %+v, want exactly one", srv.Routes)
		}
		handler, ok := srv.Routes[0].Handle[0].(ingress.Layer4ProxyHandler)
		if !ok {
			t.Fatalf("handle[0] = %#v, want a Layer4ProxyHandler", srv.Routes[0].Handle[0])
		}
		if len(handler.Upstreams) != 1 || len(handler.Upstreams[0].Dial) != 1 || handler.Upstreams[0].Dial[0] != "127.0.0.1:55433" {
			t.Errorf("upstream dial = %+v, want 127.0.0.1:55433 (the container's own published host port for container port 5433, not the stream's own HostPort 15432)", handler.Upstreams)
		}
	}
}

// TestController_Reconcile_AppStream_NoRunningContainer_SkippedNotFailed
// proves a stream whose owning service has no running container yet is
// simply left out of this pass (no layer4 app at all, since it's the
// only configured stream), not a reconcile failure: mirrors
// dialForService's own "mid-deploy, pick it up later" shape for HTTP
// routes.
func TestController_Reconcile_AppStream_NoRunningContainer_SkippedNotFailed(t *testing.T) {
	desired := store.DesiredService{Name: "pg", Image: "postgres:16", Port: 5432}
	// No seeded container at all: InspectByName returns (nil, nil).

	rt := newFakeRuntime()
	st := &fakeStore{
		services: []store.DesiredService{desired},
		streams: []store.AppStream{
			{ID: "stream_a", ServiceName: "pg", ContainerPort: 5433, HostPort: 15432, Protocol: store.AppStreamProtocolTCP},
		},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Ready=True even with an unroutable stream", cond)
	}
	if servers := applier.layer4Servers(t); len(servers) != 0 {
		t.Errorf("layer4 servers = %+v, want none: the stream's container isn't running yet", servers)
	}
}

// TestController_Reconcile_AppStream_OrphanedService_SkippedNotFailed is
// this feature's half-success case (CLAUDE.md section 7): a stream row
// surviving its owning service's deletion must skip that one stream,
// not fail the whole reconcile pass.
func TestController_Reconcile_AppStream_OrphanedService_SkippedNotFailed(t *testing.T) {
	rt := newFakeRuntime()
	st := &fakeStore{
		services: nil, // the stream's service no longer exists
		streams: []store.AppStream{
			{ID: "stream_a", ServiceName: "deleted-service", ContainerPort: 5433, HostPort: 15432, Protocol: store.AppStreamProtocolTCP},
		},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Ready=True despite the orphaned stream", cond)
	}
	if servers := applier.layer4Servers(t); len(servers) != 0 {
		t.Errorf("layer4 servers = %+v, want none", servers)
	}
}

// TestController_Reconcile_AppStream_StoreError_FailsReconcile proves
// ListAllAppStreams erroring (unlike an individual stream being
// unroutable) does fail the whole pass: a genuine failure to list
// desired state, the same "StoreError" treatment every other
// ServiceStore method on this interface already gets.
func TestController_Reconcile_AppStream_StoreError_FailsReconcile(t *testing.T) {
	rt := newFakeRuntime()
	st := &fakeStore{streamsErr: errors.New("app_streams table locked")}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	_, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an error when ListAllAppStreams fails")
	}
}
