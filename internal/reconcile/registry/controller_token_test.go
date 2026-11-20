package registry

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func tokenAuthFixture() *TokenAuth {
	return &TokenAuth{Realm: "https://cp.example/api/v1/registry/token", Service: "svc", Issuer: "lr", CertBundlePEM: []byte("-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----\n")}
}

func newTokenController(rt *fakeRuntime, tok *TokenAuth) *Controller {
	st := &fakeStore{settings: store.RegistrySettings{Enabled: true, Host: "registry.example", Username: "levelrail"}}
	return New(st, &fakeCreds{set: true, value: "s3cret"}, rt, WithContainerPrefix("acme"),
		WithTokenAuth(func(context.Context) *TokenAuth { return tok }))
}

func TestReconcile_TokenAuthOnlyWhenConfigured(t *testing.T) {
	rt := newFakeRuntime()
	if _, err := newTokenController(rt, nil).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.lastCreateSpec.Env["REGISTRY_AUTH"] != "htpasswd" || len(rt.lastCreateSpec.Labels) != 0 || TokenAuthActive() {
		t.Fatalf("default must stay htpasswd: %v active=%v", rt.lastCreateSpec.Env["REGISTRY_AUTH"], TokenAuthActive())
	}
	// An incomplete setup also keeps htpasswd.
	rt = newFakeRuntime()
	if _, err := newTokenController(rt, &TokenAuth{Realm: "x"}).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.lastCreateSpec.Env["REGISTRY_AUTH"] != "htpasswd" {
		t.Fatal("incomplete token setup must not switch modes")
	}
}

func TestReconcile_TokenAuthSetsEnvAndReplacesOnModeChange(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(image, true) // existing htpasswd container, no label
	c := newTokenController(rt, tokenAuthFixture())
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != 1 {
		t.Fatalf("a mode change must replace the container, creates=%d", rt.createCalls)
	}
	env := rt.lastCreateSpec.Env
	if env["REGISTRY_AUTH"] != "token" || env["REGISTRY_AUTH_TOKEN_REALM"] != tokenAuthFixture().Realm ||
		env["REGISTRY_AUTH_TOKEN_SERVICE"] != "svc" || env["REGISTRY_AUTH_TOKEN_ISSUER"] != "lr" ||
		env["REGISTRY_AUTH_TOKEN_ROOTCERTBUNDLE"] == "" || env[tokenBundleEnv] == "" {
		t.Fatalf("token env incomplete: %v", env)
	}
	if _, has := env[htpasswdHashEnv]; has {
		t.Error("token mode must not carry the htpasswd hash")
	}
	if !TokenAuthActive() {
		t.Error("active flag must be set after a ready token pass")
	}
	// Unchanged setup does not recreate.
	if _, err := c.Reconcile(context.Background()); err != nil || rt.createCalls != 1 {
		t.Fatalf("stable pass recreated: %v creates=%d", err, rt.createCalls)
	}
	// Dropping token auth goes back to htpasswd with a replace and clears the flag.
	if _, err := newTokenController(rt, nil).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != 2 || rt.lastCreateSpec.Env["REGISTRY_AUTH"] != "htpasswd" || TokenAuthActive() {
		t.Fatalf("revert failed: creates=%d active=%v", rt.createCalls, TokenAuthActive())
	}
}
