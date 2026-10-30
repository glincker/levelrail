package ingress

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

func seedRelease(rt *fakeRuntime, svc, image string, port int, created time.Time) {
	name := application.ContainerName(svc, image, "")
	rt.seedRunning(name, port)
	rt.containers[name].Image = image
	rt.containers[name].Created = created
}

func succeeded(image string) store.DeployAttempt {
	return store.DeployAttempt{Image: image, Status: store.DeployAttemptStatusSucceeded}
}

func failed(image string) store.DeployAttempt {
	return store.DeployAttempt{Image: image, Status: store.DeployAttemptStatusFailed}
}

func onlyUpstream(t *testing.T, applier *fakeApplier) string {
	t.Helper()
	rp := proxyHandler(t, applier)
	// A held previous release rides along as a standby, so only the primary counts.
	if len(rp.Upstreams) == 0 || len(rp.Upstreams) > 2 {
		t.Fatalf("upstreams = %+v, want a primary and at most one standby", rp.Upstreams)
	}
	return rp.Upstreams[0].Dial
}

// B4: a preview service listed before its primary must not take the host.
func TestController_PreviewServiceNeverClaimsPrimaryHost(t *testing.T) {
	primary := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	preview := store.DesiredService{Name: "web-dep209p", Image: "rb:dep_209", Port: 80, Domains: []string{"web.example.com"}}
	rt := newFakeRuntime()
	seedRelease(rt, "web", "img:v1", 34567, time.Now())
	seedRelease(rt, "web-dep209p", "rb:dep_209", 34999, time.Now())
	applier := &fakeApplier{}
	c := New(&fakeStore{services: []store.DesiredService{preview, primary}}, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := onlyUpstream(t, applier); got != "127.0.0.1:34567" {
		t.Errorf("dial = %s, want the primary 127.0.0.1:34567", got)
	}
}

// B4 half-succeeded: the primary has no ready backend of its own, the
// preview must still not take the host over from the serving release.
func TestController_PreviewServiceDoesNotTakeOverWhenPrimaryNotReady(t *testing.T) {
	primary := store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Domains: []string{"web.example.com"}}
	preview := store.DesiredService{Name: "web-dep209p", Image: "rb:dep_209", Port: 80, Domains: []string{"web.example.com"}}
	rt := newFakeRuntime()
	seedRelease(rt, "web", "img:v1", 34567, time.Now().Add(-time.Hour))
	seedRelease(rt, "web-dep209p", "rb:dep_209", 34999, time.Now())
	applier := &fakeApplier{}
	st := &fakeStore{services: []store.DesiredService{preview, primary}, notReadyServices: map[string]bool{"web": true}}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := onlyUpstream(t, applier); got != "127.0.0.1:34567" {
		t.Errorf("dial = %s, want the serving release 127.0.0.1:34567", got)
	}
}

// B5: a newer release that never passed readiness is not the fallback.
func TestController_DialPreviousRelease_PrefersLastReadyRelease(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v3", Port: 80, Domains: []string{"web.example.com"}}
	base := time.Now().Add(-time.Hour)
	rt := newFakeRuntime()
	seedRelease(rt, "web", "img:v3", 34563, base.Add(3*time.Minute))
	seedRelease(rt, "web", "img:v2", 34562, base.Add(2*time.Minute)) // created later, never ready
	seedRelease(rt, "web", "img:v1", 34561, base)
	applier := &fakeApplier{}
	st := &fakeStore{
		services:         []store.DesiredService{desired},
		notReadyServices: map[string]bool{"web": true},
		attempts:         map[string][]store.DeployAttempt{"web": {failed("img:v3"), failed("img:v2"), succeeded("img:v1")}},
	}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := onlyUpstream(t, applier); got != "127.0.0.1:34561" {
		t.Errorf("dial = %s, want last ready release 127.0.0.1:34561", got)
	}
}

