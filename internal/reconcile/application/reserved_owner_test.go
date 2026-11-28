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

type eventFakeStore struct {
	fakeStore
	events []store.AppEvent
}

func (s *eventFakeStore) AddAppEvent(_ context.Context, e store.AppEvent) error {
	s.events = append(s.events, e)
	return nil
}

// A retrying reconcile stores the reserved-name refusal once, not on every pass.
func TestReservedOwnerRefusalStoresOneCoalescedEvent(t *testing.T) {
	st := &eventFakeStore{}
	c := New("vault", st, newFakeRuntime(0))
	for i := 0; i < 5; i++ {
		if err := c.refuseReservedSecretOwner(); !errors.Is(err, errReservedSecretOwner) {
			t.Fatalf("pass %d err = %v", i, err)
		}
	}
	if len(st.events) != 1 || st.events[0].Kind != store.AppEventReservedSecretOwner || st.events[0].AppName != "vault" {
		t.Fatalf("events = %+v, want one reserved_secret_owner event for vault", st.events)
	}
}
