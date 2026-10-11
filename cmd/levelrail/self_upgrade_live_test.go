package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
)

// liveUpgradeEnv is a throwaway "host": an installed binary, a data
// directory, a process the stop and start commands manage, and a release
// server that publishes assets for the upgrade to download.
type liveUpgradeEnv struct {
	t       *testing.T
	dir     string
	bin     string
	dataDir string
	port    int
	startSh string
	stopSh  string

	mu       sync.Mutex
	assets   map[string][]byte
	releases *httptest.Server
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func buildGo(t *testing.T, pkg, out string, ldflags string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w "+ldflags, "-o", out, pkg) //nolint:gosec // test builds its own fixtures
	cmd.Dir = filepath.Join("..", "..")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, b)
	}
}

const versionPkg = "github.com/GLINCKER/levelrail/internal/version.Version"

const installedVersion = "v0.4.0"

func newLiveUpgradeEnv(t *testing.T) *liveUpgradeEnv {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "lrup")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	brandFile, err := filepath.Abs(filepath.Join("..", "..", "brand.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_BRAND_FILE", brandFile)
	e := &liveUpgradeEnv{t: t, dir: dir, bin: filepath.Join(dir, "bin", "levelrail"), dataDir: filepath.Join(dir, "data"),
		port: freePort(t), assets: map[string][]byte{}}
	for _, d := range []string{filepath.Dir(e.bin), e.dataDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	buildGo(t, "./cmd/levelrail", e.bin, "-X "+versionPkg+"="+installedVersion)

	pidFile := filepath.Join(dir, "pid")
	e.startSh = filepath.Join(dir, "start.sh")
	e.stopSh = filepath.Join(dir, "stop.sh")
	start := fmt.Sprintf(`#!/bin/sh
APP_BRAND_FILE=%[6]s APP_DEV_MODE=1 APP_DATA_DIR=%[1]s APP_HTTP_ADDR=127.0.0.1:%[2]d APP_AGENT_ADDR=127.0.0.1:0 \
APP_INGRESS_HTTP_ADDR=127.0.0.1:0 APP_INGRESS_HTTPS_ADDR=127.0.0.1:0 \
nohup %[3]s >>%[4]s/server.log 2>&1 &
echo $! > %[5]s
`, e.dataDir, e.port, e.bin, dir, pidFile, brandFile)
	stop := fmt.Sprintf(`#!/bin/sh
[ -f %[1]s ] || exit 0
pid=$(cat %[1]s)
kill "$pid" 2>/dev/null
i=0
while kill -0 "$pid" 2>/dev/null && [ $i -lt 100 ]; do sleep 0.1; i=$((i+1)); done
kill -9 "$pid" 2>/dev/null
exit 0
`, pidFile)
	for p, c := range map[string]string{e.startSh: start, e.stopSh: stop} {
		if err := os.WriteFile(p, []byte(c), 0o755); err != nil { //nolint:gosec // executable test script
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = exec.Command(e.stopSh).Run() }) //nolint:gosec // script this test wrote

	mux := http.NewServeMux()
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		body, ok := e.assets[strings.TrimPrefix(r.URL.Path, "/dl/")]
		e.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	})
	e.releases = httptest.NewServer(mux)
	t.Cleanup(e.releases.Close)
	return e
}

func (e *liveUpgradeEnv) health() string { return fmt.Sprintf("http://127.0.0.1:%d", e.port) }

