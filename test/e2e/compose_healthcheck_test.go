// TestComposeHealthcheck_Live_GatesDeploySuccess is this package's live
// proof for internal/compose's healthcheck translation (healthcheck.go,
// translate.go): internal/compose/healthcheck_test.go already proves
// resolveHealthcheck turns a curl/wget CMD-SHELL stanza into a
// store.ServiceProbe in isolation, and internal/reconcile/application's
// own controller_live_test.go already proves a hand-built
// store.ServiceHealth gates a real deploy on a real HTTP probe. Neither
// proves the two compose here: that a real compose.yaml document, run
// through the real ToDesiredServices translation, produces a readiness
// probe strict enough to actually fail a deploy against a container that
// never serves the declared health path. This is exactly the failure
// mode CLAUDE.md section 10 names as the project's main risk: a health
// check that silently stops being enforced.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestComposeHealthcheck_Live_GatesDeploySuccess builds
// test/fixtures/hello-e2e once (it serves only "/", see that fixture's
// own Dockerfile) and reconciles it twice under two distinct compose
// healthcheck stanzas: one pointed at "/" (a path the container
// genuinely answers), one pointed at a path it never will. The healthy
// case must converge to Ready; the permanently-unhealthy case must
// return a non-nil error and a False/ReadinessFailed condition, proving
// the probe compose.ToDesiredServices produces is not a decoration.
func TestComposeHealthcheck_Live_GatesDeploySuccess(t *testing.T) {
	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	t.Cleanup(func() { _ = dockerCli.Close() })

	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = dockerCli.Ping(pingCtx)
	cancel()
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	buildClient, err := build.NewClient(connectCtx, dockerCli)
	cancel()
	if err != nil {
		t.Skipf("could not connect to buildkit: %v", err)
	}
	t.Cleanup(func() { _ = buildClient.Close() })

	runtime, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("closing docker.Client: %v", err)
		}
	})

	const appName = "levelrail-test-e2e-compose-hc"
	repo := "levelrail/test-e2e-compose-hc"
	tag := repo + ":e2ecomposehc1"

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = dockerCli.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	res, err := buildClient.Build(ctx, build.Request{ContextDir: "../fixtures/hello-e2e", Tag: tag}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	t.Run("permanently unhealthy path fails the deploy", func(t *testing.T) {
		const serviceKey = "web-bad"
		serviceName := appName + "-" + serviceKey
		cleanupContainers(context.Background(), t, runtime, serviceName)
		t.Cleanup(func() { cleanupContainers(context.Background(), t, runtime, serviceName) })

		desired := composeDesiredService(t, appName, serviceKey, res.Tag, "/this-path-never-exists")
		svcStore := openLiveStore(t)
		saveComposeApp(t, svcStore, appName)
		if err := svcStore.SaveDesiredService(ctx, desired); err != nil {
			t.Fatalf("SaveDesiredService() error = %v", err)
		}

		ctrl := application.New(serviceName, svcStore, runtime, application.WithReadyBudget(6*time.Second))
		result, err := ctrl.Reconcile(ctx)
		if err == nil {
			t.Fatalf("Reconcile() error = nil, want a readiness failure for a healthcheck path the container never serves")
		}
		if len(result.Conditions) == 0 {
			t.Fatalf("Reconcile() result = %+v, want at least one condition", result)
		}
		cond := result.Conditions[0]
		if cond.Status != "False" {
			t.Errorf("condition Status = %q, want %q", cond.Status, "False")
		}
		if cond.Reason != "ReadinessFailed" {
			t.Errorf("condition Reason = %q, want %q", cond.Reason, "ReadinessFailed")
		}
		if cond.Message == "" {
			t.Error("condition Message is empty, want the probe's own failure reason surfaced")
		}

		// This is the exact failure mode CLAUDE.md section 10 calls out:
		// the container the controller created is genuinely still
		// running (it never became unhealthy, it simply never answers
		// the declared path), so a caller that only checked "is
		// something running" rather than Reconcile's own error and
		// False condition would wrongly call this a successful deploy.
		target := application.ContainerName(serviceName, res.Tag, "")
		state, err := runtime.InspectByName(ctx, target)
		if err != nil {
			t.Fatalf("InspectByName(%q) error = %v", target, err)
		}
		if state == nil || !state.Running {
			t.Fatalf("InspectByName(%q) = %+v, want the container to exist and be running despite the failed readiness gate", target, state)
		}
	})

	t.Run("real HTTP path passes the deploy", func(t *testing.T) {
		const serviceKey = "web-good"
		serviceName := appName + "-" + serviceKey
		cleanupContainers(context.Background(), t, runtime, serviceName)
		t.Cleanup(func() { cleanupContainers(context.Background(), t, runtime, serviceName) })

		desired := composeDesiredService(t, appName, serviceKey, res.Tag, "/")
		svcStore := openLiveStore(t)
		saveComposeApp(t, svcStore, appName)
		if err := svcStore.SaveDesiredService(ctx, desired); err != nil {
			t.Fatalf("SaveDesiredService() error = %v", err)
		}

		ctrl := application.New(serviceName, svcStore, runtime, application.WithReadyBudget(20*time.Second))
		result, err := ctrl.Reconcile(ctx)
		if err != nil {
			t.Fatalf("Reconcile() error = %v, result = %+v", err, result)
		}
		if len(result.Conditions) == 0 || result.Conditions[0].Status != "True" || result.Conditions[0].Reason != "Deployed" {
			t.Fatalf("Reconcile() result = %+v, want Status=True Reason=Deployed", result)
		}

		target := application.ContainerName(serviceName, res.Tag, "")
		state, err := runtime.InspectByName(ctx, target)
		if err != nil {
			t.Fatalf("InspectByName(%q) error = %v", target, err)
		}
		if state == nil || !state.Running {
			t.Fatalf("InspectByName(%q) = %+v, want a running container", target, state)
		}
	})
}

