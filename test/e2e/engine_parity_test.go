// Live proof that MySQL, MariaDB and MongoDB databases back up, verify and
// restore into a new database through the real HTTP API, and that MongoDB
// serves TLS on new instances. Skipped when Docker is absent.
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

type parityEngine struct {
	engine  string
	version string
	creds   func() database.Option
	seed    func(ctx context.Context, t *testing.T, rt docker.Runtime, container, marker string)
	read    func(ctx context.Context, t *testing.T, rt docker.Runtime, container string) string
}

const mongoShellPrefix = `exec mongosh --quiet --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin `

func mongoEval(ctx context.Context, t *testing.T, rt docker.Runtime, container, js string) string {
	t.Helper()
	rc, err := rt.Exec(ctx, container, []string{"sh", "-c", mongoShellPrefix + `--eval "$1"`, "sh", js})
	if err != nil {
		t.Fatalf("mongosh on %q: %v", container, err)
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("mongosh on %q: %v", container, err)
	}
	return string(out)
}

func parityEngines() []parityEngine {
	mysqlSeed := func(ctx context.Context, t *testing.T, rt docker.Runtime, c, marker string) {
		mysqlSQL(ctx, t, rt, c, `CREATE TABLE parity_probe (val VARCHAR(64)); INSERT INTO parity_probe VALUES ('`+marker+`')`)
	}
	mysqlRead := func(ctx context.Context, t *testing.T, rt docker.Runtime, c string) string {
		return mysqlSQL(ctx, t, rt, c, `SELECT val FROM parity_probe`)
	}
	return []parityEngine{
		{store.EngineMySQL, "8.4", func() database.Option { return mysqlFamilyCreds(mysqlFamilyCases[0]) }, mysqlSeed, mysqlRead},
		{store.EngineMariaDB, "11", func() database.Option { return mysqlFamilyCreds(mysqlFamilyCases[1]) }, mysqlSeed, mysqlRead},
		{store.EngineMongoDB, "7",
			func() database.Option {
				return database.WithMongoDBCredentials(&database.MongoDBCredentials{Username: "leveltest", Password: "leveltestpass"})
			},
			func(ctx context.Context, t *testing.T, rt docker.Runtime, c, marker string) {
				mongoEval(ctx, t, rt, c, `db.getSiblingDB("parityprobe").items.insertOne({val: "`+marker+`"})`)
			},
			func(ctx context.Context, t *testing.T, rt docker.Runtime, c string) string {
				return mongoEval(ctx, t, rt, c, `db.getSiblingDB("parityprobe").items.find().toArray().map(d => d.val).join(",")`)
			},
		},
	}
}

func TestBackupRestoreVerify_Live_Engines(t *testing.T) {
	testenv.RequireFullLive(t)
	for _, pe := range parityEngines() {
		t.Run(pe.engine, func(t *testing.T) { runBackupRestoreVerify(t, pe) })
	}
}

func waitEngineReady(ctx context.Context, t *testing.T, rt docker.Runtime, pe parityEngine, container string) {
	t.Helper()
	if pe.engine == store.EngineMongoDB {
		waitReady(ctx, t, rt, container, []string{"sh", "-c", mongoShellPrefix + `--eval 'db.adminCommand("ping")'`}, 3*time.Minute)
		return
	}
	waitMySQLReady(ctx, t, rt, container)
}

func runBackupRestoreVerify(t *testing.T, pe parityEngine) {
	env := newLiveBuildEnv(t)
	runtime := env.Runtime

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	srcName := "lr-e2e-par-" + pe.engine + "-" + suffix
	newName := srcName + "-new"
	srcC, newC := "db-"+srcName, "db-"+newName
	s3Name := "lr-e2e-par-s3-" + pe.engine + "-" + suffix
	const bucket = "parity-e2e"

	cleanup := func() {
		for _, c := range []string{srcC, newC} {
			removeContainerAndVolumes(env.DockerCli, c, c+"-data", c+"-certs")
		}
		removeContainerAndVolumes(env.DockerCli, s3Name)
	}
	cleanup()
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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

	if err := svcStore.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: srcName, Engine: pe.engine, Version: pe.version}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	engine := reconcile.NewEngine(discardTestLogger(),
		database.New(srcName, svcStore, runtime, pe.creds()),
		database.New(newName, svcStore, runtime, pe.creds()))
	engine.SetStore(svcStore)
	engineCtx, stop := context.WithCancel(ctx)
	defer stop()
	go func() { _ = engine.Run(engineCtx, nil, 2*time.Second) }()

	waitContainerRunningE2E(ctx, t, runtime, srcC, 3*time.Minute)
	waitEngineReady(ctx, t, runtime, pe, srcC)
	pe.seed(ctx, t, runtime, srcC, "parity-marker-5b1e")

	router := api.NewRouter(discardTestLogger(), &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}, svcStore,
		api.WithBackupSecrets(secretsManager),
		api.WithBackupRunner(&backup.Runner{
			Store:    svcStore,
			Secrets:  secretsManager,
			Dumper:   &backup.ContainerDumper{Runtime: runtime},
			Uploader: backup.S3Uploader{},
		}),
		api.WithBackupVerifier(&backup.VerifyRunner{Store: svcStore, Secrets: secretsManager, Downloader: backup.S3Downloader{}}),
		api.WithCloneRestoreRunner(&backup.CloneRestoreRunner{
			Store:        svcStore,
			Secrets:      secretsManager,
			Downloader:   backup.S3Downloader{},
			Restorer:     &backup.ContainerRestorer{Runtime: runtime},
			PollInterval: 500 * time.Millisecond,
		}),
	)
	ts := newE2ETestServer(t, router)
	if err := api.BootstrapAdmin(ctx, svcStore, e2eMySQLAdminUsername, e2eMySQLAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eMySQLAdminUsername, e2eMySQLAdminPassword)

	createTargetBody := fmt.Sprintf(`{"name":"s3-e2e","provider":"custom","endpoint":%q,"region":"us-east-1","bucket":%q,"access_key_id":"levelrailtest","secret_access_key":"levelrailtestsecret"}`, s3Endpoint, bucket)
	status, body := postJSON(t, client, ts.URL+"/api/v1/backup-targets", createTargetBody)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("create backup target: status = %d, body = %s", status, body)
	}
	targetID := jsonID(t, body)

	status, body = postJSON(t, client, ts.URL+"/api/v1/databases/"+srcName+"/backups", `{"target_id":"`+targetID+`"}`)
	if status != http.StatusAccepted {
		t.Fatalf("trigger backup: status = %d, body = %s", status, body)
	}
	backupID := jsonID(t, body)
	waitHistoryStatus(t, client, ts.URL+"/api/v1/databases/"+srcName+"/backups", backupID, "backup", 3*time.Minute)

	status, body = postJSON(t, client, ts.URL+"/api/v1/databases/"+srcName+"/backups/"+backupID+"/verify", ``)
	if status != http.StatusAccepted {
		t.Fatalf("verify backup: status = %d, body = %s", status, body)
	}
	verifyID := jsonID(t, body)
	waitHistoryStatus(t, client, ts.URL+"/api/v1/databases/"+srcName+"/backups/"+backupID+"/verifications", verifyID, "verification", 2*time.Minute)

	status, body = postJSON(t, client, ts.URL+"/api/v1/databases/"+srcName+"/restore-as-new", fmt.Sprintf(`{"backup_id":%q,"new_name":%q}`, backupID, newName))
	if status != http.StatusAccepted {
		t.Fatalf("restore as new: status = %d, body = %s", status, body)
	}
	cloneID := jsonID(t, body)
	waitHistoryStatus(t, client, ts.URL+"/api/v1/databases/"+srcName+"/clone-restores", cloneID, "clone restore", 6*time.Minute)

	if got := pe.read(ctx, t, runtime, newC); !strings.Contains(got, "parity-marker-5b1e") {
		t.Errorf("restored %s database is missing the seeded row: %q", pe.engine, got)
	}
}

