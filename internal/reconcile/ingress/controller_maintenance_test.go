package ingress

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// maintenanceSettingsStore adds the optional settings lister to fakeStore.
type maintenanceSettingsStore struct {
	*fakeStore
	settings []store.DomainMaintenance
}

func (s maintenanceSettingsStore) ListActiveDomainMaintenance(context.Context, time.Time) ([]store.DomainMaintenance, error) {
	return s.settings, nil
}

func appliedJSON(t *testing.T, a *fakeApplier) string {
	t.Helper()
	b, err := json.Marshal(a.routes(t))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func healthyService() store.DesiredService {
	return store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"ops.example.com"},
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz"}},
	}
}

func TestController_Maintenance_BypassGateAndPage(t *testing.T) {
	desired := healthyService()
	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(desired.Name, desired.Image, ""), 34567)
	st := maintenanceSettingsStore{
		fakeStore: &fakeStore{services: []store.DesiredService{desired}, maintenance: []string{"ops.example.com"}},
		settings: []store.DomainMaintenance{{
			Domain: "ops.example.com", Title: "Upgrading", RetryAfterSeconds: 90,
			BypassCIDRs: []string{"203.0.113.0/24"}, BypassHealthCheck: true,
		}},
	}
	applier := &fakeApplier{}
	if _, err := New(st, rt, applier, WithLogger(discardLogger())).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := appliedJSON(t, applier)
	for _, want := range []string{`"client_ip":{"ranges":["203.0.113.0/24"]}`, `"path":["/healthz"]`, `"Retry-After":["90"]`, "Upgrading", `"reverse_proxy"`} {
		if !strings.Contains(got, want) {
			t.Errorf("applied config lacks %s:\n%s", want, got)
		}
	}
}

// Without a running container there is nothing to bypass to, so the gated
// host falls back to the plain page instead of going unrouted.
func TestController_Maintenance_BypassWithoutBackendStillServesPage(t *testing.T) {
	desired := healthyService()
	st := maintenanceSettingsStore{
		fakeStore: &fakeStore{services: []store.DesiredService{desired}, maintenance: []string{"ops.example.com"}},
		settings:  []store.DomainMaintenance{{Domain: "ops.example.com", BypassCIDRs: []string{"203.0.113.0/24"}}},
	}
	applier := &fakeApplier{}
	if _, err := New(st, newFakeRuntime(), applier, WithLogger(discardLogger())).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := appliedJSON(t, applier)
	if !strings.Contains(got, `"status_code":503`) || strings.Contains(got, "reverse_proxy") || strings.Contains(got, "client_ip") {
		t.Errorf("want only the plain maintenance page:\n%s", got)
	}
}

// A health bypass without a readiness path, and no CIDRs, needs no gate.
func TestController_Maintenance_NoBypassKeepsPlainRoute(t *testing.T) {
	desired := healthyService()
	desired.Health = nil
	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(desired.Name, desired.Image, ""), 34567)
	st := maintenanceSettingsStore{
		fakeStore: &fakeStore{services: []store.DesiredService{desired}, maintenance: []string{"ops.example.com"}},
		settings:  []store.DomainMaintenance{{Domain: "ops.example.com", BypassHealthCheck: true, Message: "Back at noon"}},
	}
	applier := &fakeApplier{}
	if _, err := New(st, rt, applier, WithLogger(discardLogger())).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := appliedJSON(t, applier)
	if strings.Contains(got, "reverse_proxy") || !strings.Contains(got, "Back at noon") {
		t.Errorf("want only the plain page with the message:\n%s", got)
	}
}