// saveComposeApp saves the apps row a compose-translated
// store.DesiredService's AppID foreign key requires (desired_services.app_id
// REFERENCES apps(id), migrations/0039_apps.sql), the same row
// internal/api's handleDeployCompose (apps_compose.go) creates before
// saving any of ToDesiredServices' own output.
func saveComposeApp(t *testing.T, svcStore *store.DB, appName string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if err := svcStore.SaveApp(context.Background(), store.App{ID: appName, Name: appName, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveApp(%q) error = %v", appName, err)
	}
}

// composeDesiredService parses a real compose.yaml document declaring
// one service with a CMD-SHELL curl healthcheck against healthPath, runs
// it through the real compose.ToDesiredServices translation (not a
// hand-built store.ServiceHealth), and returns the single resulting
// store.DesiredService with its image pinned to tag: the compose file
// itself never references a build:, matching internal/compose's own
// "every service needs a pre-built image" contract for the direct-import
// path (compose.go's package doc comment).
func composeDesiredService(t *testing.T, appName, serviceKey, tag, healthPath string) store.DesiredService {
	t.Helper()

	yaml := "services:\n" +
		"  " + serviceKey + ":\n" +
		"    image: \"" + tag + "\"\n" +
		"    ports:\n" +
		"      - \"8080:8080\"\n" +
		"    healthcheck:\n" +
		"      test: [\"CMD-SHELL\", \"curl -f http://localhost:8080" + healthPath + " || exit 1\"]\n" +
		"      interval: 1s\n" +
		"      timeout: 1s\n" +
		"      retries: 2\n"

	f, err := compose.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("compose.Parse() error = %v", err)
	}

	services, warnings, err := compose.ToDesiredServices(appName, f)
	if err != nil {
		t.Fatalf("compose.ToDesiredServices() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("compose.ToDesiredServices() warnings = %v, want none for an HTTP curl healthcheck", warnings)
	}
	if len(services) != 1 {
		t.Fatalf("compose.ToDesiredServices() returned %d services, want 1", len(services))
	}
	desired := services[0]
	if desired.Health == nil || desired.Health.Readiness == nil {
		t.Fatalf("translated desired service Health = %+v, want a populated Readiness probe", desired.Health)
	}
	if desired.Health.Readiness.Path != healthPath {
		t.Fatalf("translated readiness Path = %q, want %q", desired.Health.Readiness.Path, healthPath)
	}
	return desired
}
