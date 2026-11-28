package application

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// An existing app named like a control plane namespace never gets that namespace's secrets.
func TestResolveEnv_ReservedAppNameRefusedBeforeAnyLookup(t *testing.T) {
	resolver := newFakeSecretResolver(map[string]string{"vault/credential": "platform-token"})
	desired := &store.DesiredService{Name: "vault", SecretEnv: []store.SecretEnvRef{{Name: "credential"}}}
	c := New("vault", &fakeStore{}, newFakeRuntime(0), WithSecretResolver(resolver))

	env, err := c.resolveEnv(context.Background(), desired)
	if !errors.Is(err, errReservedSecretOwner) {
		t.Fatalf("resolveEnv error = %v, want errReservedSecretOwner", err)
	}
	if env != nil {
		t.Errorf("env = %v, want nil", env)
	}
	if resolver.existsCalls != 0 {
		t.Errorf("resolver was consulted %d times, want 0", resolver.existsCalls)
	}
}