func jsonID(t *testing.T, body string) string {
	const field = "id"
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	v, _ := m[field].(string)
	if v == "" {
		t.Fatalf("response %q has no %q", body, field)
	}
	return v
}

// waitHistoryStatus polls a list endpoint until the row with id succeeded or passed.
func waitHistoryStatus(t *testing.T, client *http.Client, url, id, what string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		body, status, err := doGet(client, url)
		if err == nil && status == http.StatusOK {
			var rows []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Error  string `json:"error"`
			}
			if json.Unmarshal([]byte(body), &rows) == nil {
				for _, r := range rows {
					if r.ID != id {
						continue
					}
					switch r.Status {
					case "succeeded", "passed":
						return
					case "failed":
						t.Fatalf("%s %q failed: %s", what, id, r.Error)
					}
				}
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("%s %q did not finish within %v", what, id, timeout)
}

func TestTLS_Live_MongoDB_NewInstancesServeTLS(t *testing.T) {
	testenv.RequireFullLive(t)
	env := newLiveBuildEnv(t)
	runtime := env.Runtime
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	newName := "lr-e2e-mtls-" + suffix
	oldName := "lr-e2e-mtls-old-" + suffix
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

	status, body := postJSON(t, client, ts.URL+"/api/v1/databases", fmt.Sprintf(`{"name":%q,"engine":"mongodb","version":"7"}`, newName))
	if status != http.StatusCreated {
		t.Fatalf("create database: status = %d, body = %s", status, body)
	}
	if err := svcStore.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: oldName, Engine: store.EngineMongoDB, Version: "7"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	optsFor := func(name string) []database.Option {
		opts := []database.Option{database.WithMongoDBCredentials(&database.MongoDBCredentials{Username: "leveltest", Password: "leveltestpass"})}
		mode, err := svcStore.GetDatabaseTLSMode(ctx, name)
		if err != nil {
			t.Fatalf("GetDatabaseTLSMode(%q) error = %v", name, err)
		}
		if store.DatabaseTLSEffective(store.EngineMongoDB, mode) {
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

	pe := parityEngines()[2]
	for _, n := range []string{newName, oldName} {
		waitContainerRunningE2E(ctx, t, runtime, "db-"+n, 3*time.Minute)
		waitEngineReady(ctx, t, runtime, pe, "db-"+n)
	}

	if !mongoTLSConnects(ctx, t, runtime, "db-"+newName) {
		t.Errorf("new mongodb database refused a TLS connection")
	}
	if mongoTLSConnects(ctx, t, runtime, "db-"+oldName) {
		t.Errorf("pre-existing mongodb database accepted a TLS connection, it must be left unchanged")
	}
	if out := mongoEval(ctx, t, runtime, "db-"+newName, `db.adminCommand("ping").ok`); !strings.Contains(out, "1") {
		t.Errorf("plaintext ping on the TLS-enabled database failed: %q", out)
	}
}

func mongoTLSConnects(ctx context.Context, t *testing.T, rt docker.Runtime, container string) bool {
	t.Helper()
	script := mongoShellPrefix + `--tls --tlsAllowInvalidCertificates --host 127.0.0.1 --eval 'db.adminCommand("ping").ok'`
	rc, err := rt.Exec(ctx, container, []string{"sh", "-c", script})
	if err != nil {
		return false
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(io.Discard, rc)
	return err == nil
}
