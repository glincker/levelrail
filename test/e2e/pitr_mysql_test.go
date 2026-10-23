// Live proof for MySQL and MariaDB point-in-time restore and TLS on new
// instances, driven through the real HTTP API, a real reconcile.Engine, real
// containers and a real S3-compatible bucket. Skipped when Docker is absent.
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

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/test/e2e/testenv"
)

const (
	e2eMySQLAdminUsername = "e2e-mysql-admin"
	e2eMySQLAdminPassword = "e2e-mysql-correct-horse" //nolint:gosec // test fixture credential, not a real secret
)

type mysqlFamilyCase struct {
	engine  string
	version string
}

var mysqlFamilyCases = []mysqlFamilyCase{
	{store.EngineMySQL, "8.4"},
	{store.EngineMariaDB, "11"},
}

func mysqlFamilyCreds(c mysqlFamilyCase) database.Option {
	if c.engine == store.EngineMariaDB {
		return database.WithMariaDBCredentials(&database.MariaDBCredentials{Username: "leveltest", Password: "leveltestpass"})
	}
	return database.WithMySQLCredentials(&database.MySQLCredentials{Username: "leveltest", Password: "leveltestpass"})
}

// sqlShell runs sql as root against the container's managed schema, picking the
// client binary each image ships.
func mysqlSQL(ctx context.Context, t *testing.T, rt docker.Runtime, container, sql string) string {
	t.Helper()
	script := `C=$(command -v mariadb || command -v mysql); P="${MYSQL_ROOT_PASSWORD:-$MARIADB_ROOT_PASSWORD}"; DB="${MYSQL_DATABASE:-$MARIADB_DATABASE}"; ` +
		`exec "$C" -uroot -p"$P" -N -B "$DB" -e "$1"`
	rc, err := rt.Exec(ctx, container, []string{"bash", "-c", script, "sh", sql})
	if err != nil {
		t.Fatalf("exec sql %q: %v", sql, err)
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("exec sql %q: %v", sql, err)
	}
	return string(out)
}

func waitMySQLReady(ctx context.Context, t *testing.T, rt docker.Runtime, container string) {
	t.Helper()
	probe := []string{"bash", "-c", `C=$(command -v mariadb || command -v mysql); P="${MYSQL_ROOT_PASSWORD:-$MARIADB_ROOT_PASSWORD}"; DB="${MYSQL_DATABASE:-$MARIADB_DATABASE}"; exec "$C" -h127.0.0.1 -uroot -p"$P" "$DB" -e "SELECT 1"`}
	waitReady(ctx, t, rt, container, probe, 3*time.Minute)
}

func TestPITR_Live_MySQLFamily_RestoresIntoNewDatabase(t *testing.T) {
	testenv.RequireFullLive(t)
	for _, c := range mysqlFamilyCases {
		t.Run(c.engine, func(t *testing.T) { runMySQLFamilyPITR(t, c) })
	}
}

