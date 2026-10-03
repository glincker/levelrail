// This file closes the log archive feature's e2e gap: archive_test.go
// proves the archiver mechanism with fakes, this proves the real HTTP
// API an operator drives (dump, poll, list, download) and that a real
// container's real stdout survives the whole round trip. The bucket is
// still objectstoretest's fake S3 server, since S3 wire compatibility
// isn't what either test exists to prove.
package e2e

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dockersdk "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/strslice"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/netguard"
	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/objectstore/objectstoretest"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	e2eLogArchiveAdminUsername = "e2e-log-archive-admin"
	// e2eLogArchiveAdminPassword is a fixture credential for this test's
	// own throwaway store.DB, never a real secret.
	e2eLogArchiveAdminPassword = "e2e-log-archive-correct-horse-battery" //nolint:gosec // test fixture credential, not a real secret
)

// TestLogArchive_Live_ContainerLogsToHTTPDownload streams a real
// container's stdout into telemetry, triggers a manual dump over real
// HTTP, then lists and downloads the result, asserting the real log
// line survived. Skips cleanly if Docker isn't reachable.
func TestLogArchive_Live_ContainerLogsToHTTPDownload(t *testing.T) {
	// netguard blocks loopback by default; same opt-in archive_test.go uses.
	t.Setenv(netguard.AllowPrivateEnv, "true")

	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = dockerCli.Ping(pingCtx)
	cancel()
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	rt, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("closing docker.Client: %v", err)
		}
	})

	const appName = "levelrail-test-e2e-log-archive"
	removeMetricsContainerIfExists(t, rt, appName)
	t.Cleanup(func() { removeMetricsContainerIfExists(t, rt, appName) })

	ctx := context.Background()
	const marker = "levelrail-log-archive-e2e-marker-9c41"

	pullReader, err := dockerCli.ImagePull(ctx, "busybox:latest", image.PullOptions{})
	if err != nil {
		t.Fatalf("ImagePull() error = %v", err)
	}
	if _, err := io.Copy(io.Discard, pullReader); err != nil {
		t.Fatalf("reading image pull progress stream: %v", err)
	}
	_ = pullReader.Close()

	created, err := dockerCli.ContainerCreate(ctx,
		&dockersdk.Config{
			Image: "busybox:latest",
			Cmd:   strslice.StrSlice{"sh", "-c", "echo " + marker + "; sleep 15"},
		},
		nil, nil, nil, appName,
	)
	if err != nil {
		t.Fatalf("ContainerCreate() error = %v", err)
	}
	if err := rt.Start(ctx, created.ID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := dockerCli.Close(); err != nil {
		t.Errorf("closing docker ping/create client: %v", err)
	}

	// Step 1: the container's real stdout, collected by the real
	// telemetry.LogCollector into a real (temp-file) telemetry store,
	// under the same "service:<app>" resource ID
	// internal/api.resourceIDForApp and objectstore.ResourceID both use.
	resourceID := objectstore.ResourceID(appName)
	telemetryDB := openLogArchiveTelemetryStore(t)
	collector := telemetry.NewLogCollector(rt, telemetryDB, nil, discardTestLogger())
	streamCtx, cancelStream := context.WithTimeout(ctx, 8*time.Second)
	err = collector.StreamOne(streamCtx, telemetry.LogTarget{ResourceID: resourceID, ContainerID: created.ID})
	cancelStream()
	if err != nil && streamCtx.Err() == nil {
		t.Fatalf("StreamOne() error = %v", err)
	}

	// Step 2: a real control-plane store with the app registered
	// (handleLogArchiveDump 404s on an app it doesn't know about), a
	// real envelope-encrypting secrets.Manager, and a real *api.Router
	// wired with api.WithStorage the same way cmd/levelrail/log_archive.go
	// wires it in production, except the bucket is objectstoretest's fake
	// S3 server instead of a real provider.
	svcStore := openMetricsLiveStore(t)
	if err := svcStore.SaveDesiredService(ctx, store.DesiredService{Name: appName, Image: "busybox:latest"}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	masterKey, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(svcStore, masterKey)

	bucketSrv := objectstoretest.New("log-archive-e2e")
	t.Cleanup(bucketSrv.Close)

	resolver := &objectstore.Resolver{Store: svcStore, Secrets: secretsManager}
	archiveOpts := objectstore.DefaultOptions()
	archiver := objectstore.NewArchiver(svcStore, resolver, telemetryDB, discardTestLogger(), archiveOpts)

	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(logger, b, svcStore,
		api.WithBackupSecrets(secretsManager),
		api.WithStorage(api.StorageDeps{
			Options: svcStore, Archive: svcStore, Clients: resolver, Archiver: archiver, ArchiveRoot: archiveOpts.Prefix,
		}),
	)
	ts := httptest.NewServer(router.Handler())
	t.Cleanup(ts.Close)

	if err := api.BootstrapAdmin(ctx, svcStore, e2eLogArchiveAdminUsername, e2eLogArchiveAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	client := &http.Client{Jar: jar, Timeout: e2eHTTPTimeout}

	loginBody := fmt.Sprintf(`{"username":%q,"password":%q}`, e2eLogArchiveAdminUsername, e2eLogArchiveAdminPassword)
	status, body := postJSON(t, client, ts.URL+"/api/v1/auth/login", loginBody)
	if status != http.StatusOK {
		t.Fatalf("login: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}

	// Step 3: a real storage destination, created over real HTTP,
	// pointed at the fake bucket server. verify is left off: the probe
	// path (verifyStorageRequest) builds its own objectstore.Client
	// without the netguard override this test relies on, so it would
	// fail against a loopback endpoint regardless of whether the
	// destination itself works.
	createDestBody := fmt.Sprintf(`{"name":"e2e-log-archive","preset":"custom","endpoint":%q,"region":"us-east-1","bucket":%q,"access_key_id":"test-access","secret_access_key":"test-secret"}`,
		bucketSrv.URL, bucketSrv.Bucket)
	status, body = postJSON(t, client, ts.URL+"/api/v1/storage/destinations", createDestBody)
	if status != http.StatusCreated {
		t.Fatalf("create storage destination: status = %d, want %d, body = %s", status, http.StatusCreated, body)
	}
	var destResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &destResp); err != nil || destResp.ID == "" {
		t.Fatalf("decode storage destination response %q: %v", body, err)
	}

	// Step 4: the real trigger this test exists to prove, a manual dump
	// over real HTTP, not a direct call into objectstore.Archiver.
	from := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
	dumpBody := fmt.Sprintf(`{"app_name":%q,"target_id":%q,"from":%q,"to":%q}`, appName, destResp.ID, from, to)
	status, body = postJSON(t, client, ts.URL+"/api/v1/log-archive/dump", dumpBody)
	if status != http.StatusAccepted {
		t.Fatalf("trigger log archive dump: status = %d, want %d, body = %s", status, http.StatusAccepted, body)
	}
	var runResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &runResp); err != nil || runResp.ID == "" {
		t.Fatalf("decode log archive run response %q: %v", body, err)
	}

	waitLogArchiveRunSucceeded(t, client, ts.URL, appName, runResp.ID, 30*time.Second)

	// Step 5: list the archived objects over real HTTP.
	listURL := ts.URL + "/api/v1/log-archive/objects?" + url.Values{"target_id": {destResp.ID}, "app": {appName}}.Encode()
	body, status, err = doGet(client, listURL)
	if err != nil || status != http.StatusOK {
		t.Fatalf("list archive objects: status = %d, err = %v, body = %s", status, err, body)
	}
	var listResp struct {
		Objects []struct {
			Key string `json:"key"`
		} `json:"objects"`
	}
	if err := json.Unmarshal([]byte(body), &listResp); err != nil {
		t.Fatalf("decode archive objects response %q: %v", body, err)
	}
	if len(listResp.Objects) == 0 {
		t.Fatalf("list archive objects = %+v, want at least one archived object", listResp)
	}

	// Step 6: the real proof. Download the archived object over real
	// HTTP, decompress it, and find the container's actual log line.
	downloadURL := ts.URL + "/api/v1/log-archive/objects/download?" + url.Values{"target_id": {destResp.ID}, "key": {listResp.Objects[0].Key}}.Encode()
	downloadResp, err := client.Get(downloadURL) //nolint:noctx // test helper, ts.URL is loopback-only
	if err != nil {
		t.Fatalf("download archive object: %v", err)
	}
	defer func() {
		if err := downloadResp.Body.Close(); err != nil {
			t.Errorf("closing download response body: %v", err)
		}
	}()
	if downloadResp.StatusCode != http.StatusOK {
		t.Fatalf("download archive object: status = %d, want %d", downloadResp.StatusCode, http.StatusOK)
	}
	raw := decompressArchiveObject(t, downloadResp.Body)
	if !strings.Contains(raw, marker) {
		t.Errorf("archived object does not contain the container's real log line %q; got: %s", marker, raw)
	}
	if !strings.Contains(raw, resourceID) {
		t.Errorf("archived object does not carry its resource %q; got: %s", resourceID, raw)
	}
}

func openLogArchiveTelemetryStore(t *testing.T) *telemetry.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "telemetry.db")
	db, err := telemetry.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("telemetry.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// decompressArchiveObject gunzips a downloaded archive object into a string.
func decompressArchiveObject(t *testing.T, r io.Reader) string {
	t.Helper()
	gz, err := gzip.NewReader(r)
	if err != nil {
		t.Fatalf("gunzip archive object: %v", err)
	}
	defer func() { _ = gz.Close() }()
	raw, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("read decompressed archive object: %v", err)
	}
	return string(raw)
}

func waitLogArchiveRunSucceeded(t *testing.T, client *http.Client, baseURL, appName, runID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		body, status, err := doGet(client, baseURL+"/api/v1/log-archive/runs?app="+appName)
		if err == nil && status == http.StatusOK {
			var runs []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Error  string `json:"error"`
			}
			if json.Unmarshal([]byte(body), &runs) == nil {
				for _, r := range runs {
					if r.ID != runID {
						continue
					}
					switch r.Status {
					case "succeeded":
						return
					case "failed":
						t.Fatalf("log archive run %q failed: %s", runID, r.Error)
					}
				}
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("log archive run %q did not succeed within %v", runID, timeout)
}
