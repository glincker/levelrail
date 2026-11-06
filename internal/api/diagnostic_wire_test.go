package api

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeDiagStore struct {
	svcs            []store.DesiredService
	queued, running []store.DeployAttempt
}

func (f fakeDiagStore) ListDesiredServices(context.Context) ([]store.DesiredService, error) {
	return f.svcs, nil
}
func (f fakeDiagStore) ListQueuedDeployAttempts(context.Context) ([]store.DeployAttempt, error) {
	return f.queued, nil
}
func (f fakeDiagStore) ListRunningDeployAttempts(context.Context) ([]store.DeployAttempt, error) {
	return f.running, nil
}

type fakeDiagContainers []docker.ContainerState

func (f fakeDiagContainers) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return f, nil
}

func TestDiagnosticFactsMapsStates(t *testing.T) {
	now := time.Now()
	st := fakeDiagStore{
		svcs: []store.DesiredService{
			{Name: "build", Replicas: 1, Domains: []string{"build.test"}},
			{Name: "off", Replicas: 1, Suspended: true, Domains: []string{"off.test"}},
			{Name: "zero", Replicas: 0, Domains: []string{"zero.test"}},
			{Name: "new", Replicas: 1, Domains: []string{"new.test"}},
			{Name: "boot", Replicas: 1, Domains: []string{"boot.test"}},
			{Name: "dead", Replicas: 1, Domains: []string{"dead.test"}},
			{Name: "web", Replicas: 1, Domains: []string{"web.test"}},
		},
		queued: []store.DeployAttempt{{ServiceName: "build"}},
	}
	cs := fakeDiagContainers{
		{Name: "boot-1", Running: true, Created: now.Add(-5 * time.Second)},
		{Name: "dead", Running: false},
		{Name: "web", Running: true, Created: now.Add(-time.Hour)},
		{Name: "web-api", Running: true, Created: now}, // another service, must not count for web
	}
	snap := ingress.NewDiagnosticSnapshot(DiagnosticFacts(st, cs, time.Minute, func() time.Time { return now }))
	if err := snap.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := map[string]ingress.HostState{
		"build.test": ingress.StateBuilding, "off.test": ingress.StateStopped, "zero.test": ingress.StateStopped,
		"new.test": ingress.StateStarting, "boot.test": ingress.StateStarting, "dead.test": ingress.StateUnhealthy,
		"web.test": ingress.StateUnknown, "gone.test": ingress.StateUnattached,
	}
	for host, w := range want {
		if got := snap.Lookup(context.Background(), host).State; got != w {
			t.Errorf("%s: got %s want %s", host, got, w)
		}
	}
}
