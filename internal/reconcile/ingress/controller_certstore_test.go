package ingress

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TASKS.md 3.6 tests: the domain-uniqueness defense-in-depth guard in
// Reconcile, and WithCertStore actually threading through to
// ingress.RoutesOptions.CertStorage. Split into its own file from
// controller_test.go per this codebase's max-500-lines-per-file rule.

func TestController_Reconcile_DuplicateDomain_DefenseInDepthSkipsLoser(t *testing.T) {
	// store.SaveDesiredService (TASKS.md 3.6) now rejects a write that
	// would create this situation for any service saved after that
	// change landed, but fakeStore here stands in for data that could
	// still exist from before the constraint did (or a hypothetical
	// direct write bypassing SaveDesiredService); this controller must
	// still never hand Caddy two routes for the same host.
	first := store.DesiredService{Name: "web-a", Image: "img:v1", Port: 80, Domains: []string{"shared.example.com"}}
	second := store.DesiredService{Name: "web-b", Image: "img:v2", Port: 80, Domains: []string{"shared.example.com"}}

	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(first.Name, first.Image, ""), 11111)
	rt.seedRunning(application.ContainerName(second.Name, second.Image, ""), 22222)

	st := &fakeStore{services: []store.DesiredService{first, second}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed1Services" {
		t.Errorf("condition.Reason = %q, want Routed1Services: exactly one of the two colliding services must be routed", cond.Reason)
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("applied routes = %v, want exactly 1: two services must never both get a route for the same host", routes)
	}
	if routes[0].Match[0].Host[0] != "shared.example.com" {
		t.Errorf("routed host = %q, want shared.example.com", routes[0].Match[0].Host[0])
	}
}

// fakeCertStore is a minimal, no-op fake for ingress.CertStore: enough to
// satisfy the interface for a wiring test (WithCertStore), the actual
// certmagic.Storage behavior over a CertStore is covered exhaustively in
// internal/ingress/certstorage_test.go, this package only needs to prove
// the option is actually threaded through to RoutesOptions.CertStorage.
type fakeCertStore struct{}

func (fakeCertStore) SaveCertStorageValue(context.Context, string, []byte) error { return nil }
func (fakeCertStore) GetCertStorageValue(context.Context, string) (*store.CertStorageValue, error) {
	return nil, store.ErrCertStorageKeyNotFound
}
func (fakeCertStore) DeleteCertStorageValue(context.Context, string) error { return nil }
func (fakeCertStore) ExistsCertStorageValue(context.Context, string) (bool, error) {
	return false, nil
}
func (fakeCertStore) ListCertStorageKeys(context.Context, string, bool) ([]string, error) {
	return nil, nil
}
func (fakeCertStore) StatCertStorageValue(context.Context, string) (*store.CertStorageKeyInfo, error) {
	return nil, store.ErrCertStorageKeyNotFound
}
func (fakeCertStore) AcquireCertStorageLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}
func (fakeCertStore) TouchCertStorageLock(context.Context, string) error   { return nil }
func (fakeCertStore) ReleaseCertStorageLock(context.Context, string) error { return nil }

func TestController_WithCertStore_UsesSQLiteStorageInsteadOfFile(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	c := New(st, rt, applier,
		WithLogger(discardLogger()),
		WithStorageDir("/should/be/ignored"),
		WithCertStore(fakeCertStore{}),
	)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if applier.lastCfg == nil {
		t.Fatal("no config was applied")
	}
	ref, ok := applier.lastCfg.Storage.(ingress.SQLiteStorageRef)
	if !ok {
		t.Fatalf("applied Config.Storage = %#v (%T), want ingress.SQLiteStorageRef: WithCertStore must take precedence over WithStorageDir", applier.lastCfg.Storage, applier.lastCfg.Storage)
	}
	if ref.Module != "sqlite" {
		t.Errorf("Storage module = %q, want sqlite", ref.Module)
	}
}
