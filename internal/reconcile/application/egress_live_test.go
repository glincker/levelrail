package application

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestEgress_Live_BlocksDisallowedAllowsAllowed is the real end-to-end
// proof for both bugs fixed here: a real app container, a real egress
// sidecar built from egressImage/egressBootScript, converged by a real
// application.Controller against a real Docker daemon.
//
// It proves, against real Docker rather than the fake runtime:
//  1. EgressPolicyReady only ever reports PoliciesApplied once the
//     sidecar's own marker file actually exists (checked independently
//     here via a real Exec, not just by trusting Reconcile's return
//     value), the Bug 1 fix.
//  2. The sidecar reaches that state with no apk/package-manager step at
//     boot: egressImage ships iptables/ipset/getent preinstalled, the
//     Bug 2 fix.
//  3. Once applied, a request to an allowed host succeeds and a request
//     to a disallowed host is actually dropped, from inside the app
//     container's own network namespace (exec'd via the sidecar, which
//     shares that namespace).
//
// Skips cleanly if Docker or the network isn't reachable, matching this
// package's other live tests.
func TestEgress_Live_BlocksDisallowedAllowsAllowed(t *testing.T) {
	dockertest.SkipIfShort(t)
	rt, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	rawCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if _, err := rawCli.Ping(pingCtx); err != nil {
		cancel()
		t.Skipf("docker daemon not reachable: %v", err)
	}
	cancel()
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("closing docker client: %v", err)
		}
	})

	const serviceName = "levelrail-test-egress-live"
	image1 := "nginx:alpine"

	longCtx := context.Background()
	if err := pullIfMissing(longCtx, t, rawCli); err != nil {
		t.Fatalf("pull %s: %v", image1, err)
	}
	if err := pullRefIfMissing(longCtx, rawCli, egressImage); err != nil {
		t.Skipf("pull %s: %v (egress sidecar image unreachable, skipping)", egressImage, err)
	}

	cleanupContainers(longCtx, t, rt, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, rt, serviceName) })

	db := openLiveStore(t)

	desired := store.DesiredService{
		Name: serviceName, Image: image1, Port: 80,
		Egress: &store.ServiceEgressPolicy{
			Mode: store.EgressModeAllowlist,
			Allow: []store.ServiceEgressAllow{
				{Host: "one.one.one.one", Port: 443}, // Cloudflare's own resolver hostname, stable and always reachable
			},
		},
	}
	if err := db.SaveDesiredService(longCtx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	ctrl := New(serviceName, db, rt, WithEgressReadyBudget(45*time.Second), WithEgressReadyPollInterval(500*time.Millisecond))

	target := ContainerName(serviceName, image1, "")
	sidecarName := egressSidecarName(target)

	// Reconcile repeatedly (the real reconcile loop is event/poll driven;
	// this stands in for that) until EgressPolicyReady goes True, with a
	// bounded overall wait so a genuine regression fails the test instead
	// of hanging it.
	deadline := time.Now().Add(60 * time.Second)
	var lastResult string
	for {
		result, err := ctrl.Reconcile(longCtx)
		if err != nil {
			t.Fatalf("Reconcile() error = %v, result = %+v", err, result)
		}
		if len(result.Conditions) < 2 {
			t.Fatalf("Reconcile() result = %+v, want at least 2 conditions (Ready, EgressPolicyReady)", result)
		}
		cond := result.Conditions[1]
		lastResult = string(cond.Status) + "/" + cond.Reason + ": " + cond.Message

		if cond.Status == "True" && cond.Reason == "PoliciesApplied" {
			// Independent verification: don't just trust the condition,
			// confirm the marker is actually there via a fresh real Exec,
			// exactly what a not-yet-fixed reconciler could never have
			// proven at this point (it would have said this on the very
			// first pass, sidecar image pull included, before the boot
			// script could possibly have finished).
			sidecarState, err := rt.InspectByName(longCtx, sidecarName)
			if err != nil {
				t.Fatalf("InspectByName(%q) error = %v", sidecarName, err)
			}
			if sidecarState == nil || !sidecarState.Running {
				t.Fatalf("condition reported PoliciesApplied but sidecar %q is not running: %+v", sidecarName, sidecarState)
			}
			ready, err := egressMarkerPresent(longCtx, rt, sidecarState.ID)
			if err != nil {
				t.Fatalf("independent marker check error = %v", err)
			}
			if !ready {
				t.Fatalf("Reconcile() reported PoliciesApplied but egressReadyMarkerPath is NOT present in %q: this is exactly the false-enforced bug", sidecarName)
			}
			break
		}
		if cond.Reason == "PolicyApplyFailed" {
			t.Fatalf("EgressPolicyReady = %+v, want it to eventually reach PoliciesApplied, not fail", cond)
		}
		if time.Now().After(deadline) {
			t.Fatalf("EgressPolicyReady never reached PoliciesApplied within 60s, last = %s", lastResult)
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("EgressPolicyReady reached PoliciesApplied: %s", lastResult)

	sidecarState, err := rt.InspectByName(longCtx, sidecarName)
	if err != nil || sidecarState == nil {
		t.Fatalf("InspectByName(%q) error = %v", sidecarName, err)
	}

	// Allowed host: a real HTTPS request from inside the sidecar's netns
	// (shared with the app container) must succeed.
	if out, err := execIn(longCtx, rt, sidecarState.ID, []string{"wget", "-T", "5", "-qO-", "https://one.one.one.one"}); err != nil {
		t.Errorf("request to allowlisted host failed, want success: %v (output: %q)", err, out)
	}

	// Disallowed host: a real HTTPS request to a host never in the
	// allowlist must be dropped by the OUTPUT policy, not merely slow.
	blockCtx, blockCancel := context.WithTimeout(longCtx, 8*time.Second)
	_, err = execIn(blockCtx, rt, sidecarState.ID, []string{"wget", "-T", "5", "-qO-", "https://example.com"})
	blockCancel()
	if err == nil {
		t.Error("request to a non-allowlisted host succeeded, want it blocked by the egress DROP policy")
	} else {
		t.Logf("request to non-allowlisted host correctly failed: %v", err)
	}
}

// egressMarkerPresent is a standalone, test-local re-implementation of
// egressRulesInstalled's Exec-based check, deliberately not calling the
// controller's own method: the point of this live test is to verify the
// real container state independently of the code path under test.
func egressMarkerPresent(ctx context.Context, rt docker.Runtime, sidecarID string) (bool, error) {
	rc, err := rt.Exec(ctx, sidecarID, []string{"test", "-f", egressReadyMarkerPath})
	if err != nil {
		return false, err
	}
	defer func() { _ = rc.Close() }()
	_, readErr := io.Copy(io.Discard, rc)
	var execErr *docker.ExecExitError
	switch {
	case readErr == nil:
		return true, nil
	case errors.As(readErr, &execErr):
		return false, nil
	default:
		return false, readErr
	}
}

// execIn runs cmd inside containerID and returns its stdout, or an error
// if the exec transport failed or cmd exited nonzero
// (docker.ExecExitError).
func execIn(ctx context.Context, rt docker.Runtime, containerID string, cmd []string) (string, error) {
	rc, err := rt.Exec(ctx, containerID, cmd)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	out, readErr := io.ReadAll(rc)
	return string(out), readErr
}

// pullRefIfMissing pulls ref if not already present locally, the
// egressImage counterpart to this file's own pullIfMissing (which is
// hardcoded to nginx:alpine, this package's one other live-test fixture
// image).
func pullRefIfMissing(ctx context.Context, cli *dockerclient.Client, ref string) error {
	if _, err := cli.ImageInspect(ctx, ref); err == nil {
		return nil
	}
	rc, err := cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(io.Discard, rc)
	return err
}
