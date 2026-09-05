package e2e

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestEnv_Live_PlainAndSecretResolveInContainer is this repo's first
// test/e2e coverage of internal/secrets.Manager (TASKS.md 1.7): every
// other live test under this package (deploy_test.go) deploys a service
// with plain, literal env only. This test proves the secret-backed half
// of the same env-injection path composes correctly end to end: a real
// build of test/fixtures/hello-e2e, a real secrets.Manager backed by a
// real (generated-for-this-test) master key and the real store, a real
// application.Controller wired with that Manager as its SecretResolver,
// and, independently of the controller's own returned Result, a raw
// Docker Engine API inspect of the resulting container's actual env.
// Asserts: the plain var is present verbatim, the secret var is present
// as its resolved plaintext (proving decryption actually happened at
// container-create time), and the secret var's value is neither the
// ciphertext this test's own secrets.Manager persisted nor anything
// other than the exact plaintext.
//
// What this deliberately does not prove: it does not exercise
// internal/deploy or internal/spec's app.yaml -> { secret: true } parsing
// (see internal/deploy/translate.go's secretEnvNames, which is unit
// tested there); this test constructs store.DesiredService directly, the
// same "skip spec parsing, prove the reconcile chain" choice
// deploy_test.go already makes. It does not exercise ingress or HTTPS,
// unlike deploy_test.go's full build-to-HTTPS chain: the thing being
// proven here is entirely about container env, so a raw container
// inspect is the strongest and most direct assertion available, and
// adding Caddy on top would only add moving parts without adding proof.
// It does not exercise secret rotation, deletion, or multiple secrets
// per service.
//
// Skips cleanly, does not fail, if Docker or BuildKit aren't reachable,
// matching deploy_test.go's exact skip pattern.
func TestEnv_Live_PlainAndSecretResolveInContainer(t *testing.T) {
	env := newLiveBuildEnv(t)
	dockerCli, buildClient, runtime := env.DockerCli, env.BuildClient, env.Runtime

	// Name distinct from every other e2e test's service name
	// (deploy_test.go uses levelrail-test-e2e-deploy): other agents are
	// concurrently adding sibling e2e tests against the same shared local
	// Docker daemon, so a collision here would break both tests.
	const serviceName = "levelrail-test-e2e-env-secrets"
	repo := "levelrail/test-e2e-env-secrets"
	sha := "e2elivetestenv1"
	tag := repo + ":" + sha

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

	// Step 1: a real build via internal/build, the same fixture and call
	// shape deploy_test.go already uses.
	res, err := buildClient.Build(buildCtx, build.Request{
		ContextDir: "../fixtures/hello-e2e",
		Tag:        tag,
	}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if res.Tag != tag {
		t.Fatalf("Build() returned tag %q, want %q", res.Tag, tag)
	}

	// Step 2: a real secrets.Manager, generated master key and the real
	// store, and a real SetValue call, so SECRET_VAR has real ciphertext
	// and a real wrapped DEK persisted before reconcile ever runs. This
	// mirrors internal/reconcile/application/secrets_live_test.go's setup,
	// the only other live test exercising this path today, one level
	// below this package.
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	manager := secrets.NewManager(svcStore, mk)

	const plainValue = "plain-value"
	const secretPlaintext = "super-secret-value"
	if err := manager.SetValue(buildCtx, serviceName, "SECRET_VAR", secretPlaintext); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}

	// Step 3: save desired state with both a literal env var and a
	// secret-backed one, matching the app spec's env shape
	// (env: { PLAIN: value } alongside env: { SECRET: { secret: true } })
	// one level down, past internal/deploy/translate.go's parsing, the
	// same "construct store.DesiredService directly" choice
	// deploy_test.go already documents making.
	desired := store.DesiredService{
		Name:      serviceName,
		Image:     res.Tag,
		Port:      8080,
		Env:       map[string]string{"PLAIN_VAR": plainValue},
		SecretEnv: []string{"SECRET_VAR"},
		Health: &store.ServiceHealth{
			Readiness: &store.ServiceProbe{Path: "/"},
		},
	}
	if err := svcStore.SaveDesiredService(buildCtx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	// Step 4: a real application.Controller, wired with the real
	// secrets.Manager as its SecretResolver, converges a real container.
	appCtrl := application.New(serviceName, svcStore, runtime, application.WithSecretResolver(manager))
	result, err := appCtrl.Reconcile(buildCtx)
	if err != nil {
		t.Fatalf("application Controller.Reconcile() error = %v, result = %+v", err, result)
	}
	if len(result.Conditions) == 0 || result.Conditions[0].Status != "True" {
		t.Fatalf("application Controller.Reconcile() result = %+v, want a True Ready condition", result)
	}

	// Step 5: independent verification via the raw Docker Engine API,
	// not this controller's own Result and not internal/docker's
	// ContainerState (which does not expose env at all), the same rigor
	// internal/reconcile/application/secrets_live_test.go applies. This is
	// the assertion that actually matters: what the running container's
	// real environment contains.
	target := application.ContainerName(serviceName, res.Tag, "")
	inspect, err := dockerCli.ContainerInspect(buildCtx, target)
	if err != nil {
		t.Fatalf("raw ContainerInspect(%q) error = %v", target, err)
	}

	if !slices.Contains(inspect.Config.Env, "PLAIN_VAR="+plainValue) {
		t.Errorf("container env = %v, want it to contain PLAIN_VAR=%s", inspect.Config.Env, plainValue)
	}

	// Collect every entry keyed SECRET_VAR (there must be exactly one)
	// and assert its value is exactly the resolved plaintext, nothing
	// else: not the base64/binary ciphertext this test's own
	// secrets.Manager persisted, not a secret-reference string like
	// "secret://levelrail-test-e2e-env-secrets/SECRET_VAR", not a
	// truncated or re-encoded form. An exact-match slices.Contains check
	// alone would pass by coincidence if, say, the ciphertext happened to
	// decode to a printable string containing "super-secret-value" as a
	// substring; scanning every SECRET_VAR= entry directly rules that
	// out.
	var secretEntries []string
	for _, kv := range inspect.Config.Env {
		if strings.HasPrefix(kv, "SECRET_VAR=") {
			secretEntries = append(secretEntries, kv)
		}
	}
	if len(secretEntries) != 1 {
		t.Fatalf("container env has %d entries for SECRET_VAR, want exactly 1: %v", len(secretEntries), secretEntries)
	}
	want := "SECRET_VAR=" + secretPlaintext
	if secretEntries[0] != want {
		t.Errorf("container env SECRET_VAR entry = %q, want exactly %q (the resolved plaintext, nothing else)", secretEntries[0], want)
	}

	// Belt and suspenders against the same coincidence, from the other
	// direction: fetch what actually landed on disk for this key and
	// confirm the container's env value is not that ciphertext, so a
	// regression that accidentally passed ciphertext straight through
	// (skipping Manager.Resolve entirely) would be caught even if the
	// prefix-scan above were somehow fooled.
	ciphertext, err := svcStore.GetSecretValue(buildCtx, serviceName, "SECRET_VAR")
	if err != nil {
		t.Fatalf("GetSecretValue() error = %v", err)
	}
	if secretEntries[0] == "SECRET_VAR="+string(ciphertext) {
		t.Fatal("container env SECRET_VAR entry equals the stored ciphertext verbatim, decryption did not happen")
	}
}
