package ingress

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeStore is a hand-written fake, the same pattern
// internal/reconcile/application's tests already established: stateful
// enough for realistic multi-service scenarios, not just a call counter.
type fakeStore struct {
	services       []store.DesiredService
	err            error
	staticSites    []store.StaticSite
	staticSitesErr error
	settings       store.IngressSettings
	settingsErr    error
}

func (f *fakeStore) ListDesiredServices(_ context.Context) ([]store.DesiredService, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.services, nil
}

func (f *fakeStore) ListStaticSites(_ context.Context) ([]store.StaticSite, error) {
	if f.staticSitesErr != nil {
		return nil, f.staticSitesErr
	}
	return f.staticSites, nil
}

// GetIngressSettings returns the zero value (ACME disabled, no primary
// domain) unless a test explicitly sets f.settings or f.settingsErr:
// this matches every real, never-configured control plane's actual
// state (migrations/0023's own seeded row), so existing tests written
// before this method existed keep exercising the exact same
// internal-issuer-only, no-dashboard-route path they always have.
func (f *fakeStore) GetIngressSettings(_ context.Context) (store.IngressSettings, error) {
	if f.settingsErr != nil {
		return store.IngressSettings{}, f.settingsErr
	}
	return f.settings, nil
}

// fakeRuntime implements docker.Runtime with an in-memory container set,
// mirroring application's controller_test.go fakeRuntime. This
// controller only ever calls InspectByName, but the field type is the
// full docker.Runtime interface (matching application.Controller's own
// choice), so the fake must satisfy every method.
type fakeRuntime struct {
	mu         sync.Mutex
	containers map[string]*docker.ContainerState
	inspectErr map[string]error
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{
		containers: map[string]*docker.ContainerState{},
		inspectErr: map[string]error{},
	}
}

func (f *fakeRuntime) seedRunning(name string, hostPort int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.containers[name] = &docker.ContainerState{
		ID: name, Name: name, Running: true,
		Ports: []docker.PortBinding{{ContainerPort: 80, HostPort: hostPort}},
	}
}

func (f *fakeRuntime) seedRunningNoPorts(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.containers[name] = &docker.ContainerState{ID: name, Name: name, Running: true}
}

func (f *fakeRuntime) seedStopped(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.containers[name] = &docker.ContainerState{ID: name, Name: name, Running: false}
}

func (f *fakeRuntime) seedInspectErr(name string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspectErr[name] = err
}

func (f *fakeRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.inspectErr[name]; ok {
		return nil, err
	}
	cs, ok := f.containers[name]
	if !ok {
		return nil, nil
	}
	cp := *cs
	return &cp, nil
}

func (f *fakeRuntime) Create(_ context.Context, _ docker.ContainerSpec) (string, error) {
	return "", nil
}
func (f *fakeRuntime) Start(_ context.Context, _ string) error { return nil }
func (f *fakeRuntime) Events(_ context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}
func (f *fakeRuntime) ListImages(_ context.Context, _ string) ([]docker.ImageInfo, error) {
	return nil, nil
}
func (f *fakeRuntime) ListByPrefix(_ context.Context, _ string) ([]docker.ContainerState, error) {
	return nil, nil
}
func (f *fakeRuntime) Stop(_ context.Context, _ string, _ time.Duration) error { return nil }
func (f *fakeRuntime) Remove(_ context.Context, _ string, _ bool) error        { return nil }
func (f *fakeRuntime) EnsureVolume(_ context.Context, _ string) error          { return nil }

// Exec is unused by this package's tests: the ingress controller never
// runs a command inside a container, only manages container lifecycle
// and reads observed state. Stubbed to satisfy docker.Runtime.
func (f *fakeRuntime) Exec(_ context.Context, _ string, _ []string) (io.ReadCloser, error) {
	return nil, errors.New("fakeRuntime: Exec not implemented")
}

// ExecWithInput is unused for the same reason Exec above is. Stubbed to
// satisfy docker.Runtime.
func (f *fakeRuntime) ExecWithInput(_ context.Context, _ string, _ []string, _ io.Reader) (io.ReadCloser, error) {
	return nil, errors.New("fakeRuntime: ExecWithInput not implemented")
}

