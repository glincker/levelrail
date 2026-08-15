package e2e

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// TestWebhook_Live_PushToRunningContainer is the whole-chain proof for
// the path cmd/levelrail/main.go's loadWebhookHandler wires up: a real,
// HMAC-signed GitHub push payload, verified and dispatched by a real
// webhook.Handler, fetching a real local git repo at the pushed SHA
// (go-git resolves a plain filesystem path exactly like any other
// remote, no network dependency, the same reasoning
// internal/webhook/clone_live_test.go already documents), building it
// for real via internal/build, saving real desired state, and a real
// application.Controller converging a real running container from it.
// This is the one leg of the deploy chain TestDeploy_Live_BuildToHTTPS
// in this same package doesn't cover: that test calls internal/build and
// internal/deploy directly, this one starts from an actual push event
// the way GitHub would send one.
func TestWebhook_Live_PushToRunningContainer(t *testing.T) {
	rawCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = rawCli.Ping(pingCtx)
	cancel()
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	buildClient, err := build.NewClient(connectCtx, rawCli)
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

	const serviceName = "levelrail-test-webhook-push"
	const imageRepo = "levelrail/test-webhook-push"
	const helloBody = "hello from levelrail webhook e2e"

	cleanupContainers(context.Background(), t, runtime, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, runtime, serviceName) })

	// A real local repo, built by go-git itself, the same pattern
	// internal/webhook/clone_live_test.go uses: webhook.Config.RepoURL
	// treats a filesystem path exactly like any other git remote.
	repoDir := t.TempDir()
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("PlainInit() error = %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() error = %v", err)
	}
	dockerfile := "FROM busybox:1.36\n" +
		"WORKDIR /srv\n" +
		"RUN echo \"" + helloBody + "\" > index.html\n" +
		"EXPOSE 8080\n" +
		"CMD [\"busybox\", \"httpd\", \"-f\", \"-p\", \"8080\", \"-h\", \"/srv\"]\n"
	if err := writeFile(repoDir, "Dockerfile", dockerfile); err != nil {
		t.Fatalf("write Dockerfile: %v", err)
	}
	if _, err := wt.Add("Dockerfile"); err != nil {
		t.Fatalf("Add Dockerfile: %v", err)
	}
	sig := &object.Signature{Name: "Levelrail Test", Email: "test@example.invalid", When: time.Now()}
	commitHash, err := wt.Commit("add fixture", &git.CommitOptions{Author: sig})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	// Registered before the build even runs, matching every other live
	// test's "clean up before and after" convention in this codebase, so
	// a failure partway through still leaves nothing behind on a re-run.
	builtTag := imageRepo + ":" + commitHash.String()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = rawCli.ImageRemove(cleanupCtx, builtTag, image.RemoveOptions{Force: true})
	})

	db := openLiveStore(t)

	// Real deploy-attempt tracking end to end: a real *telemetry.DB and a
	// real *deploylog.Recorder, not fakes, so this test proves the
	// webhook trigger path actually persists a deploy_attempts row and a
	// real build's log output, not just that the mocked/fake-store unit
	// tests in internal/webhook and internal/api believe it does.
	telemetryPath := filepath.Join(t.TempDir(), "telemetry.db")
	telemetryDB, err := telemetry.Open(context.Background(), telemetryPath)
	if err != nil {
		t.Fatalf("telemetry.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = telemetryDB.Close() })
	recorder := deploylog.NewRecorder(telemetryDB, nil)

	const webhookSecret = "test-webhook-secret" //nolint:gosec // fake fixture, this test's own signature check
	cfg := webhook.Config{
		Secret:      []byte(webhookSecret),
		RepoURL:     repoDir,
		Branch:      "main",
		ServiceName: serviceName,
		Service:     spec.Service{Build: spec.Build{Type: spec.BuildDockerfile}, Port: 8080},
		ImageRepo:   imageRepo,
	}
	pipeline := deploy.New(buildClient, db)
	handler := webhook.New(cfg, pipeline, db, recorder, nil)

	payload, err := json.Marshal(struct {
		Ref   string `json:"ref"`
		After string `json:"after"`
	}{Ref: "refs/heads/main", After: commitHash.String()})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", "sha256="+signHMAC(webhookSecret, payload))
	rec := httptest.NewRecorder()

	deployCtx, deployCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	req = req.WithContext(deployCtx)
	handler.ServeHTTP(rec, req)
	deployCancel()

	if rec.Code != 200 {
		t.Fatalf("webhook response status = %d, body = %s, want 200", rec.Code, rec.Body.String())
	}

	saved, err := db.GetDesiredService(context.Background(), serviceName)
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if saved.Image != builtTag {
		t.Fatalf("saved desired state Image = %q, want %q", saved.Image, builtTag)
	}

	// This task's own real deploy-attempt history: a row for this push,
	// persisted in the real store, marked succeeded, plus at least one
	// real line of BuildKit's own output persisted in the real telemetry
	// store, proving internal/deploylog.Recorder actually received and
	// flushed events from a real build, not a fake progress callback.
	attempts, err := db.ListDeployAttempts(context.Background(), serviceName)
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("expected 1 deploy attempt recorded for this push, got %d", len(attempts))
	}
	if attempts[0].Status != store.DeployAttemptStatusSucceeded {
		t.Errorf("deploy attempt Status = %q, want %q", attempts[0].Status, store.DeployAttemptStatusSucceeded)
	}
	if attempts[0].Image != builtTag {
		t.Errorf("deploy attempt Image = %q, want %q", attempts[0].Image, builtTag)
	}
	if attempts[0].FinishedAt == nil {
		t.Error("deploy attempt FinishedAt = nil, want set")
	}
	logLines, err := telemetryDB.QueryDeployLog(context.Background(), attempts[0].ID)
	if err != nil {
		t.Fatalf("QueryDeployLog() error = %v", err)
	}
	if len(logLines) == 0 {
		t.Error("expected at least one persisted build log line for this real build, got none")
	}

	// This is the actual closure of the loop: a real application
	// controller, pointed at the same store and runtime, reconciling the
	// desired state the webhook just wrote, with no glue code beyond
	// what both packages already independently expose.
	ctrl := application.New(serviceName, db, runtime)
	result, err := ctrl.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("application Controller.Reconcile() error = %v, result = %+v", err, result)
	}
	if len(result.Conditions) == 0 || result.Conditions[0].Status != "True" {
		t.Fatalf("Reconcile() result = %+v, want a True Ready condition", result)
	}

	// Independent verification, not trusting the controller's own
	// return value: the container this whole chain, starting from an
	// HTTP push event, was supposed to produce actually exists, is
	// running, and serves the exact content the pushed commit built.
	containers, err := runtime.ListByPrefix(context.Background(), serviceName+"-")
	if err != nil {
		t.Fatalf("ListByPrefix() error = %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected exactly 1 container for %q after reconcile, got %d: %+v", serviceName, len(containers), containers)
	}
	if !containers[0].Running || len(containers[0].Ports) == 0 {
		t.Fatalf("container %+v is not running with a published port", containers[0])
	}

	client := &http.Client{Timeout: 5 * time.Second}
	addr := "http://127.0.0.1:" + strconv.Itoa(containers[0].Ports[0].HostPort) + "/"
	body := getBodyWithRetry(t, client, addr)
	if body != helloBody+"\n" {
		t.Errorf("response body = %q, want %q", body, helloBody+"\n")
	}
}

func signHMAC(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func writeFile(dir, name, content string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
}