// B6: during cutover only the held release is an upstream, not every old one.
func TestController_LoadBalancer_OnlyHeldReleaseServesDuringCutover(t *testing.T) {
	svc := lbService(1)
	svc.Image = "img:v2"
	heldName := application.ReplicaContainerName(svc.Name, "img:v1", "", 0)
	staleName := application.ReplicaContainerName(svc.Name, "img:v0", "", 0)
	base := time.Now().Add(-time.Hour)
	rt := &prefixRuntime{fakeRuntime: newFakeRuntime(), byPrefix: []docker.ContainerState{
		{Name: staleName, Image: "img:v0", Created: base.Add(time.Minute), Running: true, Ports: []docker.PortBinding{{ContainerPort: 80, HostPort: 31001}}},
		{Name: heldName, Image: "img:v1", Created: base, Running: true, Ports: []docker.PortBinding{{ContainerPort: 80, HostPort: 31000}}},
	}}
	applier := &fakeApplier{}
	st := &fakeStore{
		services: []store.DesiredService{svc},
		attempts: map[string][]store.DeployAttempt{"web": {succeeded("img:v1"), succeeded("img:v0")}},
	}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithLoadBalancers(fakeLBSource{configs: map[string]string{"web": `{}`}}, loadbalancer.NewRegistry()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := onlyUpstream(t, applier); got != "127.0.0.1:31000" {
		t.Errorf("upstream = %s, want only the held release 127.0.0.1:31000", got)
	}
}

// B7 and half-succeeded: the desired container is up but not publishing the
// stream port yet, so the stream stays on the serving release.
func TestController_StreamFallsBackToServingRelease(t *testing.T) {
	desired := store.DesiredService{Name: "pg", Image: "postgres:17", Port: 5432}
	rt := newFakeRuntime()
	newName := application.ContainerName("pg", "postgres:17", "")
	rt.containers[newName] = &docker.ContainerState{ID: newName, Name: newName, Running: true,
		Ports: []docker.PortBinding{{ContainerPort: 5432, HostPort: 40000}}}
	oldName := application.ContainerName("pg", "postgres:16", "")
	rt.containers[oldName] = &docker.ContainerState{ID: oldName, Name: oldName, Running: true,
		Ports: []docker.PortBinding{{ContainerPort: 5432, HostPort: 40001}, {ContainerPort: 5433, HostPort: 55433}}}
	siblingName := application.ContainerName("pg-dep1", "postgres:16", "")
	rt.containers[siblingName] = &docker.ContainerState{ID: siblingName, Name: siblingName, Running: true,
		Ports: []docker.PortBinding{{ContainerPort: 5433, HostPort: 55999}}, Created: time.Now()}
	st := &fakeStore{
		services: []store.DesiredService{desired},
		streams:  []store.AppStream{{ID: "s", ServiceName: "pg", ContainerPort: 5433, HostPort: 15432, Protocol: store.AppStreamProtocolTCP}},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	servers := applier.layer4Servers(t)
	if len(servers) != 1 {
		t.Fatalf("layer4 servers = %+v, want 1", servers)
	}
	for _, srv := range servers {
		h, ok := srv.Routes[0].Handle[0].(ingress.Layer4ProxyHandler)
		if !ok || h.Upstreams[0].Dial[0] != "127.0.0.1:55433" {
			t.Errorf("handler = %#v, want dial 127.0.0.1:55433 (serving release, not the sibling)", srv.Routes[0].Handle[0])
		}
	}
}

// B17/P1: after a rollback the Ready condition still belongs to the failed
// retry of the abandoned release. The rollback target is running and passed
// readiness before, so it keeps serving instead of the older held release.
func TestController_RollbackTargetKeepsServingWhileStaleReadyIsFalse(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Domains: []string{"web.example.com"}}
	base := time.Now().Add(-time.Hour)
	rt := newFakeRuntime()
	seedRelease(rt, "web", "img:v2", 34562, base.Add(time.Minute))
	seedRelease(rt, "web", "img:v1", 34561, base)
	applier := &fakeApplier{}
	st := &fakeStore{
		services:         []store.DesiredService{desired},
		notReadyServices: map[string]bool{"web": true},
		attempts: map[string][]store.DeployAttempt{"web": {
			{Image: "img:v2", Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}, failed("img:v3"), succeeded("img:v2"), succeeded("img:v1"),
		}},
	}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := onlyUpstream(t, applier); got != "127.0.0.1:34562" {
		t.Errorf("dial = %s, want the rollback target 127.0.0.1:34562", got)
	}
}

// Half-succeeded: a desired container that never passed readiness stays
// behind the serving release even though it is running.
func TestController_UnprovenTargetStaysBehindServingReleaseWhenNotReady(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Domains: []string{"web.example.com"}}
	base := time.Now().Add(-time.Hour)
	rt := newFakeRuntime()
	seedRelease(rt, "web", "img:v2", 34562, base.Add(time.Minute))
	seedRelease(rt, "web", "img:v1", 34561, base)
	applier := &fakeApplier{}
	st := &fakeStore{
		services:         []store.DesiredService{desired},
		notReadyServices: map[string]bool{"web": true},
		attempts:         map[string][]store.DeployAttempt{"web": {failed("img:v2"), succeeded("img:v1")}},
	}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := onlyUpstream(t, applier); got != "127.0.0.1:34561" {
		t.Errorf("dial = %s, want the serving release 127.0.0.1:34561", got)
	}
}

