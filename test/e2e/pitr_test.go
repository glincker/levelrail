// TestPITR_Live_HTTPRoundTrip_RestoresBeforeMarkerNotAfterMarker is this
// package's live, black-box proof for point-in-time restore
// (internal/api/pitr.go, internal/backup's BaseBackupRunner/PITRRunner):
// internal/backup/pitr_live_test.go already proves the core mechanism
// (pg_basebackup, WAL archiving, wipe-and-replay) works against a real
// container by calling those types directly. This test proves the
// thing that matters operationally on top of that: the same round trip
// driven entirely through the real HTTP API, a real cookie-authenticated
// session, a real reconcile.Engine bringing the container up and back
// down around the restore, and a real S3-compatible bucket (MinIO) for
// the base backup's own upload/download leg, none of which the
// internal/backup test exercises. A before-timestamp marker row must
// survive; an after-timestamp one must not.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2ePITRAdminUsername = "e2e-pitr-admin"
	e2ePITRAdminPassword = "e2e-pitr-correct-horse" //nolint:gosec // test fixture credential, not a real secret
)

func TestPITR_Live_HTTPRoundTrip_RestoresBeforeMarkerNotAfterMarker(t *testing.T) {
	env := newLiveBuildEnv(t)
	runtime := env.Runtime

	// A random per-run suffix, not a fixed name: docker.Runtime has no
	// exported RemoveVolume (dataVolumeName's own doc comment,
	// internal/reconcile/database/controller.go, explains why volumes
	// are deliberately never removed in production), so a rerun against
	// a fixed name would attach to a previous run's already-populated
	// data volume instead of a genuinely fresh database, the same fix
	// internal/backup/pitr_live_test.go's own doc comment applies for
	// its identical concern.
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	dbName := "levelrail-test-e2e-pitr-postgres-" + suffix
	target := "db-" + dbName
	minioName := "levelrail-test-e2e-pitr-minio-" + suffix
	const bucket = "pitr-e2e"

	cleanupDB := func() {
		ctx := context.Background()
		if state, err := runtime.InspectByName(ctx, target); err == nil && state != nil {
			_ = runtime.Stop(ctx, state.ID, 5*time.Second)
			_ = runtime.Remove(ctx, state.ID, true)
		}
	}
	cleanupMinio := func() {
		ctx := context.Background()
		if state, err := runtime.InspectByName(ctx, minioName); err == nil && state != nil {
			_ = runtime.Stop(ctx, state.ID, 5*time.Second)
			_ = runtime.Remove(ctx, state.ID, true)
		}
	}
	cleanupDB()
	cleanupMinio()
	t.Cleanup(cleanupDB)
	t.Cleanup(cleanupMinio)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	minioPort := freePort(t)
	startMinio(ctx, t, runtime, minioName, minioPort)
	minioEndpoint := fmt.Sprintf("http://127.0.0.1:%d", minioPort)
	createBucket(ctx, t, minioEndpoint, bucket)

	svcStore := openLiveStore(t)

	masterKey, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(svcStore, masterKey)

	ctrl := database.New(dbName, svcStore, runtime, database.WithPostgresCredentials(&database.PostgresCredentials{
		Username: "leveltest", Password: "leveltestpass",
	}))

	if err := svcStore.SaveDesiredDatabase(ctx, store.DesiredDatabase{
		Name: dbName, Engine: store.EnginePostgres, Version: "16",
	}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	engine := reconcile.NewEngine(discardTestLogger(), ctrl)
	engineCtx, stopEngine := context.WithCancel(ctx)
	defer stopEngine()
	go func() { _ = engine.Run(engineCtx, nil, 2*time.Second) }()

	waitContainerRunningE2E(ctx, t, runtime, target, 90*time.Second)
	waitReady(ctx, t, runtime, target, []string{"psql", "-U", "leveltest", "-d", "leveltest", "-c", "SELECT 1"}, 30*time.Second)

	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(logger, b, svcStore,
		api.WithBackupSecrets(secretsManager),
		api.WithBaseBackupRunner(&backup.BaseBackupRunner{
			Store:        svcStore,
			Secrets:      secretsManager,
			BaseBackuper: &backup.ContainerBaseBackuper{Runtime: runtime},
			Uploader:     backup.S3Uploader{},
			Runtime:      runtime,
		}),
		api.WithPITRRestoreRunner(&backup.PITRRunner{
			Store:           svcStore,
			Secrets:         secretsManager,
			Downloader:      backup.S3Downloader{},
			Restorer:        &backup.ContainerPITRRestorer{Runtime: runtime},
			Runtime:         runtime,
			Nudge:           engine.Nudge,
			PollInterval:    500 * time.Millisecond,
			TeardownTimeout: 60 * time.Second,
			StartupTimeout:  2 * time.Minute,
			PromoteTimeout:  2 * time.Minute,
		}),
	)
	ts := newE2ETestServer(t, router)

	if err := api.BootstrapAdmin(ctx, svcStore, e2ePITRAdminUsername, e2ePITRAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2ePITRAdminUsername, e2ePITRAdminPassword)

	// Step 1: enable PITR over real HTTP, then wait for the reconciler
	// (driven by the same real engine.Nudge PITRRunner itself uses) to
	// replace the container with one that actually has archive_mode on.
	status, body := postJSON(t, client, ts.URL+"/api/v1/databases/"+dbName+"/pitr", ``)
	if status != http.StatusOK {
		t.Fatalf("enable pitr: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	engine.Nudge()
	waitContainerRunningE2E(ctx, t, runtime, target, 90*time.Second)
	waitReady(ctx, t, runtime, target, []string{"psql", "-U", "leveltest", "-d", "leveltest", "-c", "SELECT 1"}, 30*time.Second)

	// Step 2: real backup target pointed at the real MinIO bucket, over
	// real HTTP, credentials round-tripped through real envelope
	// encryption.
	createTargetBody := fmt.Sprintf(`{"name":"minio-e2e","provider":"custom","endpoint":%q,"region":"us-east-1","bucket":%q,"access_key_id":"minioadmin","secret_access_key":"minioadmin"}`, minioEndpoint, bucket)
	status, body = postJSON(t, client, ts.URL+"/api/v1/backup-targets", createTargetBody)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("create backup target: status = %d, want 200/201, body = %s", status, body)
	}
	var targetResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &targetResp); err != nil || targetResp.ID == "" {
		t.Fatalf("decode backup target response %q: %v", body, err)
	}

	// Step 3: seed the "before" marker, then take a real physical base
	// backup over real HTTP, real pg_basebackup, real upload to MinIO.
	beforeMarker := "levelrail-pitr-e2e-before-7d2a"
	runSQL(ctx, t, runtime, target, `CREATE TABLE pitr_e2e_probe (val text); INSERT INTO pitr_e2e_probe VALUES ('`+beforeMarker+`');`)

	status, body = postJSON(t, client, ts.URL+"/api/v1/databases/"+dbName+"/base-backups", `{"target_id":"`+targetResp.ID+`"}`)
	if status != http.StatusAccepted {
		t.Fatalf("trigger base backup: status = %d, want %d, body = %s", status, http.StatusAccepted, body)
	}
	var baseBackupResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &baseBackupResp); err != nil || baseBackupResp.ID == "" {
		t.Fatalf("decode base backup response %q: %v", body, err)
	}
	waitBaseBackupSucceeded(t, client, ts.URL, dbName, baseBackupResp.ID, 90*time.Second)

	// Step 4: capture the recoverable window's own end, over real HTTP,
	// as this restore's target timestamp: it is the latest instant the
	// server itself claims WAL archiving has provably reached.
	targetTime := currentPITRWindowEnd(t, client, ts.URL, dbName)

	// Step 5: seed the "after" marker, which must NOT survive a restore
	// targeting a timestamp before it was written.
	afterMarker := "levelrail-pitr-e2e-after-4f91"
	runSQL(ctx, t, runtime, target, `INSERT INTO pitr_e2e_probe VALUES ('`+afterMarker+`');`)

	// Step 6: the actual point-in-time restore, over real HTTP.
	restoreBody := fmt.Sprintf(`{"base_backup_id":%q,"target_time":%q}`, baseBackupResp.ID, targetTime)
	status, body = postJSON(t, client, ts.URL+"/api/v1/databases/"+dbName+"/pitr-restore", restoreBody)
	if status != http.StatusAccepted {
		t.Fatalf("trigger pitr restore: status = %d, want %d, body = %s", status, http.StatusAccepted, body)
	}
	var restoreResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &restoreResp); err != nil || restoreResp.ID == "" {
		t.Fatalf("decode pitr restore response %q: %v", body, err)
	}
	waitPITRRestoreSucceeded(t, client, ts.URL, dbName, restoreResp.ID, 5*time.Minute)

	// Step 7: the real proof. Query the live, post-restore database
	// directly.
	out := runSQLCapture(ctx, t, runtime, target, `SELECT val FROM pitr_e2e_probe ORDER BY val;`)
	if !strings.Contains(out, beforeMarker) {
		t.Errorf("restored database is missing the before marker %q; rows: %q", beforeMarker, out)
	}
	if strings.Contains(out, afterMarker) {
		t.Errorf("restored database still has the after marker %q, which must not have survived a restore targeting a point before it was written; rows: %q", afterMarker, out)
	}
	count := strings.TrimSpace(runSQLCapture(ctx, t, runtime, target, `SELECT count(*) FROM pitr_e2e_probe;`))
	if count != "1" {
		t.Errorf("row count after restore = %q, want exactly 1 (only the before marker)", count)
	}
}