func runMySQLFamilyPITR(t *testing.T, c mysqlFamilyCase) {
	env := newLiveBuildEnv(t)
	runtime := env.Runtime

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	srcName := "lr-e2e-pitr-" + c.engine + "-" + suffix
	newName := srcName + "-new"
	srcC, newC := "db-"+srcName, "db-"+newName
	s3Name := "lr-e2e-pitr-minio-" + c.engine + "-" + suffix
	const bucket = "pitr-e2e"

	cleanup := func() {
		removeContainerAndVolumes(env.DockerCli, srcC, srcC+"-data", srcC+"-wal-archive", srcC+"-certs")
		removeContainerAndVolumes(env.DockerCli, newC, newC+"-data", newC+"-certs")
		removeContainerAndVolumes(env.DockerCli, s3Name)
	}
	cleanup()
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	s3Port := freePort(t)
	startSeaweedS3(ctx, t, runtime, s3Name, s3Port)
	s3Endpoint := fmt.Sprintf("http://127.0.0.1:%d", s3Port)
	createBucket(ctx, t, s3Endpoint, bucket)

	svcStore := openLiveStore(t)
	masterKey, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(svcStore, masterKey)

	if err := svcStore.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: srcName, Engine: c.engine, Version: c.version}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	srcCtrl := database.New(srcName, svcStore, runtime, mysqlFamilyCreds(c))
	newCtrl := database.New(newName, svcStore, runtime, mysqlFamilyCreds(c))

	engine := reconcile.NewEngine(discardTestLogger(), srcCtrl, newCtrl)
	engine.SetStore(svcStore)
	engineCtx, stopEngine := context.WithCancel(ctx)
	defer stopEngine()
	go func() { _ = engine.Run(engineCtx, nil, 2*time.Second) }()

	waitContainerRunningE2E(ctx, t, runtime, srcC, 3*time.Minute)
	waitMySQLReady(ctx, t, runtime, srcC)

	downloader := backup.S3Downloader{}
	router := api.NewRouter(discardTestLogger(), &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}, svcStore,
		api.WithBackupSecrets(secretsManager),
		api.WithBaseBackupRunner(&backup.BaseBackupRunner{
			Store:        svcStore,
			Secrets:      secretsManager,
			BaseBackuper: &backup.ContainerBaseBackuper{Runtime: runtime},
			ByEngine: map[string]backup.BaseBackuper{
				store.EngineMySQL:   &backup.MySQLBaseBackuper{Runtime: runtime},
				store.EngineMariaDB: &backup.MySQLBaseBackuper{Runtime: runtime},
			},
			EngineOf: func(ctx context.Context, name string) (string, error) {
				d, err := svcStore.GetDesiredDatabase(ctx, name)
				if err != nil {
					return "", err
				}
				return d.Engine, nil
			},
			BinlogWAL: &backup.BinlogShipper{Runtime: runtime, Uploader: backup.S3Uploader{}, Lister: backup.S3Lister{}},
			Uploader:  backup.S3Uploader{},
			Runtime:   runtime,
		}),
		api.WithMySQLPITRRestoreRunner(&backup.MySQLPITRRunner{
			Store:        svcStore,
			Secrets:      secretsManager,
			Downloader:   downloader,
			Lister:       backup.S3Lister{},
			Restorer:     &backup.ContainerRestorer{Runtime: runtime},
			Shipper:      &backup.BinlogShipper{Runtime: runtime, Uploader: backup.S3Uploader{}, Lister: backup.S3Lister{}},
			Runtime:      runtime,
			PollInterval: 500 * time.Millisecond,
		}),
	)
	ts := newE2ETestServer(t, router)
	if err := api.BootstrapAdmin(ctx, svcStore, e2eMySQLAdminUsername, e2eMySQLAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eMySQLAdminUsername, e2eMySQLAdminPassword)

	if status, body := postJSON(t, client, ts.URL+"/api/v1/databases/"+srcName+"/pitr", ``); status != http.StatusOK {
		t.Fatalf("enable pitr: status = %d, body = %s", status, body)
	}
	engine.Nudge()
	waitMySQLBinlogOn(ctx, t, runtime, srcC)
	waitMySQLReady(ctx, t, runtime, srcC)

	createTargetBody := fmt.Sprintf(`{"name":"minio-e2e","provider":"custom","endpoint":%q,"region":"us-east-1","bucket":%q,"access_key_id":"minioadmin","secret_access_key":"minioadmin"}`, s3Endpoint, bucket)
	status, body := postJSON(t, client, ts.URL+"/api/v1/backup-targets", createTargetBody)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("create backup target: status = %d, body = %s", status, body)
	}
	var targetResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &targetResp); err != nil || targetResp.ID == "" {
		t.Fatalf("decode backup target %q: %v", body, err)
	}

	beforeMarker, midMarker, afterMarker := "before-7d2a", "mid-31c8", "after-4f91"
	mysqlSQL(ctx, t, runtime, srcC, `CREATE TABLE pitr_probe (val VARCHAR(64)); INSERT INTO pitr_probe VALUES ('`+beforeMarker+`')`)

	status, body = postJSON(t, client, ts.URL+"/api/v1/databases/"+srcName+"/base-backups", `{"target_id":"`+targetResp.ID+`"}`)
	if status != http.StatusAccepted {
		t.Fatalf("trigger base backup: status = %d, body = %s", status, body)
	}
	var bb struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &bb); err != nil || bb.ID == "" {
		t.Fatalf("decode base backup %q: %v", body, err)
	}
	waitBaseBackupSucceeded(t, client, ts.URL, srcName, bb.ID, 3*time.Minute)

	listBody, _, _ := doGet(client, ts.URL+"/api/v1/databases/"+srcName+"/base-backups")
	if !strings.Contains(listBody, `"lsn":"mysql-bin.`) {
		t.Errorf("base backup did not record a binlog coordinate: %s", listBody)
	}

	time.Sleep(2 * time.Second)
	mysqlSQL(ctx, t, runtime, srcC, `INSERT INTO pitr_probe VALUES ('`+midMarker+`')`)
	time.Sleep(2 * time.Second)
	targetTime := currentPITRWindowEnd(t, client, ts.URL, srcName)
	time.Sleep(2 * time.Second)
	mysqlSQL(ctx, t, runtime, srcC, `INSERT INTO pitr_probe VALUES ('`+afterMarker+`')`)

	status, body = postJSON(t, client, ts.URL+"/api/v1/databases/"+srcName+"/pitr-restore",
		fmt.Sprintf(`{"base_backup_id":%q,"target_time":%q}`, bb.ID, targetTime))
	if status != http.StatusBadRequest {
		t.Fatalf("restore without restore_as: status = %d, want 400, body = %s", status, body)
	}
	status, body = postJSON(t, client, ts.URL+"/api/v1/databases/"+srcName+"/pitr-restore",
		fmt.Sprintf(`{"base_backup_id":%q,"target_time":%q,"restore_as":%q}`, bb.ID, targetTime, newName))
	if status != http.StatusAccepted {
		t.Fatalf("trigger pitr restore: status = %d, body = %s", status, body)
	}
	var rr struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &rr); err != nil || rr.ID == "" {
		t.Fatalf("decode restore %q: %v", body, err)
	}
	waitPITRRestoreSucceeded(t, client, ts.URL, srcName, rr.ID, 8*time.Minute)

	rows := mysqlSQL(ctx, t, runtime, newC, `SELECT val FROM pitr_probe ORDER BY val`)
	if !strings.Contains(rows, beforeMarker) || !strings.Contains(rows, midMarker) {
		t.Errorf("restored database is missing rows written before the target (before=%q mid=%q): %q", beforeMarker, midMarker, rows)
	}
	if strings.Contains(rows, afterMarker) {
		t.Errorf("restored database has %q, written after the target: %q", afterMarker, rows)
	}
	if got := strings.TrimSpace(mysqlSQL(ctx, t, runtime, newC, `SELECT COUNT(*) FROM pitr_probe`)); got != "2" {
		t.Errorf("restored row count = %q, want 2", got)
	}
	src := mysqlSQL(ctx, t, runtime, srcC, `SELECT val FROM pitr_probe`)
	if !strings.Contains(src, afterMarker) {
		t.Errorf("source database lost %q, a new-database restore must not touch it: %q", afterMarker, src)
	}
}

