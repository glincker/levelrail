package ingress

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDialForService_KeepsLastBackendWhenDockerErrors(t *testing.T) {
	svc := store.DesiredService{Name: "web", Image: "web:1", Port: 80}
	name := application.ContainerName(svc.Name, application.NameImage(svc), svc.RestartNonce)
	rt := newFakeRuntime()
	rt.seedRunning(name, 4321)
	c := New(&fakeStore{}, rt, &fakeApplier{}, WithLogger(discardLogger()))

	dial, ok := c.dialForService(context.Background(), svc, true)
	if !ok || dial != "127.0.0.1:4321" {
		t.Fatalf("healthy dial = %q, %t", dial, ok)
	}

	rt.seedInspectErr(name, errors.New("cannot connect to the Docker daemon"))
	dial, ok = c.dialForService(context.Background(), svc, true)
	if !ok || dial != "127.0.0.1:4321" {
		t.Fatalf("dial during outage = %q, %t, want the last known backend", dial, ok)
	}

	other := store.DesiredService{Name: "never-seen", Image: "x:1", Port: 80}
	otherName := application.ContainerName(other.Name, application.NameImage(other), other.RestartNonce)
	rt.seedInspectErr(otherName, errors.New("cannot connect to the Docker daemon"))
	if _, ok := c.dialForService(context.Background(), other, true); ok {
		t.Fatal("a service never resolved must not get an invented backend")
	}
}
