package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/integrity"
	"github.com/GLINCKER/levelrail/internal/safemode"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/tokenhash"
	"github.com/GLINCKER/levelrail/internal/upgradehistory"
)

const (
	envUpgradeFixtureDir = "LEVELRAIL_UPGRADE_FIXTURE_DIR"
	matrixTokenPlain     = "upgrade-matrix-token"
	matrixSecretPlain    = "upgrade-matrix-secret-value"
)

// Older schema ceilings the matrix upgrades from; 0 means the current head.
var upgradeMatrixCeilings = []int{1000, 1700, 2080, 2160, 0}

type matrixSeed struct {
	name  string
	write func(ctx context.Context, db *store.DB, mgr *secrets.Manager) error
	check func(ctx context.Context, db *store.DB, mgr *secrets.Manager) error
}

func matrixSeeds() []matrixSeed {
	now := time.Now().UTC()
	return []matrixSeed{
		{
			name: "user",
			write: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				return db.CreateUser(ctx, store.User{ID: "usr_matrix", Email: "matrix@example.test", DisplayName: "Matrix", Abilities: []string{api.AbilityRoot}, IsFirstUser: true, CreatedAt: now})
			},
			check: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				u, err := db.GetUserByEmail(ctx, "matrix@example.test")
				if err != nil {
					return err
				}
				if u.ID != "usr_matrix" {
					return fmt.Errorf("user id %q", u.ID)
				}
				return nil
			},
		},
		{
			name: "api token",
			write: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				return db.SaveAPIToken(ctx, store.APIToken{ID: "tok_matrix", Name: "matrix", TokenHash: tokenhash.Sum(matrixTokenPlain), Abilities: []string{api.AbilityRead}, CreatedAt: now})
			},
			check: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				tok, err := db.GetAPITokenByHash(ctx, tokenhash.Sum(matrixTokenPlain))
				if err != nil {
					return err
				}
				if tok.ID != "tok_matrix" || tok.RevokedAt != nil {
					return fmt.Errorf("token changed: id %q revoked %v", tok.ID, tok.RevokedAt != nil)
				}
				return nil
			},
		},
		{
			name: "app",
			write: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				ts := now.Format(time.RFC3339Nano)
				return db.SaveApp(ctx, store.App{ID: "app_matrix", Name: "matrix-app", CreatedAt: ts, UpdatedAt: ts})
			},
		},
		{
			name: "service with env and domain",
			write: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				return db.SaveDesiredService(ctx, store.DesiredService{
					Name: "matrix-web", Image: "nginx:1.27", Port: 8080,
					Domains: []string{"matrix.example.test"},
					Env:     map[string]string{"MODE": "matrix"},
				})
			},
			check: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				svc, err := db.GetDesiredService(ctx, "matrix-web")
				if err != nil {
					return err
				}
				if svc.Image != "nginx:1.27" || svc.Env["MODE"] != "matrix" || len(svc.Domains) != 1 || svc.Domains[0] != "matrix.example.test" {
					return errors.New("service fields changed")
				}
				return nil
			},
		},
		{
			name: "secret",
			write: func(ctx context.Context, _ *store.DB, mgr *secrets.Manager) error {
				return mgr.SetValue(ctx, "matrix-web", "API_KEY", matrixSecretPlain)
			},
			check: func(ctx context.Context, _ *store.DB, mgr *secrets.Manager) error {
				got, err := mgr.Resolve(ctx, "matrix-web", "API_KEY")
				if err != nil {
					return fmt.Errorf("secret no longer resolves: %w", err)
				}
				if got != matrixSecretPlain {
					return errors.New("secret decrypts to a different value")
				}
				return nil
			},
		},
		{
			name: "deploy attempt",
			write: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				return db.SaveDeployAttempt(ctx, store.DeployAttempt{
					ID: "dpl_matrix", ServiceName: "matrix-web", Image: "nginx:1.27",
					Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusRunning, StartedAt: now,
				})
			},
		},
		{
			name: "audit log",
			write: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				for i := 0; i < 5; i++ {
					id, err := store.NewAuditEntryID()
					if err != nil {
						return err
					}
					e := store.AuditEntry{ID: id, ActorType: "system", ActorID: "matrix", Ability: api.AbilityRead, Method: "GET", Path: fmt.Sprintf("/matrix/%d", i), StatusCode: 200, CreatedAt: store.FormatAuditTime(now.Add(time.Duration(i) * time.Millisecond))}
					if err := db.SaveAuditEntry(ctx, e); err != nil {
						return err
					}
				}
				return nil
			},
			check: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				rep, err := db.VerifyAuditChain(ctx)
				if err != nil {
					return err
				}
				if !rep.OK {
					return fmt.Errorf("audit chain broken after migration: %+v", rep.Problems)
				}
				return nil
			},
		},
		{
			name: "backup target",
			write: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				return db.SaveBackupTarget(ctx, store.BackupTarget{ID: "bt_matrix", Name: "matrix", Provider: "custom", Endpoint: "https://s3.example.test", Region: "auto", Bucket: "matrix", CreatedAt: now.Format(time.RFC3339)})
			},
			check: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				bt, err := db.GetBackupTarget(ctx, "bt_matrix")
				if err != nil {
					return err
				}
				if bt.Bucket != "matrix" {
					return errors.New("backup target changed")
				}
				return nil
			},
		},
		{
			name: "plugin",
			write: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				_, err := db.ExecContext(ctx, `INSERT INTO plugins (name, version, kind, publisher, image, digest, manifest, signature, key_id, state, installed_at, installed_by, updated_at)
					VALUES ('matrix-plugin', '1.0.0', 'dns', 'matrix', 'example.test/p:1', 'sha256:00', '{}', 'sig', 'key', 'installed', ?, 'matrix', ?)`, now.Format(time.RFC3339), now.Format(time.RFC3339))
				return err
			},
			check: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				var v string
				return db.QueryRowContext(ctx, `SELECT version FROM plugins WHERE name = 'matrix-plugin'`).Scan(&v)
			},
		},
		{
			name: "model endpoint",
			write: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				return db.SaveInferenceEndpoint(ctx,
					store.Model{Name: "matrix-model", Engine: "external", ModelRef: "llama"},
					store.InferenceEndpoint{Name: "matrix-model", BaseURL: "http://127.0.0.1:11434", APIType: "openai"})
			},
			check: func(ctx context.Context, db *store.DB, _ *secrets.Manager) error {
				ep, err := db.GetInferenceEndpoint(ctx, "matrix-model")
				if err != nil {
					return err
				}
				if ep.BaseURL != "http://127.0.0.1:11434" {
					return errors.New("endpoint changed")
				}
				return nil
			},
		},
	}
}