// A restart or same-tag redeploy creates a new container with a proven
// image; it must still wait for Ready.
func TestController_FreshContainerOfProvenImageWaitsForReady(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Domains: []string{"web.example.com"}}
	base := time.Now().Add(-time.Hour)
	rt := newFakeRuntime()
	seedRelease(rt, "web", "img:v2", 34562, time.Now().Add(time.Minute))
	seedRelease(rt, "web", "img:v1", 34561, base)
	applier := &fakeApplier{}
	st := &fakeStore{
		services:         []store.DesiredService{desired},
		notReadyServices: map[string]bool{"web": true},
		attempts: map[string][]store.DeployAttempt{"web": {
			{Image: "img:v2", Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}, succeeded("img:v2"), succeeded("img:v1"),
		}},
	}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := onlyUpstream(t, applier); got != "127.0.0.1:34561" {
		t.Errorf("dial = %s, want the serving release 127.0.0.1:34561", got)
	}
}

// Podman reports containers with docker.io/ qualified names while attempts keep the short ref.
func TestController_DialPreviousRelease_PodmanQualifiedImageNames(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "traefik/whoami:v3", Port: 80, Domains: []string{"web.example.com"}}
	base := time.Now().Add(-time.Hour)
	rt := newFakeRuntime()
	seedRelease(rt, "web", "traefik/whoami:v3", 34563, base.Add(3*time.Minute))
	seedRelease(rt, "web", "traefik/whoami:v2", 34562, base.Add(2*time.Minute))
	seedRelease(rt, "web", "traefik/whoami:v1", 34561, base)
	for _, tag := range []string{"v3", "v2", "v1"} {
		rt.containers[application.ContainerName("web", "traefik/whoami:"+tag, "")].Image = "docker.io/traefik/whoami:" + tag
	}
	applier := &fakeApplier{}
	st := &fakeStore{
		services:         []store.DesiredService{desired},
		notReadyServices: map[string]bool{"web": true},
		attempts:         map[string][]store.DeployAttempt{"web": {failed("traefik/whoami:v3"), failed("traefik/whoami:v2"), succeeded("traefik/whoami:v1")}},
	}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := onlyUpstream(t, applier); got != "127.0.0.1:34561" {
		t.Errorf("dial = %s, want last ready release 127.0.0.1:34561", got)
	}
	state := &docker.ContainerState{Image: "docker.io/traefik/whoami:v1", Created: base}
	st.attempts["web"] = []store.DeployAttempt{{Image: "traefik/whoami:v3", Status: store.DeployAttemptStatusFailed, StartedAt: time.Now()}, succeeded("traefik/whoami:v1")}
	if !c.passedReadiness(context.Background(), "web", state) {
		t.Error("passedReadiness must match a qualified image against the short attempt ref")
	}
}
