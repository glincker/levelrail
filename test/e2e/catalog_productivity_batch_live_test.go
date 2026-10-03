// TestCatalogProductivityBatch_Live_Deploys proves three productivity-
// batch catalog entries boot for real: the same parse/resolve/translate
// pipeline handleDeployCompose uses, reconciled against real Docker in
// dependency order, secrets included.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCatalogProductivityBatch_Live_Deploys(t *testing.T) {
	env := newLiveBuildEnv(t)

	t.Run("triliumnext", func(t *testing.T) {
		deployCatalogTemplateLive(t, env.Runtime, "triliumnext", "levelrail-test-e2e-cat-trilium", []string{"triliumnext"})
	})
	t.Run("redmine", func(t *testing.T) {
		deployCatalogTemplateLive(t, env.Runtime, "redmine", "levelrail-test-e2e-cat-redmine", []string{"postgresql", "redmine"})
	})
	t.Run("docuseal", func(t *testing.T) {
		deployCatalogTemplateLive(t, env.Runtime, "docuseal", "levelrail-test-e2e-cat-docuseal", []string{"postgresql", "docuseal"})
	})
}

// deployCatalogTemplateLive deploys templateID's real catalog.Template
// Compose body under appName, reconciling serviceKeysInOrder in the
// order given (the dependency first: manual ordering, same as
// multi_service_test.go's own fan-out, rather than trusting an
// un-inspected automatic depends_on wait), and fails the test unless
// every one converges to a True/Ready condition.
func deployCatalogTemplateLive(t *testing.T, runtime docker.Runtime, templateID, appName string, serviceKeysInOrder []string) {
	t.Helper()

	tpl, ok := catalog.TemplateByID(templateID)
	if !ok {
		t.Fatalf("no catalog template with ID %q", templateID)
	}

	for _, key := range serviceKeysInOrder {
		serviceName := appName + "-" + key
		cleanupContainers(context.Background(), t, runtime, serviceName)
		t.Cleanup(func() { cleanupContainers(context.Background(), t, runtime, serviceName) })
	}

	svcStore := openLiveStore(t)

	file, err := compose.Parse([]byte(tpl.Compose))
	if err != nil {
		t.Fatalf("compose.Parse() error = %v", err)
	}

	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	manager := secrets.NewManager(svcStore, mk)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	generate := func(kind, _ string, length int) (string, error) {
		return compose.GenerateValue(kind, length)
	}
	persist := func(serviceKey, envKey, value string) error {
		return manager.SetValue(ctx, appName+"-"+serviceKey, envKey, value)
	}
	secretEnv, unresolved, err := compose.ResolveMagicVars(file, generate, persist)
	if err != nil {
		t.Fatalf("compose.ResolveMagicVars() error = %v", err)
	}
	if len(unresolved) > 0 {
		t.Fatalf("compose.ResolveMagicVars() left %d unresolved var(s): %v", len(unresolved), unresolved)
	}

	services, warnings, err := compose.ToDesiredServices(appName, file)
	if err != nil {
		t.Fatalf("compose.ToDesiredServices() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("compose.ToDesiredServices() warnings = %v, want none for template %q", warnings, templateID)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := svcStore.SaveApp(ctx, store.App{ID: appName, Name: appName, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveApp() error = %v", err)
	}

	byKey := make(map[string]store.DesiredService, len(services))
	for _, svc := range services {
		key := svc.Name[len(appName)+1:]
		svc.SecretEnv = store.SecretEnvRefsFromNames(secretEnv[key])
		byKey[key] = svc
		if err := svcStore.SaveDesiredService(ctx, svc); err != nil {
			t.Fatalf("SaveDesiredService(%q) error = %v", svc.Name, err)
		}
	}

	for _, key := range serviceKeysInOrder {
		svc, ok := byKey[key]
		if !ok {
			t.Fatalf("template %q: no translated service for key %q", templateID, key)
		}
		ctrl := application.New(svc.Name, svcStore, runtime, application.WithSecretResolver(manager), application.WithReadyBudget(3*time.Minute))
		result, err := ctrl.Reconcile(ctx)
		if err != nil {
			t.Fatalf("Reconcile(%q) error = %v, result = %+v", svc.Name, err, result)
		}
		if len(result.Conditions) == 0 || result.Conditions[0].Status != "True" {
			t.Fatalf("Reconcile(%q) result = %+v, want a True condition", svc.Name, result)
		}

		containerName := application.ContainerName(svc.Name, svc.Image, "")
		state, err := runtime.InspectByName(ctx, containerName)
		if err != nil {
			t.Fatalf("InspectByName(%q) error = %v", containerName, err)
		}
		if state == nil || !state.Running {
			t.Fatalf("InspectByName(%q) = %+v, want a running container", containerName, state)
		}
	}
}