func startMinio(ctx context.Context, t *testing.T, rt docker.Runtime, name string, hostPort int) {
	t.Helper()
	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:  name,
		Image: "quay.io/minio/minio:latest",
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minioadmin",
			"MINIO_ROOT_PASSWORD": "minioadmin",
		},
		Command: []string{"server", "/data"},
		Ports: []docker.PortBinding{
			{ContainerPort: 9000, HostPort: hostPort, HostIP: "127.0.0.1"},
		},
	})
	if err != nil {
		t.Fatalf("create minio container: %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("start minio container: %v", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/minio/health/live", hostPort)) //nolint:noctx,gosec // test helper, loopback-only
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("minio never became healthy on port %d", hostPort)
}

func createBucket(ctx context.Context, t *testing.T, endpoint, bucket string) {
	t.Helper()
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("minioadmin", "minioadmin", "")),
	)
	if err != nil {
		t.Fatalf("load s3 client config: %v", err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("create bucket %q: %v", bucket, err)
	}
}

func waitContainerRunningE2E(ctx context.Context, t *testing.T, rt docker.Runtime, name string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state, err := rt.InspectByName(ctx, name)
		if err == nil && state != nil && state.Running {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("container %q never became running within %v", name, timeout)
}

// waitReady mirrors internal/backup's own identically-named live-test
// helper (dump_live_test.go): re-runs probe via rt.Exec until it exits
// zero or timeout elapses. A real query against the target database,
// not pg_isready: the official Postgres entrypoint runs a temporary
// internal server during initdb that pg_isready happily reports ready
// against too, the same gap that helper's own doc comment explains.
func waitReady(ctx context.Context, t *testing.T, rt docker.Runtime, containerName string, probe []string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		rc, err := rt.Exec(ctx, containerName, probe)
		if err == nil {
			_, readErr := io.Copy(io.Discard, rc)
			_ = rc.Close()
			if readErr == nil {
				return
			}
			lastErr = readErr
		} else {
			lastErr = err
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("container %q never became ready within %v: %v", containerName, timeout, lastErr)
}

func runSQL(ctx context.Context, t *testing.T, rt docker.Runtime, name, sql string) {
	t.Helper()
	rc, err := rt.Exec(ctx, name, []string{"psql", "-U", "leveltest", "-d", "leveltest", "-c", sql})
	if err != nil {
		t.Fatalf("Exec(%q) error = %v", sql, err)
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		t.Fatalf("Exec(%q) drain error = %v", sql, err)
	}
}

func runSQLCapture(ctx context.Context, t *testing.T, rt docker.Runtime, name, sql string) string {
	t.Helper()
	rc, err := rt.Exec(ctx, name, []string{"psql", "-U", "leveltest", "-d", "leveltest", "-Atq", "-c", sql})
	if err != nil {
		t.Fatalf("Exec(%q) error = %v", sql, err)
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Exec(%q) read error = %v", sql, err)
	}
	return string(out)
}

func waitBaseBackupSucceeded(t *testing.T, client *http.Client, baseURL, dbName, id string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		body, status, err := doGet(client, baseURL+"/api/v1/databases/"+dbName+"/base-backups")
		if err == nil && status == http.StatusOK {
			var history []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Error  string `json:"error"`
			}
			if json.Unmarshal([]byte(body), &history) == nil {
				for _, h := range history {
					if h.ID != id {
						continue
					}
					switch h.Status {
					case "succeeded":
						return
					case "failed":
						t.Fatalf("base backup %q failed: %s", id, h.Error)
					}
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("base backup %q did not succeed within %v", id, timeout)
}

func waitPITRRestoreSucceeded(t *testing.T, client *http.Client, baseURL, dbName, id string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		body, status, err := doGet(client, baseURL+"/api/v1/databases/"+dbName+"/pitr-restores")
		if err == nil && status == http.StatusOK {
			var history []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Error  string `json:"error"`
			}
			if json.Unmarshal([]byte(body), &history) == nil {
				for _, h := range history {
					if h.ID != id {
						continue
					}
					switch h.Status {
					case "succeeded":
						return
					case "failed":
						t.Fatalf("pitr restore %q failed: %s", id, h.Error)
					}
				}
			}
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("pitr restore %q did not succeed within %v", id, timeout)
}

func currentPITRWindowEnd(t *testing.T, client *http.Client, baseURL, dbName string) string {
	t.Helper()
	body, status, err := doGet(client, baseURL+"/api/v1/databases/"+dbName+"/pitr")
	if err != nil || status != http.StatusOK {
		t.Fatalf("get pitr status: status = %d, err = %v, body = %s", status, err, body)
	}
	var resp struct {
		HasBaseBackup bool   `json:"has_base_backup"`
		WindowEnd     string `json:"window_end"`
		WindowError   string `json:"window_error"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode pitr status %q: %v", body, err)
	}
	if resp.WindowError != "" {
		t.Fatalf("pitr status window_error = %q, want a usable window", resp.WindowError)
	}
	if !resp.HasBaseBackup || resp.WindowEnd == "" {
		t.Fatalf("pitr status = %+v, want a usable window", resp)
	}
	return resp.WindowEnd
}
