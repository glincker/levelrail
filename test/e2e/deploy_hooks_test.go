// TestDeployHooks_Live_PreAndPostDeployHooksRunInsideRealContainer is this
// feature's own whole-chain proof, the same shape
// TestDeploy_Live_BuildToHTTPS establishes: a real build, a real
// application.Controller, and a real Docker container. What it
// specifically proves that the reconciler's own fake-Docker-client unit
// tests (internal/reconcile/application/controller_hooks_test.go) can't:
// the pre/post-deploy hook commands actually execute inside the real
// container via the real Docker Engine API exec facility, not a
// simulated one, by having each hook write a marker file this test then
// reads back via a second, independent Exec call.
package e2e

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDeployHooks_Live_PreAndPostDeployHooksRunInsideRealContainer(t *testing.T) {
	env := newLiveBuildEnv(t)
	dockerCli, buildClient, runtime := env.DockerCli, env.BuildClient, env.Runtime

	const serviceName = "levelrail-test-e2e-hooks"
	repo := "levelrail/test-e2e-hooks"
	tag := repo + ":e2ehooks1"

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = dockerCli.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})
	cleanupContainers(context.Background(), t, runtime, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, runtime, serviceName) })

	svcStore := openLiveStore(t)

	buildCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	res, err := buildClient.Build(buildCtx, build.Request{
		ContextDir: "../fixtures/hello-e2e",
		Tag:        tag,
	}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	desired := store.DesiredService{
		Name:  serviceName,
		Image: res.Tag,
		Port:  8080,
		Health: &store.ServiceHealth{
			Readiness: &store.ServiceProbe{Path: "/"},
		},
		Hooks: &store.ServiceHooks{
			PreDeploy:  "echo pre-hook-output && echo pre-hook-ran > /tmp/pre-marker",
			PostDeploy: "echo post-hook-output && echo post-hook-ran > /tmp/post-marker",
		},
	}
	if err := svcStore.SaveDesiredService(buildCtx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	// A real application.Controller, wired with a real HookRunRecorder
	// (svcStore itself, *store.DB satisfies HookRunRecorder structurally,
	// the same way it satisfies every other narrow interface this
	// controller depends on): this proves the whole chain, exec plus
	// persistence, not just exec in isolation.
	ctrl := application.New(serviceName, svcStore, runtime, application.WithHookRunRecorder(svcStore))
	result, err := ctrl.Reconcile(buildCtx)
	if err != nil {
		t.Fatalf("Controller.Reconcile() error = %v, result = %+v", err, result)
	}
	if len(result.Conditions) == 0 || result.Conditions[0].Status != "True" || result.Conditions[0].Reason != "Deployed" {
		t.Fatalf("Controller.Reconcile() result = %+v, want a True/Deployed Ready condition", result)
	}

	containerName := application.ContainerName(serviceName, res.Tag, "")
	state, err := runtime.InspectByName(buildCtx, containerName)
	if err != nil {
		t.Fatalf("InspectByName() error = %v", err)
	}
	if state == nil || !state.Running {
		t.Fatalf("InspectByName() = %+v, want a running container", state)
	}

	// Prove the hooks actually ran inside this real container: read back
	// the marker files each hook command wrote, via a second, independent
	// Exec call this test drives itself.
	preMarker := execCatOrFail(buildCtx, t, runtime, state.ID, "/tmp/pre-marker")
	if preMarker != "pre-hook-ran\n" {
		t.Errorf("/tmp/pre-marker = %q, want %q", preMarker, "pre-hook-ran\n")
	}
	postMarker := execCatOrFail(buildCtx, t, runtime, state.ID, "/tmp/post-marker")
	if postMarker != "post-hook-ran\n" {
		t.Errorf("/tmp/post-marker = %q, want %q", postMarker, "post-hook-ran\n")
	}

	// Prove the outcome was persisted through the real store, not just
	// executed: GET /api/v1/apps/{name}/hook-runs (internal/api/apps_hooks.go)
	// reads through this exact same method.
	runs, err := svcStore.GetHookRuns(buildCtx, serviceName)
	if err != nil {
		t.Fatalf("GetHookRuns() error = %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("GetHookRuns() = %d rows, want 2", len(runs))
	}
	byType := make(map[string]store.HookRun, len(runs))
	for _, run := range runs {
		byType[run.HookType] = run
	}
	pre, ok := byType[store.HookTypePreDeploy]
	if !ok || !pre.Success || pre.Output != "pre-hook-output\n" {
		t.Errorf("pre_deploy run = %+v, want a successful run with output %q", pre, "pre-hook-output\n")
	}
	post, ok := byType[store.HookTypePostDeploy]
	if !ok || !post.Success || post.Output != "post-hook-output\n" {
		t.Errorf("post_deploy run = %+v, want a successful run with output %q", post, "post-hook-output\n")
	}
}

// execCatOrFail runs "cat path" inside containerID via the real
// docker.Runtime.Exec and returns its stdout, failing the test on any
// exec-transport failure or nonzero exit (a missing marker file, in
// practice, meaning the hook never actually wrote it).
func execCatOrFail(ctx context.Context, t *testing.T, runtime docker.Runtime, containerID, path string) string {
	t.Helper()
	rc, err := runtime.Exec(ctx, containerID, []string{"cat", path})
	if err != nil {
		t.Fatalf("Exec(cat %s) error = %v", path, err)
	}
	defer func() { _ = rc.Close() }()

	out, readErr := io.ReadAll(rc)
	var execErr *docker.ExecExitError
	if errors.As(readErr, &execErr) {
		t.Fatalf("cat %s exited %d: %s", path, execErr.ExitCode, execErr.Stderr)
	} else if readErr != nil {
		t.Fatalf("read exec output for cat %s: %v", path, readErr)
	}
	return string(out)
}