func isMissingSchema(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "no such table") || strings.Contains(s, "no such column") || strings.Contains(s, "has no column")
}

func matrixMasterKey(t *testing.T) *secrets.MasterKey {
	t.Helper()
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	return mk
}

// seedOldDatabase builds a database at the given schema ceiling and writes
// every record the old schema can hold; it returns the seeds that landed.
func seedOldDatabase(t *testing.T, path string, ceiling int, mk *secrets.MasterKey) (int, []matrixSeed) {
	t.Helper()
	ctx := context.Background()
	var opts []store.OpenOption
	if ceiling > 0 {
		opts = append(opts, store.WithMigrationCeiling(ceiling))
	}
	db, err := store.Open(ctx, path, opts...)
	if err != nil {
		t.Fatalf("open at ceiling %d: %v", ceiling, err)
	}
	defer func() { _ = db.Close() }()
	mgr := secrets.NewManager(db, mk)
	var landed []matrixSeed
	for _, s := range matrixSeeds() {
		if err := s.write(ctx, db, mgr); err != nil {
			if isMissingSchema(err) {
				continue
			}
			t.Fatalf("seed %s at ceiling %d: %v", s.name, ceiling, err)
		}
		landed = append(landed, s)
	}
	var version int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version, landed
}

func tableCounts(t *testing.T, path string) map[string]int {
	t.Helper()
	sdb, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sdb.Close() }()
	rows, err := sdb.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	_ = rows.Close()
	out := make(map[string]int, len(names))
	for _, n := range names {
		var c int
		if err := sdb.QueryRow(`SELECT COUNT(*) FROM "` + n + `"`).Scan(&c); err != nil {
			t.Fatal(err)
		}
		out[n] = c
	}
	return out
}

