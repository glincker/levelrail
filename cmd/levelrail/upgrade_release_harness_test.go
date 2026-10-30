package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
)

// The release harness drives real, previously released binaries, not the
// current source relabelled. It is off unless the operator names tags, because
// building a release needs the network-free but slow git archive plus go build.
const (
	envHarnessFromTags  = "LEVELRAIL_UPGRADE_FROM_TAGS"
	envHarnessBinaryDir = "LEVELRAIL_UPGRADE_FROM_BINARY_DIR"
	envHarnessTarget    = "LEVELRAIL_UPGRADE_TARGET_VERSION"
	defaultHarnessTo    = "v0.8.99"
	harnessRootToken    = "harness-root-token"
	harnessSecretValue  = "harness-secret-value-0451"
	harnessAppName      = "harness-web"
	harnessDBName       = "harness-db"
	harnessUserEmail    = "harness-user@example.test"
	harnessProbeValue   = "real data that must survive"
)

func harnessTags(t *testing.T) []string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs real release binaries")
	}
	raw := strings.TrimSpace(os.Getenv(envHarnessFromTags))
	if raw == "" {
		t.Skipf("set %s=v0.8.2,v0.8.1 to run the release upgrade harness (scripts/upgrade-harness.sh does)", envHarnessFromTags)
	}
	var tags []string
	for _, tag := range strings.Split(raw, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func harnessTarget() string {
	if v := strings.TrimSpace(os.Getenv(envHarnessTarget)); v != "" {
		return v
	}
	return defaultHarnessTo
}

// installReleaseBinary puts the released binary for tag at out: a prebuilt one
// from the binary dir when present, otherwise built from the tag's own source
// in a temporary directory so no working tree is touched.
func installReleaseBinary(t *testing.T, tag, out string) {
	t.Helper()
	if dir := os.Getenv(envHarnessBinaryDir); dir != "" {
		pre := filepath.Join(dir, tag, "levelrail")
		if raw, err := os.ReadFile(pre); err == nil { //nolint:gosec // operator-supplied prebuilt release
			if err := os.WriteFile(out, raw, 0o755); err != nil { //nolint:gosec // executable test fixture
				t.Fatal(err)
			}
			return
		}
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", tag+"^{commit}").Run(); err != nil { //nolint:gosec // test-controlled args
		t.Skipf("tag %s is not in this clone (git fetch --tags)", tag)
	}
	src, err := os.MkdirTemp("", "lrrel")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(src) })
	archive := exec.Command("git", "-C", root, "archive", "--format=tar", tag) //nolint:gosec // test-controlled args
	untar := exec.Command("tar", "-x", "-C", src)                              //nolint:gosec // temp dir this test created
	pipe, err := archive.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	untar.Stdin = pipe
	if err := untar.Start(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Run(); err != nil {
		t.Fatalf("git archive %s: %v", tag, err)
	}
	if err := untar.Wait(); err != nil {
		t.Fatalf("untar %s: %v", tag, err)
	}
	build := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X "+versionPkg+"="+tag, "-o", out, "./cmd/levelrail") //nolint:gosec // builds a release from its own tagged source
	build.Dir = src
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", tag, err, b)
	}
}

type harnessEnv struct {
	*liveUpgradeEnv
	tag string
	mk  *secrets.MasterKey
}

