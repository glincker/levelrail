package application

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeVaultSettingsStore is the narrow VaultSettingsStore fake this
// package's controller tests need, the same shape fakeSecretResolver
// establishes for SecretResolver.
type fakeVaultSettingsStore struct {
	settings store.VaultSettings
	err      error
}

func (f *fakeVaultSettingsStore) GetVaultSettings(context.Context) (store.VaultSettings, error) {
	if f.err != nil {
		return store.VaultSettings{}, f.err
	}
	return f.settings, nil
}

// fakeVaultResolver is the narrow VaultResolver fake, keyed by
// "path#key" so a test can assert exactly which reference was resolved.
type fakeVaultResolver struct {
	values       map[string]string
	err          error
	resolveCalls int
}

func (f *fakeVaultResolver) Resolve(_ context.Context, _ store.VaultSettings, _ string, path, key string) (string, error) {
	f.resolveCalls++
	if f.err != nil {
		return "", f.err
	}
	return f.values[path+"#"+key], nil
}

func TestController_Reconcile_VaultEnv_NotConfigured_FailsLoudly(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		VaultEnv: map[string]store.VaultEnvRef{"API_KEY": {Path: "myapp/config", Key: "api_key"}},
	}
	c := New("web", &fakeStore{svc: desired}, rt) // no WithVaultSettings/WithVaultResolver/WithSecretResolver

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a failure: a vault-backed env var with nothing configured must never silently start a container missing it")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "CreateFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=CreateFailed", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

func TestController_Reconcile_VaultEnv_Disabled_FailsLoudly(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		VaultEnv: map[string]store.VaultEnvRef{"API_KEY": {Path: "myapp/config", Key: "api_key"}},
	}
	settings := &fakeVaultSettingsStore{settings: store.VaultSettings{Enabled: false}}
	resolver := &fakeVaultResolver{}
	secrets := newFakeSecretResolver(map[string]string{"vault/credential": "a-token"})
	c := New("web", &fakeStore{svc: desired}, rt,
		WithVaultSettings(settings), WithVaultResolver(resolver), WithSecretResolver(secrets))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a failure: vault configured but disabled must not silently deploy with the var unset")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse {
		t.Errorf("condition = %+v, want Status=False", cond)
	}
	if resolver.resolveCalls != 0 {
		t.Errorf("resolveCalls = %d, want 0: must never reach Vault when disabled", resolver.resolveCalls)
	}
}

func TestController_Reconcile_VaultEnv_Resolved_MergedIntoContainerEnv(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Env:      map[string]string{"NODE_ENV": "production"},
		VaultEnv: map[string]store.VaultEnvRef{"API_KEY": {Path: "myapp/config", Key: "api_key"}},
	}
	settings := &fakeVaultSettingsStore{settings: store.VaultSettings{Enabled: true, Address: "https://vault:8200"}}
	resolver := &fakeVaultResolver{values: map[string]string{"myapp/config#api_key": "live-secret"}}
	secrets := newFakeSecretResolver(map[string]string{"vault/credential": "a-token"})
	c := New("web", &fakeStore{svc: desired}, rt,
		WithVaultSettings(settings), WithVaultResolver(resolver), WithSecretResolver(secrets))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Status=True", cond)
	}
	if got := rt.lastCreateEnv["API_KEY"]; got != "live-secret" {
		t.Errorf("container env API_KEY = %q, want the resolved vault value", got)
	}
	if got := rt.lastCreateEnv["NODE_ENV"]; got != "production" {
		t.Errorf("container env NODE_ENV = %q, want the literal value preserved alongside the resolved vault value", got)
	}
}

func TestController_Reconcile_VaultEnv_UnreachableVault_FailsLoudly(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		VaultEnv: map[string]store.VaultEnvRef{"API_KEY": {Path: "myapp/config", Key: "api_key"}},
	}
	settings := &fakeVaultSettingsStore{settings: store.VaultSettings{Enabled: true, Address: "https://vault:8200"}}
	resolver := &fakeVaultResolver{err: errors.New("dial tcp: connection refused")}
	secrets := newFakeSecretResolver(map[string]string{"vault/credential": "a-token"})
	c := New("web", &fakeStore{svc: desired}, rt,
		WithVaultSettings(settings), WithVaultResolver(resolver), WithSecretResolver(secrets))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the resolver's error (unreachable Vault) to propagate, never a container created with an empty value")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse {
		t.Errorf("condition = %+v, want Status=False", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: a deploy must fail loudly, not deploy with an empty value, when vault is unreachable", rt.createCalls)
	}
}

func TestController_Reconcile_NoVaultEnv_ResolverNeverCalled(t *testing.T) {
	rt := newFakeRuntime(0)
	settings := &fakeVaultSettingsStore{settings: store.VaultSettings{Enabled: true}}
	resolver := &fakeVaultResolver{}
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt, WithVaultSettings(settings), WithVaultResolver(resolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if resolver.resolveCalls != 0 {
		t.Errorf("resolveCalls = %d, want 0", resolver.resolveCalls)
	}
}