func assertNoRowLoss(t *testing.T, before, after map[string]int) {
	t.Helper()
	var names []string
	for n := range before {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if n == "schema_migrations" || n == "upgrade_history" {
			continue
		}
		post, ok := after[n]
		if !ok {
			continue
		}
		if post < before[n] {
			t.Errorf("table %s lost rows: %d before, %d after", n, before[n], post)
		}
	}
}

func matrixRouter(t *testing.T, db *store.DB) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := api.NewRouter(logger, &brand.Brand{Name: "Test Platform", BinaryName: "testplatform"}, db, api.WithReadinessProbes(api.ReadinessProbes{
		Database: db.PingContext,
		Migrations: func(ctx context.Context) error {
			var v int
			return db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v)
		},
	}))
	return rt.Handler()
}

func matrixGet(h http.Handler, path, bearer string) int {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestUpgradeMatrixForwardMigration(t *testing.T) {
	ceilings := upgradeMatrixCeilings
	if testing.Short() {
		ceilings = ceilings[len(ceilings)-2:]
	}
	head, err := store.MaxSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	for _, ceiling := range ceilings {
		name := fmt.Sprintf("from_%d", ceiling)
		if ceiling == 0 {
			name = "from_head"
		}
		t.Run(name, func(t *testing.T) { runUpgradeFrom(t, ceiling, head) })
	}
}

func runUpgradeFrom(t *testing.T, ceiling, head int) {
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv("APP_DATA_DIR", dir)
	path := filepath.Join(dir, storeFilename)
	mk := matrixMasterKey(t)

	oldVersion, landed := seedOldDatabase(t, path, ceiling, mk)
	if len(landed) < 2 {
		t.Fatalf("only %d seeds fit schema %d, fixture is too thin", len(landed), oldVersion)
	}
	names := make([]string, 0, len(landed))
	for _, s := range landed {
		names = append(names, s.name)
	}
	t.Logf("schema %d seeded: %s", oldVersion, strings.Join(names, ", "))
	before := tableCounts(t, path)
	if out := os.Getenv(envUpgradeFixtureDir); out != "" {
		if err := os.MkdirAll(out, 0o750); err != nil { //nolint:gosec // test-only fixture dir from an env var
			t.Fatal(err)
		}
		if _, _, err := mustFileCopy(path, filepath.Join(out, fmt.Sprintf("schema-%04d.db", oldVersion))); err != nil {
			t.Fatal(err)
		}
	}

	obs := upgradehistory.Observed{SchemaBefore: -1}
	db, err := openStoreObserved(ctx, &obs)
	if err != nil {
		t.Fatalf("forward migration from %d: %v", oldVersion, err)
	}
	mgr := secrets.NewManager(db, mk)

	var after int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != head {
		t.Fatalf("schema after upgrade = %d, want head %d", after, head)
	}

	for _, s := range landed {
		if s.check == nil {
			continue
		}
		if err := s.check(ctx, db, mgr); err != nil {
			t.Errorf("%s after upgrade from %d: %v", s.name, oldVersion, err)
		}
	}

	h := matrixRouter(t, db)
	if code := matrixGet(h, "/healthz", ""); code != http.StatusOK {
		t.Errorf("/healthz = %d on the migrated database", code)
	}
	if code := matrixGet(h, "/readyz", ""); code != http.StatusOK {
		t.Errorf("/readyz = %d on the migrated database", code)
	}
	if hasSeed(landed, "api token") {
		if code := matrixGet(h, "/api/v1/apps", matrixTokenPlain); code != http.StatusOK {
			t.Errorf("pre-upgrade token rejected after migration: status %d", code)
		}
	}
	if code := matrixGet(h, "/api/v1/apps", "not-a-real-token"); code == http.StatusOK {
		t.Error("an unknown token must never authenticate")
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	assertNoRowLoss(t, before, tableCounts(t, path))

	if oldVersion < head {
		assertSnapshotRestores(t, dir, obs, oldVersion, landed, mk)
	}
	assertMigrationIdempotent(t, dir, path, head)
}

func hasSeed(seeds []matrixSeed, name string) bool {
	for _, s := range seeds {
		if s.name == name {
			return true
		}
	}
	return false
}

func mustFileCopy(src, dst string) (int64, string, error) {
	b, err := os.ReadFile(src) //nolint:gosec // test-controlled path
	if err != nil {
		return 0, "", err
	}
	if err := os.WriteFile(dst, b, 0o600); err != nil { //nolint:gosec // test-only fixture path
		return 0, "", err
	}
	return store.FileSHA256(dst)
}

func assertSnapshotRestores(t *testing.T, dir string, obs upgradehistory.Observed, oldVersion int, landed []matrixSeed, mk *secrets.MasterKey) {
	t.Helper()
	ctx := context.Background()
	if obs.SchemaBefore != oldVersion {
		t.Errorf("upgrade recorded schema before = %d, want %d", obs.SchemaBefore, oldVersion)
	}
	if obs.BackupName == "" {
		t.Fatal("no pre-migration snapshot was taken")
	}
	snap := filepath.Join(dir, cpbackup.DirName, obs.BackupName)
	got, err := store.InspectSnapshot(ctx, snap)
	if err != nil {
		t.Fatalf("pre-migration snapshot is not a valid database: %v", err)
	}
	if got != oldVersion {
		t.Fatalf("snapshot schema = %d, want the pre-upgrade %d", got, oldVersion)
	}

	if _, _, err := integrity.Swap(ctx, dir, storeFilename, snap, time.Now()); err != nil {
		t.Fatalf("restore from the pre-migration snapshot: %v", err)
	}
	live := filepath.Join(dir, storeFilename)
	st, err := store.ReadSchemaState(ctx, live)
	if err != nil || st.Schema != oldVersion {
		t.Fatalf("restored schema = %d (err %v), want %d", st.Schema, err, oldVersion)
	}

	db, err := store.Open(ctx, live)
	if err != nil {
		t.Fatalf("migrating the restored database again: %v", err)
	}
	defer func() { _ = db.Close() }()
	mgr := secrets.NewManager(db, mk)
	for _, s := range landed {
		if s.check == nil {
			continue
		}
		if err := s.check(ctx, db, mgr); err != nil {
			t.Errorf("%s after restore and re-upgrade: %v", s.name, err)
		}
	}
}

func assertMigrationIdempotent(t *testing.T, dir, path string, head int) {
	t.Helper()
	t.Setenv("APP_DATA_DIR", dir)
	before := tableCounts(t, path)
	obs := upgradehistory.Observed{SchemaBefore: -1}
	db, err := openStoreObserved(context.Background(), &obs)
	if err != nil {
		t.Fatalf("second open after migration: %v", err)
	}
	_ = db.Close()
	if obs.SchemaBefore != -1 || obs.BackupName != "" {
		t.Errorf("re-open migrated again: %+v", obs)
	}
	st, err := store.ReadSchemaState(context.Background(), path)
	if err != nil || st.Schema != head {
		t.Fatalf("schema after re-open = %d (err %v), want %d", st.Schema, err, head)
	}
	for n, c := range tableCounts(t, path) {
		if n != "upgrade_history" && before[n] != c {
			t.Errorf("table %s changed on an idempotent re-open: %d to %d", n, before[n], c)
		}
	}
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	_, sum, err := store.FileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	return sum
}

func TestUpgradeRefusesDowngradeWithoutTouchingDatabase(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv("APP_DATA_DIR", dir)
	path := filepath.Join(dir, storeFilename)
	head, err := store.MaxSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	seedOldDatabase(t, path, 0, matrixMasterKey(t))

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `INSERT INTO schema_migrations (version, name) VALUES (?, 'from_the_future')`, head+10); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	digest := fileDigest(t, path)
	counts := tableCounts(t, path)

	s := &bootState{ctx: ctx, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	err = s.openStore()
	var refusal *downgradeRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("openStore on a newer database = %v, want a downgrade refusal", err)
	}
	for _, want := range []string{fmt.Sprint(head + 10), "nothing was changed", "Fix"} {
		if !strings.Contains(refusal.msg, want) {
			t.Errorf("refusal message lacks %q:\n%s", want, refusal.msg)
		}
	}
	if selfupgrade.ExitDowngradeRefused != 78 {
		t.Errorf("exit code = %d, want 78 (the unit's RestartPreventExitStatus)", selfupgrade.ExitDowngradeRefused)
	}

	if got := fileDigest(t, path); got != digest {
		t.Error("the refused start modified the database file")
	}
	after := tableCounts(t, path)
	for n, c := range counts {
		if after[n] != c {
			t.Errorf("table %s changed during a refused start: %d to %d", n, c, after[n])
		}
	}
	if _, err := os.Stat(filepath.Join(dir, cpbackup.DirName)); err == nil {
		t.Error("a refused start must not take a snapshot")
	}
}

func TestUpgradeMigrationFailureLeavesRecoverableState(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv("APP_DATA_DIR", dir)
	path := filepath.Join(dir, storeFilename)
	mk := matrixMasterKey(t)

	oldVersion, landed := seedOldDatabase(t, path, 2160, mk)

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `CREATE TABLE inference_endpoints (collision TEXT)`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	before := tableCounts(t, path)

	obs := upgradehistory.Observed{SchemaBefore: -1}
	db, err := openStoreObserved(ctx, &obs)
	if err == nil {
		_ = db.Close()
		t.Fatal("expected the colliding migration to fail")
	}
	if !strings.Contains(err.Error(), "2220") {
		t.Fatalf("error should name the failing migration: %v", err)
	}

	st, rerr := store.ReadSchemaState(ctx, path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if st.Schema <= oldVersion || st.Schema >= 2220 {
		t.Fatalf("schema = %d, want committed migrations kept and the failed one absent (old %d)", st.Schema, oldVersion)
	}
	for n, c := range before {
		if n == "schema_migrations" || n == "ingress_edge_settings" {
			continue
		}
		if got := tableCounts(t, path)[n]; got != c {
			t.Errorf("table %s changed by a failed migration: %d to %d", n, c, got)
		}
	}

	if obs.BackupName == "" {
		t.Fatal("snapshot missing after a failed migration")
	}
	snap := filepath.Join(dir, cpbackup.DirName, obs.BackupName)
	if v, err := store.InspectSnapshot(ctx, snap); err != nil || v != oldVersion {
		t.Fatalf("snapshot schema = %d (err %v), want %d", v, err, oldVersion)
	}

	if entry := classifyStoreError(err, 1).entry; entry.Reason != safemode.ReasonMigrationFailed {
		t.Errorf("failed migration enters safe mode as %q, want %q", entry.Reason, safemode.ReasonMigrationFailed)
	}

	if _, _, err := integrity.Swap(ctx, dir, storeFilename, snap, time.Now()); err != nil {
		t.Fatalf("restore after a failed migration: %v", err)
	}
	restored, err := store.Open(ctx, path, store.WithMigrationCeiling(oldVersion))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	mgr := secrets.NewManager(restored, mk)
	for _, s := range landed {
		if s.check == nil {
			continue
		}
		if err := s.check(ctx, restored, mgr); err != nil {
			t.Errorf("%s after restore: %v", s.name, err)
		}
	}
}
