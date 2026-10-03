// TestCatalogAutomationBatch_Live proves, against real Docker, that a
// sample of templates_automation_batch.go's templates actually boot:
// real compose resolve/translate plus a real application.Controller
// reconcile per service. Static tests only prove the Compose parses;
// this is the one check that proves the pinned image tag is real.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCatalogAutomationBatch_Live(t *testing.T) {
	env := newLiveBuildEnv(t)
	runtime := env.Runtime
	dockerCli := env.DockerCli
	svcStore := openLiveStore(t)

	tests := []struct {
		id          string
		readyBudget time.Duration
	}{
		{id: "actualbudget", readyBudget: 90 * time.Second},
		{id: "linkding-plus", readyBudget: 90 * time.Second},
		{id: "openobserve", readyBudget: 120 * time.Second},
		{id: "onetimesecret", readyBudget: 90 * time.Second},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.id, func(t *testing.T) {
			tpl, ok := catalog.TemplateByID(tt.id)
			if !ok {
				t.Fatalf("no catalog template %q", tt.id)
			}

			appName := "levelrail-test-e2e-" + tt.id
			services := resolveAndSaveTemplate(t, svcStore, appName, tpl.Compose)

			cleanupTemplateResources := func() {
				for _, svc := range services {
					cleanupContainers(context.Background(), t, runtime, svc.Name)
					for _, v := range svc.Volumes {
						if v.Name != "" {
							_ = dockerCli.VolumeRemove(context.Background(), v.Name, true)
						}
					}
				}
				_ = dockerCli.NetworkRemove(context.Background(), application.NetworkName("", appName))
			}
			cleanupTemplateResources()
			t.Cleanup(cleanupTemplateResources)

			ctx, cancel := context.WithTimeout(context.Background(), tt.readyBudget+30*time.Second)
			defer cancel()

			for _, svc := range orderByDependsOn(services) {
				ctrl := application.New(svc.Name, svcStore, runtime, application.WithReadyBudget(tt.readyBudget))
				result, err := ctrl.Reconcile(ctx)
				if err != nil {
					t.Fatalf("Reconcile(%q) error = %v, conditions = %+v", svc.Name, err, result.Conditions)
				}

				state, err := runtime.InspectByName(ctx, application.ContainerName(svc.Name, svc.Image, ""))
				if err != nil {
					t.Fatalf("InspectByName(%q) error = %v", svc.Name, err)
				}
				if state == nil || !state.Running {
					t.Fatalf("service %q: container state = %+v, want running", svc.Name, state)
				}
			}
		})
	}
}

// resolveAndSaveTemplate parses composeBody, resolves every SERVICE_
// magic var with real generated values (re-spliced into the service
// Environment directly, rather than routed through secrets storage,
// since this test only needs the running container to see the right
// literal env, not a real secrets-at-rest round trip), converts to
// store.DesiredService, and saves each one, returning them in
// compose's own sorted-by-name order.
func resolveAndSaveTemplate(t *testing.T, svcStore *store.DB, appName, composeBody string) []store.DesiredService {
	t.Helper()
	ctx := context.Background()

	f, err := compose.Parse([]byte(composeBody))
	if err != nil {
		t.Fatalf("compose.Parse() error = %v", err)
	}

	generated := make(map[string]map[string]string)
	persist := func(serviceKey, envKey, value string) error {
		if generated[serviceKey] == nil {
			generated[serviceKey] = make(map[string]string)
		}
		generated[serviceKey][envKey] = value
		return nil
	}
	generate := func(kind, _ string, length int) (string, error) {
		return compose.GenerateValue(kind, length)
	}

	secretEnv, unresolved, err := compose.ResolveMagicVars(f, generate, persist)
	if err != nil {
		t.Fatalf("compose.ResolveMagicVars() error = %v", err)
	}
	if len(unresolved) > 0 {
		t.Fatalf("compose.ResolveMagicVars() unresolved = %v", unresolved)
	}
	for svcKey, envKeys := range secretEnv {
		svc := f.Services[svcKey]
		for _, envKey := range envKeys {
			svc.Environment[envKey] = generated[svcKey][envKey]
		}
	}

	if err := f.Validate(); err != nil {
		t.Fatalf("(*compose.File).Validate() error = %v", err)
	}

	services, warnings, err := compose.ToDesiredServices(appName, f)
	if err != nil {
		t.Fatalf("compose.ToDesiredServices() error = %v", err)
	}
	for _, w := range warnings {
		t.Logf("healthcheck warning: %s", w)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := svcStore.SaveApp(ctx, store.App{ID: appName, Name: appName, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveApp() error = %v", err)
	}
	for _, svc := range services {
		if err := svcStore.SaveDesiredService(ctx, svc); err != nil {
			t.Fatalf("SaveDesiredService(%q) error = %v", svc.Name, err)
		}
	}
	return services
}

// orderByDependsOn returns services with every dependency-free service
// first, so reconciling in this order never hits
// application.Controller's own WaitingForDependency gate (depends_on.go):
// every template in this batch nests at most one level deep (an app
// plus its own db/cache sidecars), so a single partition is enough.
func orderByDependsOn(services []store.DesiredService) []store.DesiredService {
	out := make([]store.DesiredService, 0, len(services))
	for _, svc := range services {
		if len(svc.DependsOn) == 0 {
			out = append(out, svc)
		}
	}
	for _, svc := range services {
		if len(svc.DependsOn) > 0 {
			out = append(out, svc)
		}
	}
	return out
}