// fakeApplier is a hand-written fake for internal/ingress.Driver: it
// records the last Config it was handed so tests can assert on exactly
// what would have been sent to Caddy, without starting a real instance.
type fakeApplier struct {
	mu       sync.Mutex
	applyErr error
	lastCfg  *ingress.Config
	calls    int
}

func (f *fakeApplier) Apply(_ context.Context, cfg *ingress.Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastCfg = cfg
	return f.applyErr
}

// routes returns the routes from the last applied Config's default-named
// server. Every test in this file uses New without WithServerName, so
// defaultServerName is always the right key; a parameter here would be
// unparam-flagged dead flexibility.
func (f *fakeApplier) routes(t *testing.T) []ingress.Route {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastCfg == nil || f.lastCfg.Apps.HTTP == nil {
		return nil
	}
	server, ok := f.lastCfg.Apps.HTTP.Servers[defaultServerName]
	if !ok {
		return nil
	}
	return server.Routes
}

func conditionOf(t *testing.T, result reconcile.Result) reconcile.Condition {
	t.Helper()
	if len(result.Conditions) == 0 {
		t.Fatal("expected at least one condition, got none")
	}
	return result.Conditions[0]
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

func TestController_Reconcile_NoServicesWithDomains(t *testing.T) {
	st := &fakeStore{services: []store.DesiredService{
		{Name: "worker", Image: "img:v1", Port: 0}, // no domains at all
	}}
	rt := newFakeRuntime()
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil (empty config applies cleanly)", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Routed0Services" {
		t.Errorf("condition = %+v, want Status=True Reason=Routed0Services", cond)
	}
	if applier.calls != 1 {
		t.Fatalf("Apply calls = %d, want 1", applier.calls)
	}
	if routes := applier.routes(t); len(routes) != 0 {
		t.Errorf("applied routes = %v, want none", routes)
	}
}

func TestController_Reconcile_OneServiceRunning_RouteAppears(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Routed1Services" {
		t.Errorf("condition = %+v, want Status=True Reason=Routed1Services", cond)
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("applied routes = %v, want exactly 1", routes)
	}
	if len(routes[0].Match) != 1 || len(routes[0].Match[0].Host) != 1 || routes[0].Match[0].Host[0] != "web.example.com" {
		t.Errorf("route match = %+v, want host web.example.com", routes[0].Match)
	}
	handler, ok := routes[0].Handle[0].(ingress.ReverseProxyHandler)
	if !ok {
		t.Fatalf("route handler is %T, want ingress.ReverseProxyHandler", routes[0].Handle[0])
	}
	if len(handler.Upstreams) != 1 || handler.Upstreams[0].Dial != "127.0.0.1:34567" {
		t.Errorf("upstream = %+v, want dial 127.0.0.1:34567", handler.Upstreams)
	}
}

func TestController_Reconcile_ContainerNotRunning_SkippedNotFailed(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedStopped(target) // container exists but crashed / not started

	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil (a service mid-deploy is not a reconcile failure)", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Routed0Services" {
		t.Errorf("condition = %+v, want Status=True Reason=Routed0Services", cond)
	}
	if routes := applier.routes(t); len(routes) != 0 {
		t.Errorf("applied routes = %v, want none (container not running yet)", routes)
	}
}

func TestController_Reconcile_ContainerMissing_SkippedNotFailed(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	// No container seeded at all: InspectByName returns (nil, nil).

	rt := newFakeRuntime()
	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Routed0Services" {
		t.Errorf("condition = %+v, want Status=True Reason=Routed0Services", cond)
	}
}

func TestController_Reconcile_RunningNoPorts_SkippedNotFailed(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunningNoPorts(target)

	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed0Services" {
		t.Errorf("condition.Reason = %q, want Routed0Services", cond.Reason)
	}
}

// build.type: static (static sites served by the embedded ingress
// directly, no container) coverage for Controller lives in
// controller_static_test.go, not here, purely to keep this file's line
// count down; it shares every fake and helper defined in this file.

func TestController_Reconcile_InspectError_SkippedNotFailed(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedInspectErr(target, errors.New("docker socket hiccup"))

	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil: a single inspect failure must not fail the whole ingress reconcile", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed0Services" {
		t.Errorf("condition.Reason = %q, want Routed0Services", cond.Reason)
	}
}