func newHarnessEnv(t *testing.T, tag string) *harnessEnv {
	t.Helper()
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	fixtures := filepath.Join(t.TempDir(), "dev-fixtures.yml")
	yml := "tokens:\n  - name: harness-root\n    plaintext: " + harnessRootToken + "\n    abilities: [root]\n"
	if err := os.WriteFile(fixtures, []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	env := fmt.Sprintf("APP_MASTER_KEY=%s APP_DEV_FIXTURES_FILE=%s", mk.String(), fixtures)
	e := newLiveUpgradeEnvWith(t, liveUpgradeOpts{
		install:  func(t *testing.T, bin string) { installReleaseBinary(t, tag, bin) },
		extraEnv: env,
	})
	return &harnessEnv{liveUpgradeEnv: e, tag: tag, mk: mk}
}

func (h *harnessEnv) api(method, path string, body any) (int, []byte) {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, h.health()+path, rdr) //nolint:noctx // test helper
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+harnessRootToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // loopback test server
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func (h *harnessEnv) mustAPI(method, path string, body any) []byte {
	h.t.Helper()
	code, raw := h.api(method, path, body)
	if code/100 != 2 {
		h.t.Fatalf("%s %s on %s: %d %s", method, path, h.tag, code, raw)
	}
	return raw
}

func (h *harnessEnv) seed() {
	h.t.Helper()
	h.mustAPI("POST", "/api/v1/apps", map[string]any{"name": harnessAppName, "image": "nginx:1.27", "port": 80, "env": map[string]string{"MODE": "harness"}})
	h.mustAPI("PUT", "/api/v1/apps/"+harnessAppName+"/secrets/API_KEY", map[string]string{"value": harnessSecretValue})
	h.mustAPI("POST", "/api/v1/databases", map[string]any{"name": harnessDBName, "engine": "postgres", "version": "16"})
	h.mustAPI("POST", "/api/v1/auth/users", map[string]any{"email": harnessUserEmail, "password": "harness-password-1234567", "abilities": []string{"read"}})
	h.plantData()
}

// harnessState is what an operator would compare across an upgrade.
type harnessState struct {
	Apps      []string
	Databases []string
	Users     []string
	AuditIDs  []string
	Probe     string
	Secret    string
}

func names(raw []byte, keys ...string) []string {
	var rows []map[string]any
	if json.Unmarshal(raw, &rows) != nil {
		var generic map[string]json.RawMessage
		if json.Unmarshal(raw, &generic) == nil {
			for _, v := range generic {
				if json.Unmarshal(v, &rows) == nil && len(rows) > 0 {
					break
				}
			}
		}
	}
	var out []string
	for _, r := range rows {
		for _, k := range keys {
			if v, ok := r[k].(string); ok && v != "" {
				out = append(out, v)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func (h *harnessEnv) capture() harnessState {
	h.t.Helper()
	st := harnessState{
		Apps:      names(h.mustAPI("GET", "/api/v1/apps", nil), "name"),
		Databases: names(h.mustAPI("GET", "/api/v1/databases", nil), "name"),
		Users:     names(h.mustAPI("GET", "/api/v1/users", nil), "email", "username"),
		AuditIDs:  names(h.mustAPI("GET", "/api/v1/audit-log?limit=1000", nil), "id"),
		Probe:     h.planted(),
	}
	// Read a snapshot: opening the live file with this tree would migrate an old schema.
	snap := filepath.Join(h.t.TempDir(), "capture.db")
	if _, err := h.sql().Exec("VACUUM INTO ?", snap); err != nil {
		h.t.Fatal(err)
	}
	db, err := store.Open(context.Background(), snap)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	got, err := secrets.NewManager(db, h.mk).Resolve(context.Background(), harnessAppName, "API_KEY")
	if err != nil {
		h.t.Fatalf("secret does not resolve on %s: %v", h.tag, err)
	}
	st.Secret = got
	return st
}

func contains(have []string, want string) bool {
	for _, s := range have {
		if s == want {
			return true
		}
	}
	return false
}

func assertSeeded(t *testing.T, st harnessState, label string) {
	t.Helper()
	for what, ok := range map[string]bool{
		"app":          contains(st.Apps, harnessAppName),
		"database":     contains(st.Databases, harnessDBName),
		"user":         contains(st.Users, harnessUserEmail),
		"audit rows":   len(st.AuditIDs) > 0,
		"probe row":    st.Probe == harnessProbeValue,
		"secret value": st.Secret == harnessSecretValue,
	} {
		if !ok {
			t.Errorf("%s: %s is missing or changed: %+v", label, what, st)
		}
	}
}

func assertAuditKept(t *testing.T, before, after harnessState, label string) {
	t.Helper()
	for _, id := range before.AuditIDs {
		if !contains(after.AuditIDs, id) {
			t.Errorf("%s: audit row %s was lost", label, id)
		}
	}
}

func (h *harnessEnv) run(args ...string) (string, error) {
	h.t.Helper()
	cmd := exec.Command(h.bin, args...) //nolint:gosec // the binary this test installed
	cmd.Env = append(os.Environ(), "APP_DATA_DIR="+h.dataDir, "APP_MASTER_KEY="+h.mk.String())
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func (h *harnessEnv) restorePoints() []selfupgrade.RestorePoint {
	h.t.Helper()
	pts, err := selfupgrade.NewRestorePoints(h.dataDir).List()
	if err != nil {
		h.t.Fatal(err)
	}
	return pts
}

func digestOf(t *testing.T, path string) string {
	t.Helper()
	sum := sha256.Sum256(readFile(t, path))
	return hex.EncodeToString(sum[:])
}

func (h *harnessEnv) publishCurrent(tag string) {
	h.t.Helper()
	next := filepath.Join(h.dir, "levelrail-"+tag)
	buildGo(h.t, "./cmd/levelrail", next, "-X "+versionPkg+"="+tag)
	h.publish(tag, readFile(h.t, next), "")
}

func TestReleaseUpgradeHarness(t *testing.T) {
	for _, tag := range harnessTags(t) {
		t.Run(tag, func(t *testing.T) { runReleaseHarness(t, tag) })
	}
}

func runReleaseHarness(t *testing.T, tag string) {
	to := harnessTarget()
	h := newHarnessEnv(t, tag)
	oldBinary := readFile(t, h.bin)
	if got := h.runningVersion(); got != tag {
		t.Fatalf("installed release reports %s, want %s", got, tag)
	}
	h.startService()
	h.seed()
	before := h.capture()
	assertSeeded(t, before, "seeded on "+tag)
	schemaBefore := h.schema()

	h.publishCurrent(to)
	planOut, planErr := h.selfUpgrade(to, "--dry-run")
	if planErr != nil {
		t.Fatalf("self-upgrade --dry-run failed: %v\n%s", planErr, planOut)
	}
	if got := h.runningVersion(); got != tag || h.schema() != schemaBefore || !bytes.Equal(readFile(t, h.bin), oldBinary) || len(h.restorePoints()) != 0 {
		t.Fatalf("--dry-run changed the instance: version %s schema %d restore points %d", got, h.schema(), len(h.restorePoints()))
	}
	assertSeeded(t, h.capture(), "after --dry-run")
	out, err := h.selfUpgrade(to)
	if err != nil {
		log, _ := os.ReadFile(filepath.Join(h.dir, "server.log"))
		t.Fatalf("upgrade %s to %s failed: %v\n%s\nserver log:\n%s", tag, to, err, out, log)
	}
	for _, step := range []string{"verify_checksum", "backup", "migration_check", "health"} {
		if !strings.Contains(out, "[ok] "+step) {
			t.Errorf("timeline missing ok %s:\n%s", step, out)
		}
	}
	if got := h.runningVersion(); got != to {
		t.Fatalf("version after upgrade = %s, want %s", got, to)
	}
	schemaAfter := h.schema()
	if schemaAfter < schemaBefore {
		t.Fatalf("schema went backwards: %d -> %d", schemaBefore, schemaAfter)
	}
	after := h.capture()
	assertSeeded(t, after, "after upgrade")
	assertAuditKept(t, before, after, "after upgrade")
	if code, raw := h.api("GET", "/api/v1/audit-log/verify", nil); code/100 != 2 {
		t.Errorf("audit chain verification after upgrade: %d %s", code, raw)
	}

	pts := h.restorePoints()
	if len(pts) != 1 || pts[0].Version != tag || !pts[0].HasBinary {
		t.Fatalf("restore points = %+v, want one holding the %s binary", pts, tag)
	}
	rp := selfupgrade.NewRestorePoints(h.dataDir)
	if !bytes.Equal(readFile(t, rp.BinaryPath(pts[0].ID)), oldBinary) {
		t.Fatal("restore point binary is not the previous release byte for byte")
	}
	assertSnapshotHoldsSeed(t, rp.DBPath(pts[0].ID))

	t.Run("manual restore point", func(t *testing.T) {
		h.mustAPI("POST", "/api/v1/apps", map[string]any{"name": "after-upgrade-app", "image": "nginx:1.27", "port": 80})
		out, err := h.run("restore-point", "--id", pts[0].ID, "--stop-cmd", h.stopSh, "--start-cmd", h.startSh, "--health-url", h.health())
		if err != nil {
			t.Fatalf("restore-point: %v\n%s", err, out)
		}
		if got := h.runningVersion(); got != tag {
			t.Fatalf("version after restore point = %s, want %s", got, tag)
		}
		if schemaAfter == schemaBefore {
			if !strings.Contains(out, "database kept") {
				t.Errorf("expected the compatible database to be kept:\n%s", out)
			}
		} else {
			if got := h.schema(); got != schemaBefore {
				t.Fatalf("schema after restore point = %d, want %d", got, schemaBefore)
			}
			if !strings.Contains(out, "database restored") {
				t.Errorf("expected the migrated database to be restored:\n%s", out)
			}
		}
		restored := h.capture()
		assertSeeded(t, restored, "after restore point")
		assertAuditKept(t, before, restored, "after restore point")
		kept := contains(restored.Apps, "after-upgrade-app")
		if kept != (schemaAfter == schemaBefore) {
			t.Errorf("post-upgrade change kept=%v, want %v for schemas %d to %d", kept, schemaAfter == schemaBefore, schemaBefore, schemaAfter)
		}
	})

	t.Run("failed verification restores automatically", func(t *testing.T) {
		schema := h.schema()
		installed := readFile(t, h.bin)
		broken := filepath.Join(h.dir, "brokenbuild")
		buildGo(t, "./cmd/levelrail/testdata/brokenbuild", broken, fmt.Sprintf("-X main.baseSchema=%d -X main.reportedVersion=v0.8.98", schema))
		h.publish("v0.8.98", readFile(t, broken), "")
		out, err := h.selfUpgrade("v0.8.98", "--timeout", "6s")
		if err == nil || !strings.Contains(out, "rolled_back") {
			t.Fatalf("a release that never became healthy was not rolled back: %v\n%s", err, out)
		}
		if !bytes.Equal(readFile(t, h.bin), installed) {
			t.Fatal("previous binary was not restored byte for byte")
		}
		if !strings.Contains(out, "database restored") && !strings.Contains(out, "database_restored") {
			t.Logf("timeline does not mention a database restore:\n%s", out)
		}
		if got := h.schema(); got != schema {
			t.Fatalf("schema after rollback = %d, want %d", got, schema)
		}
		restored := h.capture()
		assertSeeded(t, restored, "after automatic restore")
		assertAuditKept(t, before, restored, "after automatic restore")
	})

	t.Run("downgrade is refused", func(t *testing.T) {
		if out, err := h.selfUpgrade(to); err != nil {
			t.Fatalf("second upgrade to %s: %v\n%s", to, err, out)
		}
		migrated := h.schema()
		installed := readFile(t, h.bin)
		out, err := h.selfUpgrade(tag)
		if err == nil || !strings.Contains(out, "refused") {
			t.Fatalf("self-upgrade to the older %s was not refused: %v\n%s", tag, err, out)
		}
		if !bytes.Equal(readFile(t, h.bin), installed) {
			t.Fatal("binary changed although the downgrade was refused")
		}
		if resp, herr := http.Get(h.health() + "/readyz"); herr != nil || resp.StatusCode/100 != 2 { //nolint:noctx // test poll
			t.Fatalf("service disturbed by a refused downgrade: %v", herr)
		}

		_ = exec.Command(h.stopSh).Run() //nolint:gosec // script this test wrote
		dbPath := filepath.Join(h.dataDir, "levelrail.db")
		if migrated == schemaBefore {
			plantNewerRelease(t, h, migrated+1)
		}
		digest := digestOf(t, dbPath)
		oldPath := filepath.Join(h.dir, "old-release")
		if err := os.WriteFile(oldPath, oldBinary, 0o755); err != nil { //nolint:gosec // executable test fixture
			t.Fatal(err)
		}
		cmd := exec.Command(oldPath) //nolint:gosec // the previous release this test installed
		cmd.Env = append(os.Environ(), "APP_DEV_MODE=1", "APP_DATA_DIR="+h.dataDir, "APP_MASTER_KEY="+h.mk.String(), "APP_HTTP_ADDR=127.0.0.1:0",
			"APP_AGENT_ADDR=127.0.0.1:0", "APP_INGRESS_HTTP_ADDR=127.0.0.1:0", "APP_INGRESS_HTTPS_ADDR=127.0.0.1:0")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		runErr := cmd.Run()
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != selfupgrade.ExitDowngradeRefused {
			t.Fatalf("old release on the migrated database: exit %v, want %d\n%s", runErr, selfupgrade.ExitDowngradeRefused, stderr.String())
		}
		if !strings.Contains(stderr.String(), "forward-only") {
			t.Errorf("refusal does not explain the recovery:\n%s", stderr.String())
		}
		if digestOf(t, dbPath) != digest {
			t.Error("the refused start modified the database file")
		}
		clearNewerRelease(t, h)
		h.startService()
		assertSeeded(t, h.capture(), "after refused downgrade")
	})
}

// plantNewerRelease stands in for a release whose migrations the previous
// binary does not know, which neighbouring tags cannot provide on their own
// when they share one schema head.
func plantNewerRelease(t *testing.T, h *harnessEnv, schema int) {
	t.Helper()
	db := h.sql()
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name) VALUES (?, 'from_the_future')`, schema); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
}

func clearNewerRelease(t *testing.T, h *harnessEnv) {
	t.Helper()
	db := h.sql()
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE name = 'from_the_future'`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
}

func assertSnapshotHoldsSeed(t *testing.T, snapshot string) {
	t.Helper()
	copyPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.WriteFile(copyPath, readFile(t, snapshot), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+copyPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var check string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&check); err != nil || check != "ok" {
		t.Fatalf("restore point database integrity: %q %v", check, err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM apps WHERE name = ?`, harnessAppName).Scan(&n); err != nil || n != 1 {
		t.Fatalf("restore point database lacks the seeded app: %d %v", n, err)
	}
}