func (e *liveUpgradeEnv) startService() {
	e.t.Helper()
	if out, err := exec.Command(e.startSh).CombinedOutput(); err != nil { //nolint:gosec // script this test wrote
		e.t.Fatalf("start: %v %s", err, out)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(e.health() + "/readyz"); err == nil { //nolint:noctx // test poll
			_ = resp.Body.Close()
			if resp.StatusCode/100 == 2 {
				return
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	log, _ := os.ReadFile(filepath.Join(e.dir, "server.log"))
	e.t.Fatalf("service did not become ready:\n%s", log)
}

func (e *liveUpgradeEnv) runningVersion() string {
	e.t.Helper()
	out, err := exec.Command(e.bin, "version").CombinedOutput() //nolint:gosec // binary this test built
	if err != nil {
		e.t.Fatalf("version: %v %s", err, out)
	}
	return strings.Fields(string(out))[0]
}

func (e *liveUpgradeEnv) publish(tag string, binary []byte, sumOverride string) {
	asset := "levelrail-linux-" + runtime.GOARCH
	sum := sha256.Sum256(binary)
	digest := hex.EncodeToString(sum[:])
	if sumOverride != "" {
		digest = sumOverride
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.assets[tag+"/"+asset] = binary
	e.assets[tag+"/checksums.txt"] = []byte(digest + "  " + asset + "\n")
}

func (e *liveUpgradeEnv) selfUpgrade(tag string, extra ...string) (string, error) {
	e.t.Helper()
	args := append([]string{"self-upgrade", "--to", tag, "--yes", "--skip-notes-check", "--repo", "glincker/levelrail",
		"--stop-cmd", e.stopSh, "--start-cmd", e.startSh, "--health-url", e.health(), "--timeout", "20s"}, extra...)
	cmd := exec.Command(e.bin, args...) //nolint:gosec // test runs the binary it just built
	cmd.Env = append(os.Environ(), "APP_DATA_DIR="+e.dataDir,
		"LEVELRAIL_RELEASE_BASE_URL="+e.releases.URL+"/dl", "LEVELRAIL_INSECURE_MIRROR=1")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func (e *liveUpgradeEnv) sql() *sql.DB {
	e.t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(e.dataDir, "levelrail.db"))
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = db.Close() })
	return db
}

func (e *liveUpgradeEnv) plantData() {
	e.t.Helper()
	db := e.sql()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS live_probe (k TEXT PRIMARY KEY, v TEXT)`); err != nil {
		e.t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR REPLACE INTO live_probe VALUES ('customer', 'real data that must survive')`); err != nil {
		e.t.Fatal(err)
	}
}

func (e *liveUpgradeEnv) planted() string {
	e.t.Helper()
	var v string
	if err := e.sql().QueryRow(`SELECT v FROM live_probe WHERE k = 'customer'`).Scan(&v); err != nil {
		return "missing: " + err.Error()
	}
	return v
}

func (e *liveUpgradeEnv) schema() int {
	e.t.Helper()
	var v int
	if err := e.sql().QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil {
		e.t.Fatal(err)
	}
	return v
}

func readFile(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLive_SelfUpgrade_PreviousBuildToNewBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs real binaries")
	}
	e := newLiveUpgradeEnv(t)
	e.startService()
	e.plantData()
	before := e.schema()

	next := filepath.Join(e.dir, "levelrail-next")
	buildGo(t, "./cmd/levelrail", next, "-X "+versionPkg+"=v0.5.0")
	e.publish("v0.5.0", readFile(t, next), "")

	out, err := e.selfUpgrade("v0.5.0")
	if err != nil {
		log, _ := os.ReadFile(filepath.Join(e.dir, "server.log"))
		t.Fatalf("self-upgrade failed: %v\n%s\nserver log:\n%s", err, out, log)
	}
	for _, step := range []string{"download", "verify_checksum", "backup", "migration_check", "health"} {
		if !strings.Contains(out, "[ok] "+step) {
			t.Errorf("timeline missing ok %s:\n%s", step, out)
		}
	}
	if got := e.runningVersion(); got != "v0.5.0" {
		t.Fatalf("installed version = %s, want v0.5.0", got)
	}
	if got := e.planted(); got != "real data that must survive" {
		t.Fatalf("data after upgrade: %s", got)
	}
	if got := e.schema(); got < before {
		t.Fatalf("schema went backwards: %d -> %d", before, got)
	}
	if _, err := http.Get(e.health() + "/healthz"); err != nil { //nolint:noctx // test poll
		t.Fatalf("service not answering after upgrade: %v", err)
	}

	db, err := store.Open(context.Background(), filepath.Join(e.dataDir, "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	attempts, err := db.ListSelfUpgradeAttempts(context.Background(), 10)
	if err != nil || len(attempts) != 1 || attempts[0].Outcome != selfupgrade.OutcomeSucceeded || attempts[0].ToVersion != "v0.5.0" {
		t.Fatalf("recorded attempts = %+v, %v", attempts, err)
	}
	hist, err := db.ListUpgradeHistory(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	var sawSelfUpgrade bool
	for _, h := range hist {
		if h.ToVersion == "v0.5.0" && h.Method == "self-upgrade" {
			sawSelfUpgrade = true
		}
	}
	if !sawSelfUpgrade {
		t.Fatalf("upgrade history has no self-upgrade row for v0.5.0: %+v", hist)
	}
}

func TestLive_SelfUpgrade_BrokenBuildRollsBackWithDataIntact(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs real binaries")
	}
	e := newLiveUpgradeEnv(t)
	e.startService()
	e.plantData()
	schema := e.schema()
	installed := readFile(t, e.bin)

	broken := filepath.Join(e.dir, "brokenbuild")
	buildGo(t, "./cmd/levelrail/testdata/brokenbuild", broken, fmt.Sprintf("-X main.baseSchema=%d", schema))
	e.publish("v0.6.0", readFile(t, broken), "")

	out, err := e.selfUpgrade("v0.6.0", "--timeout", "6s")
	if err == nil {
		t.Fatalf("a broken build was reported as upgraded:\n%s", out)
	}
	if !strings.Contains(out, "rolled_back") {
		t.Fatalf("outcome is not rolled_back:\n%s", out)
	}
	if !bytes.Equal(readFile(t, e.bin), installed) {
		t.Fatal("previous binary was not restored byte for byte")
	}
	if got := e.runningVersion(); got != "v0.4.0" {
		t.Fatalf("version after rollback = %s", got)
	}
	if got := e.planted(); got != "real data that must survive" {
		t.Fatalf("data after rollback: %s", got)
	}
	if got := e.schema(); got != schema {
		t.Fatalf("schema after rollback = %d, want %d", got, schema)
	}
	resp, herr := http.Get(e.health() + "/readyz") //nolint:noctx // test poll
	if herr != nil || resp.StatusCode/100 != 2 {
		t.Fatalf("previous release not healthy after rollback: %v", herr)
	}
	_ = resp.Body.Close()
}

func TestLive_SelfUpgrade_ChecksumMismatchChangesNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs real binaries")
	}
	e := newLiveUpgradeEnv(t)
	e.startService()
	installed := readFile(t, e.bin)
	e.publish("v0.5.0", []byte("not the binary the checksum describes"), strings.Repeat("0", 64))

	out, err := e.selfUpgrade("v0.5.0")
	if err == nil || !strings.Contains(out, "refused") {
		t.Fatalf("mismatch was accepted: %v\n%s", err, out)
	}
	if !bytes.Equal(readFile(t, e.bin), installed) {
		t.Fatal("binary changed although verification failed")
	}
	if resp, herr := http.Get(e.health() + "/readyz"); herr != nil || resp.StatusCode/100 != 2 { //nolint:noctx // test poll
		t.Fatalf("service disturbed by a refused upgrade: %v", herr)
	}
}

func TestLive_DowngradeGuardRefusesNewerDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs real binaries")
	}
	e := newLiveUpgradeEnv(t)
	e.startService()
	_ = exec.Command(e.stopSh).Run() //nolint:gosec // script this test wrote
	db := e.sql()
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name) VALUES (99999, 'from_the_future')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO upgrade_history (id, kind, from_version, to_version, channel, schema_before, schema_after, occurred_at, initiator, method, backup_name, health, notes, notes_state)
		VALUES ('uh_future', 'upgraded', 'v0.4.0', 'v9.9.9', 'stable', 1, 99999, '2099-01-01T00:00:00.000Z', 'x', '', '', 'booted', '', 'unavailable')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	cmd := exec.Command(e.bin) //nolint:gosec // test runs the binary it just built
	cmd.Env = append(os.Environ(), "APP_DEV_MODE=1", "APP_DATA_DIR="+e.dataDir, "APP_HTTP_ADDR=127.0.0.1:0",
		"APP_AGENT_ADDR=127.0.0.1:0", "APP_INGRESS_HTTP_ADDR=127.0.0.1:0", "APP_INGRESS_HTTPS_ADDR=127.0.0.1:0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != selfupgrade.ExitDowngradeRefused {
		t.Fatalf("exit = %v, want %d\n%s", err, selfupgrade.ExitDowngradeRefused, stderr.String())
	}
	for _, want := range []string{"v0.4.0", "schema 99999", "v9.9.9", "LEVELRAIL_VERSION=v9.9.9", "restore-snapshot --list", "forward-only"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("message missing %q:\n%s", want, stderr.String())
		}
	}
}
