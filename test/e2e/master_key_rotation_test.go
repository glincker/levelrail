// TestMasterKeyRotation_Live_SecretStillResolvesAfterRotation is this
// package's live proof for internal/secrets' master key rotation
// (rotate.go, manager.go's RotateMasterKey, shipped in "feat: master key
// rotation for envelope-encrypted secrets"). internal/secrets/rotate_test.go
// and internal/api/master_key_rotation_test.go already prove, respectively,
// that RotateStoredDEKs re-wraps a DEK correctly in isolation and that
// the HTTP handler returns the right status/response shapes against a
// fake rotator. Neither proves the end-to-end operator promise: that a
// secret set before rotation still resolves to its exact original
// plaintext inside a real, freshly (re)created container, driven through
// a real HTTP rotation call, after the master key has genuinely changed.
// test/e2e/env_test.go's own doc comment explicitly names rotation as
// out of scope for that test; this file closes that gap.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eRotationAdminUsername = "e2e-rotation-admin"
	e2eRotationAdminPassword = "e2e-rotation-correct-horse" //nolint:gosec // test fixture credential, not a real secret
)

func TestMasterKeyRotation_Live_SecretStillResolvesAfterRotation(t *testing.T) {
	env := newLiveBuildEnv(t)
	dockerCli, buildClient, runtime := env.DockerCli, env.BuildClient, env.Runtime

	const serviceName = "levelrail-test-e2e-key-rotation"
	repo := "levelrail/test-e2e-key-rotation"
	tag := repo + ":e2ekeyrotation1"

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = dockerCli.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})

	cleanupContainers(context.Background(), t, runtime, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, runtime, serviceName) })

	svcStore := openLiveStore(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	res, err := buildClient.Build(ctx, build.Request{ContextDir: "../fixtures/hello-e2e", Tag: tag}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Step 1: a real secrets.Manager under a real, freshly generated
	// master key, a real secret set through it, matching
	// test/e2e/env_test.go's own setup exactly.
	oldKey, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	manager := secrets.NewManager(svcStore, oldKey)

	const secretPlaintext = "rotate-me-super-secret"
	if err := manager.SetValue(ctx, serviceName, "API_KEY", secretPlaintext); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}

	desired := store.DesiredService{
		Name:      serviceName,
		Image:     res.Tag,
		Port:      8080,
		SecretEnv: []string{"API_KEY"},
		Health: &store.ServiceHealth{
			Readiness: &store.ServiceProbe{Path: "/"},
		},
	}
	if err := svcStore.SaveDesiredService(ctx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	appCtrl := application.New(serviceName, svcStore, runtime, application.WithSecretResolver(manager))
	target := application.ContainerName(serviceName, res.Tag, "")

	// Step 2: deploy once, under the pre-rotation key, and prove the
	// real container's real env has the real plaintext, the same rigor
	// env_test.go applies.
	if result, err := appCtrl.Reconcile(ctx); err != nil {
		t.Fatalf("pre-rotation Reconcile() error = %v, result = %+v", err, result)
	}
	assertContainerHasSecretEnv(ctx, t, dockerCli, target, secretPlaintext)

	ciphertextBefore, err := svcStore.GetSecretValue(ctx, serviceName, "API_KEY")
	if err != nil {
		t.Fatalf("GetSecretValue() before rotation error = %v", err)
	}

	// Step 3: a real *api.Router with rotation wired to the same real
	// Manager, a real HTTP server, a real authenticated session, and a
	// real POST to the actual rotation route (not calling
	// manager.RotateMasterKey directly): this is the operator-facing
	// path the whole feature exists to expose.
	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	keyPath := filepath.Join(t.TempDir(), "master.key")
	router := api.NewRouter(logger, b, svcStore, api.WithMasterKeyRotation(manager, keyPath))
	ts := newE2ETestServer(t, router)

	if err := api.BootstrapAdmin(ctx, svcStore, e2eRotationAdminUsername, e2eRotationAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eRotationAdminUsername, e2eRotationAdminPassword)

	newKey, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() for new key error = %v", err)
	}

	status, body := postJSON(t, client, ts.URL+"/api/v1/system/master-key/rotate", `{"newMasterKey":"`+newKey.String()+`"}`)
	if status != http.StatusOK {
		t.Fatalf("rotate: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	var rotateResp struct {
		RotatedAt       time.Time `json:"rotatedAt"`
		PersistedToFile bool      `json:"persistedToFile"`
		Warning         string    `json:"warning"`
	}
	if err := json.Unmarshal([]byte(body), &rotateResp); err != nil {
		t.Fatalf("decode rotate response %s: %v", body, err)
	}
	if !rotateResp.PersistedToFile {
		t.Errorf("rotate response PersistedToFile = false, want true (a master key file path was configured)")
	}
	if rotateResp.RotatedAt.IsZero() {
		t.Error("rotate response RotatedAt is zero, want a real timestamp")
	}

	// Step 4: re-wrap, not re-encrypt. The stored ciphertext bytes for
	// this exact secret must be byte-identical before and after
	// rotation: only the DEK's own wrapping key changed.
	ciphertextAfter, err := svcStore.GetSecretValue(ctx, serviceName, "API_KEY")
	if err != nil {
		t.Fatalf("GetSecretValue() after rotation error = %v", err)
	}
	if string(ciphertextAfter) != string(ciphertextBefore) {
		t.Error("stored secret ciphertext changed across rotation, want it untouched (only the DEK wrapping should change)")
	}

	// Step 5: force a fresh container creation under the now-rotated
	// Manager (the same *manager value RotateMasterKey mutated in
	// place), the real proof that Resolve genuinely still decrypts the
	// exact original plaintext post-rotation, not merely that the
	// ciphertext bytes look unchanged.
	state, err := runtime.InspectByName(ctx, target)
	if err != nil {
		t.Fatalf("InspectByName(%q) error = %v", target, err)
	}
	if state == nil {
		t.Fatalf("InspectByName(%q) = nil, want the pre-rotation container to still exist", target)
	}
	if err := runtime.Stop(ctx, state.ID, 3*time.Second); err != nil {
		t.Fatalf("Stop(%q) error = %v", state.ID, err)
	}
	if err := runtime.Remove(ctx, state.ID, true); err != nil {
		t.Fatalf("Remove(%q) error = %v", state.ID, err)
	}

	if result, err := appCtrl.Reconcile(ctx); err != nil {
		t.Fatalf("post-rotation Reconcile() error = %v, result = %+v", err, result)
	}
	assertContainerHasSecretEnv(ctx, t, dockerCli, target, secretPlaintext)
}

// assertContainerHasSecretEnv inspects target through the raw Docker
// Engine API, bypassing internal/docker's ContainerState entirely (it
// does not expose env), and asserts exactly one API_KEY= entry exists
// with wantPlaintext as its value: the same rigor
// test/e2e/env_test.go's TestEnv_Live_PlainAndSecretResolveInContainer
// already applies to its own SECRET_VAR assertion.
func assertContainerHasSecretEnv(ctx context.Context, t *testing.T, dockerCli *dockerclient.Client, target, wantPlaintext string) {
	t.Helper()
	inspect, err := dockerCli.ContainerInspect(ctx, target)
	if err != nil {
		t.Fatalf("raw ContainerInspect(%q) error = %v", target, err)
	}

	var matches []string
	for _, kv := range inspect.Config.Env {
		if strings.HasPrefix(kv, "API_KEY=") {
			matches = append(matches, kv)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("container env has %d entries for API_KEY, want exactly 1: %v", len(matches), matches)
	}
	want := "API_KEY=" + wantPlaintext
	if matches[0] != want {
		t.Errorf("container env API_KEY entry = %q, want exactly %q", matches[0], want)
	}
}
