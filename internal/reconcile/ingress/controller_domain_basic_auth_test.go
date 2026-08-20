package ingress

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakePasswordResolver is a hand-written fake for
// DomainBasicAuthPasswordResolver, mirroring fakeDNSTokenResolver's own
// "record what was asked for, return a scripted result" shape.
type fakePasswordResolver struct {
	passwords   map[string]string // keyed by serviceName (store.DomainBasicAuthSecretsKey(domain))
	err         error
	resolveArgs [][2]string // every (serviceName, envKey) pair Resolve was called with, in order
}

func (f *fakePasswordResolver) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	f.resolveArgs = append(f.resolveArgs, [2]string{serviceName, envKey})
	if f.err != nil {
		return "", f.err
	}
	return f.passwords[serviceName], nil
}

// TestController_Reconcile_DomainBasicAuth_NoResolverConfigured_RegressionUnchanged
// is this feature's own regression test: a domain_basic_auth row exists
// but WithDomainBasicAuthSecrets was never called, the same
// "feature wired at the store layer but not enforced" state a control
// plane started without a master key is in. The domain must not be
// routed unprotected: it's simply left out of this pass, same as a
// service with no running backend.
func TestController_Reconcile_DomainBasicAuth_NoResolverConfigured_RegressionUnchanged(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services:  []store.DesiredService{desired},
		basicAuth: []store.DomainBasicAuth{{Domain: "web.example.com", Username: "operator"}},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger())) // no WithDomainBasicAuthSecrets

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed0Services" {
		t.Errorf("condition reason = %q, want Routed0Services: an unresolvable protected domain must not be routed at all", cond.Reason)
	}
	if routes := applier.routes(t); len(routes) != 0 {
		t.Errorf("applied routes = %+v, want none", routes)
	}
}

// TestController_Reconcile_DomainBasicAuth_ProtectedDomain_GetsAuthHandler
// proves a domain_basic_auth row plus a working resolver produces a
// route whose first handler is a bcrypt-checkable BasicAuthHandler
// ahead of the reverse_proxy handler.
func TestController_Reconcile_DomainBasicAuth_ProtectedDomain_GetsAuthHandler(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services:  []store.DesiredService{desired},
		basicAuth: []store.DomainBasicAuth{{Domain: "web.example.com", Username: "operator"}},
	}
	key := store.DomainBasicAuthSecretsKey("web.example.com")
	resolver := &fakePasswordResolver{passwords: map[string]string{key: "hunter2"}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithDomainBasicAuthSecrets(resolver))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed1Services" {
		t.Errorf("condition reason = %q, want Routed1Services", cond.Reason)
	}

	wantArgs := [2]string{key, store.DomainBasicAuthPasswordEnvKey}
	if len(resolver.resolveArgs) != 1 || resolver.resolveArgs[0] != wantArgs {
		t.Errorf("Resolve() called with %v, want exactly [%v]", resolver.resolveArgs, wantArgs)
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("applied routes = %+v, want exactly 1", routes)
	}
	route := routes[0]
	if len(route.Handle) != 2 {
		t.Fatalf("route.Handle = %+v, want [authentication, reverse_proxy]", route.Handle)
	}
	authHandler, ok := route.Handle[0].(ingress.BasicAuthHandler)
	if !ok {
		t.Fatalf("route.Handle[0] is %T, want ingress.BasicAuthHandler", route.Handle[0])
	}
	if authHandler.Providers.HTTPBasic == nil || len(authHandler.Providers.HTTPBasic.Accounts) != 1 {
		t.Fatalf("authHandler = %+v, want exactly one account", authHandler)
	}
	account := authHandler.Providers.HTTPBasic.Accounts[0]
	if account.Username != "operator" {
		t.Errorf("account.Username = %q, want operator", account.Username)
	}
	if account.Password == "hunter2" {
		t.Errorf("account.Password is the raw plaintext, want a bcrypt hash")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte("hunter2")); err != nil {
		t.Errorf("stored hash does not verify against the real password: %v", err)
	}
	if _, ok := route.Handle[1].(ingress.ReverseProxyHandler); !ok {
		t.Errorf("route.Handle[1] is %T, want ingress.ReverseProxyHandler (must run after authentication)", route.Handle[1])
	}
}

// TestController_Reconcile_DomainBasicAuth_ResolveError_DomainSkippedNotFailed
// proves a password-resolve failure never fails the whole reconcile: the
// protected domain is simply left unrouted this pass, the safer failure
// mode than serving it unprotected.
func TestController_Reconcile_DomainBasicAuth_ResolveError_DomainSkippedNotFailed(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services:  []store.DesiredService{desired},
		basicAuth: []store.DomainBasicAuth{{Domain: "web.example.com", Username: "operator"}},
	}
	resolver := &fakePasswordResolver{err: errors.New("secrets: master key not set")}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithDomainBasicAuthSecrets(resolver))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil: a password-resolve failure must not fail the whole reconcile", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed0Services" {
		t.Errorf("condition reason = %q, want Routed0Services", cond.Reason)
	}
	if routes := applier.routes(t); len(routes) != 0 {
		t.Errorf("applied routes = %+v, want none: failing closed on a resolve error", routes)
	}
}

// TestController_Reconcile_DomainBasicAuth_MixedDomains_SplitsIntoTwoRoutes
// proves one service with one protected and one unprotected domain
// produces two separate ProxyRoutes: the protected host alone with its
// own authentication handler, and every unprotected host grouped into
// one shared route with none, exactly as before this feature existed.
func TestController_Reconcile_DomainBasicAuth_MixedDomains_SplitsIntoTwoRoutes(t *testing.T) {
	desired := store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"protected.example.com", "open.example.com"},
	}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services:  []store.DesiredService{desired},
		basicAuth: []store.DomainBasicAuth{{Domain: "protected.example.com", Username: "operator"}},
	}
	key := store.DomainBasicAuthSecretsKey("protected.example.com")
	resolver := &fakePasswordResolver{passwords: map[string]string{key: "hunter2"}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithDomainBasicAuthSecrets(resolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	routes := applier.routes(t)
	if len(routes) != 2 {
		t.Fatalf("applied routes = %+v, want exactly 2 (one protected, one open)", routes)
	}

	var sawProtected, sawOpen bool
	for _, route := range routes {
		hosts := route.Match[0].Host
		switch {
		case len(hosts) == 1 && hosts[0] == "protected.example.com":
			sawProtected = true
			if len(route.Handle) != 2 {
				t.Errorf("protected route.Handle = %+v, want [authentication, reverse_proxy]", route.Handle)
			}
		case len(hosts) == 1 && hosts[0] == "open.example.com":
			sawOpen = true
			if len(route.Handle) != 1 {
				t.Errorf("open route.Handle = %+v, want [reverse_proxy] only", route.Handle)
			}
		default:
			t.Errorf("unexpected route hosts %+v", hosts)
		}
	}
	if !sawProtected || !sawOpen {
		t.Errorf("routes = %+v, want one protected.example.com route and one open.example.com route", routes)
	}
}