// startSeaweedS3 runs a throwaway S3 compatible server; MinIO's free images are retired and the
// replacement needs a licence.
func startSeaweedS3(ctx context.Context, t *testing.T, rt docker.Runtime, name string, hostPort int) {
	t.Helper()
	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:    name,
		Image:   "chrislusf/seaweedfs:latest",
		Command: []string{"server", "-s3", "-dir=/data"},
		Ports:   []docker.PortBinding{{ContainerPort: 8333, HostPort: hostPort, HostIP: "127.0.0.1"}},
	})
	if err != nil {
		if strings.Contains(err.Error(), "pull image") {
			t.Skipf("s3 test image unavailable, skipping: %v", err)
		}
		t.Fatalf("create s3 container: %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("start s3 container: %v", err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", hostPort)) //nolint:noctx,gosec // test helper, loopback-only
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("s3 server never answered on port %d", hostPort)
}

func waitMySQLBinlogOn(ctx context.Context, t *testing.T, rt docker.Runtime, container string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		state, err := rt.InspectByName(ctx, container)
		if err == nil && state != nil && state.Running {
			rc, err := rt.Exec(ctx, container, []string{"bash", "-c", `C=$(command -v mariadb || command -v mysql); P="${MYSQL_ROOT_PASSWORD:-$MARIADB_ROOT_PASSWORD}"; exec "$C" -h127.0.0.1 -uroot -p"$P" -N -B -e "SELECT @@log_bin"`})
			if err == nil {
				out, _ := io.ReadAll(rc)
				_ = rc.Close()
				if strings.TrimSpace(string(out)) == "1" {
					if bin := mysqlBinlogDir(ctx, rt, container); strings.Contains(bin, "mysql-bin") {
						return
					}
				}
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("container %q never came back with the binlog on", container)
}

func mysqlBinlogDir(ctx context.Context, rt docker.Runtime, container string) string {
	rc, err := rt.Exec(ctx, container, []string{"sh", "-c", "ls /var/lib/mysql-binlog"})
	if err != nil {
		return ""
	}
	defer func() { _ = rc.Close() }()
	out, _ := io.ReadAll(rc)
	return string(out)
}

func TestTLS_Live_MySQLFamily_NewInstancesServeTLS(t *testing.T) {
	testenv.RequireFullLive(t)
	for _, c := range mysqlFamilyCases {
		t.Run(c.engine, func(t *testing.T) { runMySQLFamilyTLS(t, c) })
	}
}

func runMySQLFamilyTLS(t *testing.T, c mysqlFamilyCase) {
	env := newLiveBuildEnv(t)
	runtime := env.Runtime
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	newName := "lr-e2e-tls-" + c.engine + "-" + suffix
	oldName := "lr-e2e-tls-old-" + c.engine + "-" + suffix
	cleanup := func() {
		for _, n := range []string{newName, oldName} {
			removeContainerAndVolumes(env.DockerCli, "db-"+n, "db-"+n+"-data", "db-"+n+"-certs")
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	svcStore := openLiveStore(t)

	router := api.NewRouter(discardTestLogger(), &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}, svcStore)
	ts := newE2ETestServer(t, router)
	if err := api.BootstrapAdmin(ctx, svcStore, e2eMySQLAdminUsername, e2eMySQLAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eMySQLAdminUsername, e2eMySQLAdminPassword)

	status, body := postJSON(t, client, ts.URL+"/api/v1/databases", fmt.Sprintf(`{"name":%q,"engine":%q,"version":%q}`, newName, c.engine, c.version))
	if status != http.StatusCreated {
		t.Fatalf("create database: status = %d, body = %s", status, body)
	}
	// A database saved before this behaviour existed has no stored mode.
	if err := svcStore.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: oldName, Engine: c.engine, Version: c.version}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	optsFor := func(name string) []database.Option {
		opts := []database.Option{mysqlFamilyCreds(c)}
		mode, err := svcStore.GetDatabaseTLSMode(ctx, name)
		if err != nil {
			t.Fatalf("GetDatabaseTLSMode(%q) error = %v", name, err)
		}
		if store.DatabaseTLSEffective(c.engine, mode) {
			certPEM, keyPEM, err := database.GenerateSelfSignedCert(database.ContainerName(name))
			if err != nil {
				t.Fatalf("GenerateSelfSignedCert() error = %v", err)
			}
			opts = append(opts, database.WithTLS(&database.TLSMaterial{CertPEM: certPEM, KeyPEM: keyPEM}))
		}
		return opts
	}

	engine := reconcile.NewEngine(discardTestLogger(),
		database.New(newName, svcStore, runtime, optsFor(newName)...),
		database.New(oldName, svcStore, runtime, optsFor(oldName)...))
	engineCtx, stop := context.WithCancel(ctx)
	defer stop()
	go func() { _ = engine.Run(engineCtx, nil, 2*time.Second) }()

	for _, n := range []string{newName, oldName} {
		waitContainerRunningE2E(ctx, t, runtime, "db-"+n, 3*time.Minute)
		waitMySQLReady(ctx, t, runtime, "db-"+n)
	}

	if cipher := mysqlTCPCipher(ctx, t, runtime, "db-"+newName); cipher == "" {
		t.Errorf("new %s database negotiated no TLS cipher over TCP when TLS was required", c.engine)
	}
	if got := mysqlVar(ctx, t, runtime, "db-"+newName, "ssl_cert"); got != "/certs/tls.crt" {
		t.Errorf("new %s database ssl_cert = %q, want the platform certificate /certs/tls.crt", c.engine, got)
	}
	if got := mysqlVar(ctx, t, runtime, "db-"+oldName, "ssl_cert"); got == "/certs/tls.crt" {
		t.Errorf("pre-existing %s database serves the platform certificate, it must be left unchanged", c.engine)
	}
}

func mysqlVar(ctx context.Context, t *testing.T, rt docker.Runtime, container, name string) string {
	t.Helper()
	out := mysqlSQL(ctx, t, rt, container, "SHOW VARIABLES LIKE '"+name+"'")
	fields := strings.Fields(out)
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

// mysqlTCPCipher connects over TCP, refusing a plaintext session, and reports the negotiated cipher.
func mysqlTCPCipher(ctx context.Context, t *testing.T, rt docker.Runtime, container string) string {
	t.Helper()
	script := `C=$(command -v mariadb || command -v mysql); P="${MYSQL_ROOT_PASSWORD:-$MARIADB_ROOT_PASSWORD}"; ` +
		`case "$C" in *mariadb) F="--ssl --skip-ssl-verify-server-cert";; *) F=--ssl-mode=REQUIRED;; esac; ` +
		`exec "$C" -h127.0.0.1 -uroot -p"$P" $F -N -B -e "SHOW STATUS LIKE 'Ssl_cipher'"`
	rc, err := rt.Exec(ctx, container, []string{"bash", "-c", script})
	if err != nil {
		t.Fatalf("tls probe on %q: %v", container, err)
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("tls probe on %q: %v", container, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}