func TestController_Reconcile_TwoServicesDistinctDomains_BothRouted(t *testing.T) {
	web := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	api := store.DesiredService{Name: "api", Image: "img:v2", Port: 8080, Domains: []string{"api.example.com"}}

	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(web.Name, web.Image, ""), 11111)
	rt.seedRunning(application.ContainerName(api.Name, api.Image, ""), 22222)

	st := &fakeStore{services: []store.DesiredService{web, api}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Routed2Services" {
		t.Errorf("condition = %+v, want Status=True Reason=Routed2Services", cond)
	}

	routes := applier.routes(t)
	if len(routes) != 2 {
		t.Fatalf("applied routes = %v, want exactly 2", routes)
	}
	gotHosts := map[string]bool{}
	for _, r := range routes {
		gotHosts[r.Match[0].Host[0]] = true
	}
	if !gotHosts["web.example.com"] || !gotHosts["api.example.com"] {
		t.Errorf("routed hosts = %v, want both web.example.com and api.example.com", gotHosts)
	}
}

func TestController_Reconcile_MixedRoutableAndNot(t *testing.T) {
	web := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	stillDeploying := store.DesiredService{Name: "api", Image: "img:v2", Port: 8080, Domains: []string{"api.example.com"}}
	noDomains := store.DesiredService{Name: "worker", Image: "img:v3", Port: 0}

	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(web.Name, web.Image, ""), 11111)
	// api's container deliberately not seeded: mid-deploy.

	st := &fakeStore{services: []store.DesiredService{web, stillDeploying, noDomains}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed1Services" {
		t.Errorf("condition.Reason = %q, want Routed1Services", cond.Reason)
	}
}

func TestController_Reconcile_StoreError(t *testing.T) {
	st := &fakeStore{err: errors.New("db unreachable")}
	rt := newFakeRuntime()
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "StoreError" {
		t.Errorf("condition = %+v, want Status=False Reason=StoreError", cond)
	}
	if applier.calls != 0 {
		t.Errorf("Apply calls = %d, want 0: never reached with no desired state", applier.calls)
	}
}

func TestController_Reconcile_ApplyFailed(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{applyErr: errors.New("caddy rejected config")}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the Caddy apply failure to surface as a real error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "ApplyFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=ApplyFailed", cond)
	}
}

func TestController_Name(t *testing.T) {
	c := New(&fakeStore{}, newFakeRuntime(), &fakeApplier{})
	if got, want := c.Name(), "ingress"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestOptions(t *testing.T) {
	custom := slog.Default()
	c := New(&fakeStore{}, newFakeRuntime(), &fakeApplier{},
		WithServerName("custom-server"),
		WithListenAddr(":8443"),
		WithAdminListen("localhost:9999"),
		WithStorageDir("/data/caddy"),
		WithLogger(custom),
	)
	if c.serverName != "custom-server" {
		t.Errorf("serverName = %q, want custom-server", c.serverName)
	}
	if c.listenAddr != ":8443" {
		t.Errorf("listenAddr = %q, want :8443", c.listenAddr)
	}
	if c.adminListen != "localhost:9999" {
		t.Errorf("adminListen = %q, want localhost:9999", c.adminListen)
	}
	if c.storageDir != "/data/caddy" {
		t.Errorf("storageDir = %q, want /data/caddy", c.storageDir)
	}
	if c.logger != custom {
		t.Error("WithLogger did not override the default logger")
	}
}

func TestNew_Defaults(t *testing.T) {
	c := New(&fakeStore{}, newFakeRuntime(), &fakeApplier{})
	if c.serverName != defaultServerName {
		t.Errorf("serverName = %q, want default %q", c.serverName, defaultServerName)
	}
	if c.listenAddr != defaultListenAddr {
		t.Errorf("listenAddr = %q, want default %q", c.listenAddr, defaultListenAddr)
	}
	if c.adminListen != defaultAdminListen {
		t.Errorf("adminListen = %q, want default %q", c.adminListen, defaultAdminListen)
	}
}
