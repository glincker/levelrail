package authengine_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/scripts/auth-rehearsal/rehearsal"
)

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v >= 0 {
		return v
	}
	return def
}

type rig struct {
	t   *testing.T
	ctx context.Context
	dir string
	bin string
	m   *rehearsal.Manifest
}

func (r *rig) snapshot(dir string, engine bool) *rehearsal.Snapshot {
	r.t.Helper()
	db, err := store.Open(r.ctx, filepath.Join(dir, rehearsal.StoreFile))
	if err != nil {
		r.t.Fatalf("open store: %v", err)
	}
	defer func() { _ = db.Close() }()
	s, err := rehearsal.TakeSnapshot(r.ctx, db.DB, engine)
	if err != nil {
		r.t.Fatalf("snapshot: %v", err)
	}
	return s
}

func (r *rig) noEngineRows(dir, when string) {
	r.t.Helper()
	if n := r.snapshot(dir, true).EngineRowTotal(); n != 0 {
		r.t.Fatalf("%s left %d library or mapping rows behind", when, n)
	}
}

func (r *rig) expectCounts(rep rehearsal.CLIReport, out string) {
	r.t.Helper()
	e := r.m.Expected
	got := [...]int{rep.Users, rep.Passwords, rep.Tokens, rep.TokensSkipped, rep.Passkeys, rep.TOTP, rep.RecoveryNotMoved}
	want := [...]int{e.Users, e.Passwords, e.Tokens, e.TokensSkipped, e.Passkeys, e.TOTP, e.ExpectedRecovered}
	if got != want {
		r.t.Fatalf("counts users,passwords,tokens,skipped,passkeys,totp,recovery = %v, seed manifest says %v\n%s", got, want, out)
	}
	if rep.OAuth >= 0 && rep.OAuth != e.OAuth {
		r.t.Fatalf("oauth identities reported %d, seed manifest says %d", rep.OAuth, e.OAuth)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRehearsal(t *testing.T) {
	if os.Getenv("APP_REHEARSAL") == "" || testing.Short() {
		t.Skip("set APP_REHEARSAL=1 (and omit -short) to run the backfill rehearsal")
	}
	ctx := context.Background()
	dir := os.Getenv("APP_REHEARSAL_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	cfg := rehearsal.DefaultConfig(filepath.Join(dir, "main"))
	if v, err := strconv.ParseUint(os.Getenv("APP_REHEARSAL_SEED"), 10, 64); err == nil {
		cfg.Seed = v
	}
	cfg.Users = envInt("APP_REHEARSAL_USERS", cfg.Users)
	cfg.Tokens = envInt("APP_REHEARSAL_TOKENS", cfg.Tokens)
	cfg.Passkeys = envInt("APP_REHEARSAL_PASSKEYS", cfg.Passkeys)
	cfg.TOTPUsers = envInt("APP_REHEARSAL_TOTP_USERS", cfg.TOTPUsers)
	cfg.CaseDupes = envInt("APP_REHEARSAL_CASE_DUPES", cfg.CaseDupes)
	cfg.OAuth = envInt("APP_REHEARSAL_OAUTH", cfg.OAuth)
	cfg.BcryptCost = envInt("APP_REHEARSAL_BCRYPT_COST", cfg.BcryptCost)

	r := &rig{t: t, ctx: ctx, dir: cfg.Dir}
	t.Run("seed", func(t *testing.T) {
		start := time.Now()
		m, err := rehearsal.Seed(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		r.m = m
		bin, err := rehearsal.BuildBinary(ctx, repoRoot(t), dir)
		if err != nil {
			t.Fatal(err)
		}
		r.bin = bin
		t.Logf("seeded %d users, %d tokens, %d passkeys, %d totp, %d oauth in %s (tokens by state %v)",
			m.Expected.Users, len(m.Tokens), m.Expected.Passkeys, m.Expected.TOTP, m.Expected.OAuth, time.Since(start).Round(time.Millisecond), m.Expected.TokensByState)
	})
	if r.m == nil {
		t.FailNow()
	}
	r.collisionPolicy(t)
	r.dryRun(t)
	r.faultInjection(t, dir)
	r.realRun(t)
	r.parity(t)
	r.rollbackDrill(t)
	if scale := envInt("APP_REHEARSAL_SCALE", 0); scale > 1 {
		r.scaleRun(t, dir, cfg, scale)
	}
}

func (r *rig) collisionPolicy(t *testing.T) {
	t.Run("case_duplicate_policy", func(t *testing.T) {
		n := r.m.Expected.CaseCollisions
		for _, dry := range []bool{true, false} {
			res := rehearsal.RunBackfill(r.ctx, r.bin, r.dir, dry)
			if n == 0 {
				continue
			}
			want := fmt.Sprintf("%d user emails collide", n)
			if res.Err == nil || !strings.Contains(res.Output, want) {
				t.Fatalf("dry=%v: want refusal containing %q, got err=%v\n%s", dry, want, res.Err, res.Output)
			}
		}
		r.noEngineRows(r.dir, "a refused backfill")
		db, err := store.Open(r.ctx, filepath.Join(r.dir, rehearsal.StoreFile))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		if err := rehearsal.ResolveCaseDuplicates(r.ctx, db.DB, r.m); err != nil {
			t.Fatal(err)
		}
		t.Logf("policy: the backfill refuses all-or-nothing on %d case-insensitive email collisions; renamed the later duplicates and continued", n)
	})
}

func (r *rig) dryRun(t *testing.T) {
	t.Run("dry_run_writes_nothing", func(t *testing.T) {
		before := r.snapshot(r.dir, false)
		res := rehearsal.RunBackfill(r.ctx, r.bin, r.dir, true)
		if res.Err != nil {
			t.Fatalf("dry run failed: %v\n%s", res.Err, res.Output)
		}
		r.expectCounts(rehearsal.ParseReport(res.Output), res.Output)
		r.noEngineRows(r.dir, "the dry run")
		if d := rehearsal.DiffLegacy(before, r.snapshot(r.dir, false)); len(d) > 0 {
			t.Fatalf("dry run changed legacy tables: %v", d)
		}
		t.Logf("dry run ok in %s, peak RSS %d MiB", res.Wall.Round(time.Millisecond), res.PeakRSS>>20)
	})
}

func (r *rig) faultInjection(t *testing.T, dir string) {
	faults := map[string]string{
		"bad_token_ability_after_users": `INSERT INTO api_tokens (id, name, token_hash, abilities, created_at, agent_name, agent_description, owner_user_id)
			VALUES ('tok_fault', 'fault', '` + strings.Repeat("ab", 32) + `', '["bogus"]', '2999-01-01T00:00:00Z', '', '', 'user_legacy_admin')`,
		"bad_passkey_after_tokens": `INSERT INTO user_passkeys (id, user_id, credential_id, public_key, sign_count, aaguid, transports, label, backup_eligible, backup_state, created_at)
			VALUES ('pk_fault', 'user_legacy_admin', '!!not base64!!', x'00', 0, '', '', 'fault', 0, 0, '2999-01-01T00:00:00Z')`,
	}
	for name, stmt := range faults {
		t.Run("rollback_"+name, func(t *testing.T) {
			fdir := filepath.Join(dir, "fault-"+name)
			if err := os.MkdirAll(fdir, 0o750); err != nil { //nolint:gosec // test temp dir
				t.Fatal(err)
			}
			src, err := store.Open(r.ctx, filepath.Join(r.dir, rehearsal.StoreFile))
			if err != nil {
				t.Fatal(err)
			}
			_, err = src.ExecContext(r.ctx, "VACUUM INTO ?", filepath.Join(fdir, rehearsal.StoreFile))
			_ = src.Close()
			if err != nil {
				t.Fatal(err)
			}
			key, err := os.ReadFile(filepath.Join(r.dir, rehearsal.MasterKeyFile))
			if err != nil || os.WriteFile(filepath.Join(fdir, rehearsal.MasterKeyFile), key, 0o600) != nil { //nolint:gosec // test temp dir
				t.Fatalf("copy master key: %v", err)
			}
			db, err := store.Open(r.ctx, filepath.Join(fdir, rehearsal.StoreFile))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			if _, err := db.ExecContext(r.ctx, stmt); err != nil {
				t.Fatalf("inject fault: %v", err)
			}
			mgr, err := rehearsal.LoadManager(db, fdir)
			if err != nil {
				t.Fatal(err)
			}
			ek, err := authengine.LoadOrCreateKey(r.ctx, mgr, false)
			if err != nil {
				t.Fatal(err)
			}
			before, err := rehearsal.TakeSnapshot(r.ctx, db.DB, false)
			if err != nil {
				t.Fatal(err)
			}
			_, bfErr := authengine.Backfill(r.ctx, db.DB, authengine.BackfillOptions{
				Secrets: mgr, EncryptionKey: ek, TOTPSecretService: store.UserTOTPSecretsKey, TOTPSecretKey: rehearsal.TOTPSecretKey,
			})
			if bfErr == nil {
				t.Fatal("backfill must fail on the injected row")
			}
			after, err := rehearsal.TakeSnapshot(r.ctx, db.DB, true)
			if err != nil {
				t.Fatal(err)
			}
			if n := after.EngineRowTotal(); n != 0 {
				t.Fatalf("failed backfill left %d library or mapping rows (%v)", n, after.Rows)
			}
			legacy, err := rehearsal.TakeSnapshot(r.ctx, db.DB, false)
			if err != nil {
				t.Fatal(err)
			}
			if d := rehearsal.DiffExact(before, legacy); len(d) > 0 {
				t.Fatalf("failed backfill changed legacy tables: %v", d)
			}
			t.Logf("fault %q: backfill failed (%v), zero library rows, legacy unchanged", name, bfErr)
		})
	}
}

func (r *rig) realRun(t *testing.T) {
	t.Run("real_run_then_idempotent_rerun", func(t *testing.T) {
		legacyBefore := r.snapshot(r.dir, false)
		stop, done := make(chan struct{}), make(chan map[int]bool, 1)
		go func() {
			seen, _ := rehearsal.ObservedCounts(r.ctx, filepath.Join(r.dir, rehearsal.StoreFile), stop)
			done <- seen
		}()
		res := rehearsal.RunBackfill(r.ctx, r.bin, r.dir, false)
		close(stop)
		seen := <-done
		if res.Err != nil {
			t.Fatalf("backfill failed: %v\n%s", res.Err, res.Output)
		}
		rep := rehearsal.ParseReport(res.Output)
		r.expectCounts(rep, res.Output)
		for n := range seen {
			if n != 0 && n != r.m.Expected.Users {
				t.Errorf("a concurrent reader saw %d library users mid run (want only 0 or %d): not one transaction", n, r.m.Expected.Users)
			}
		}
		if d := rehearsal.DiffLegacy(legacyBefore, r.snapshot(r.dir, false)); len(d) > 0 {
			t.Fatalf("backfill changed legacy tables: %v", d)
		}
		t.Logf("TIMING default size: backfill wall %s, peak RSS %d MiB, reader observed counts %v\n%s", res.Wall.Round(time.Millisecond), res.PeakRSS>>20, keys(seen), res.Output)

		engineBefore := r.snapshot(r.dir, true)
		res2 := rehearsal.RunBackfill(r.ctx, r.bin, r.dir, false)
		if res2.Err != nil {
			t.Fatalf("second run failed: %v\n%s", res2.Err, res2.Output)
		}
		rep2 := rehearsal.ParseReport(res2.Output)
		if rep2.Users != 0 || rep2.Passwords != 0 || rep2.Tokens != 0 || rep2.Passkeys != 0 || rep2.TOTP != 0 || rep2.AlreadyMapped != r.m.Expected.Users || rep2.OAuth > 0 {
			t.Fatalf("second run copied again: %+v\n%s", rep2, res2.Output)
		}
		if d := rehearsal.DiffExact(engineBefore, r.snapshot(r.dir, true)); len(d) > 0 {
			t.Fatalf("second run changed library tables: %v", d)
		}
		t.Logf("idempotent: second run copied nothing and changed no library table (wall %s)", res2.Wall.Round(time.Millisecond))
	})
}

func keys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func (r *rig) parity(t *testing.T) {
	t.Run("parity", func(t *testing.T) {
		env, err := rehearsal.NewEnv(r.ctx, r.dir)
		if err != nil {
			t.Fatal(err)
		}
		defer env.Close()
		users := rehearsal.SampleUsers(r.m, envInt("APP_REHEARSAL_SAMPLE_USERS", 250))
		toks := rehearsal.SampleTokens(r.m, envInt("APP_REHEARSAL_SAMPLE_TOKENS", 600))
		t.Logf("sampling %d users and %d tokens", len(users), len(toks))
		legacyBefore := r.snapshot(r.dir, false)
		var bad []string
		start := time.Now()
		bad = append(bad, env.CheckUsers(r.ctx, users, 6)...)
		t.Logf("user parity took %s", time.Since(start).Round(time.Millisecond))
		bad = append(bad, env.CheckTokens(r.ctx, r.m, toks)...)
		bad = append(bad, env.CheckPasskeys(r.ctx, r.m)...)
		var codes int
		if err := env.DB.QueryRowContext(r.ctx, `SELECT COUNT(*) FROM theauth_totp_recovery_codes`).Scan(&codes); err != nil || codes != 0 {
			bad = append(bad, fmt.Sprintf("recovery codes: %d library rows (err %v), want 0 (not converted)", codes, err))
		}
		if rep := rehearsal.ParseReport(rehearsal.RunBackfill(r.ctx, r.bin, r.dir, true).Output); rep.OAuth < 0 {
			t.Log("oauth identity parity SKIPPED: the backfill does not report an oauth identity count yet")
		} else {
			bad = append(bad, env.CheckOAuth(r.ctx, r.m)...)
		}
		if d := rehearsal.DiffLegacy(legacyBefore, r.snapshot(r.dir, false)); len(d) > 0 {
			bad = append(bad, fmt.Sprintf("library sign-ins and token checks changed legacy tables: %v", d))
		}
		for _, b := range bad {
			t.Error(b)
		}
	})
}

func (r *rig) rollbackDrill(t *testing.T) {
	t.Run("rollback_to_legacy", func(t *testing.T) {
		db, err := store.Open(r.ctx, filepath.Join(r.dir, rehearsal.StoreFile))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		before, err := rehearsal.TakeSnapshot(r.ctx, db.DB, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv(authengine.EnvEngine, authengine.EngineLibrary)
		t.Setenv(authengine.EnvAreas, "tokens")
		if !authengine.AreaActive(authengine.AreaTokens) || authengine.AreaActive(authengine.AreaSessions) {
			t.Fatal("area gating did not follow APP_AUTH_ENGINE_AREAS")
		}
		t.Setenv(authengine.EnvAreas, "")
		t.Setenv(authengine.EnvEngine, authengine.EngineLegacy)
		for _, a := range authengine.AllAreas {
			if authengine.AreaActive(a) {
				t.Fatalf("area %s still active after flipping the engine back to legacy", a)
			}
		}
		t.Setenv(authengine.EnvEngine, authengine.EngineLibrary)
		t.Setenv(authengine.EnvAreas, "device")
		if authengine.AreaActive(authengine.AreaTokens) || authengine.AreaActive(authengine.AreaSessions) {
			t.Fatal("flipping the tokens and sessions areas off left them active")
		}
		t.Setenv(authengine.EnvEngine, authengine.EngineLegacy)

		h := api.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), &brand.Brand{Name: "rehearsal", ShortName: "rh", BinaryName: "rehearsal"}, db).Handler()
		logins, tokens, orphansAccepted := 0, 0, 0
		for _, u := range rehearsal.SampleUsers(r.m, 40) {
			if u.Password == "" || strings.Contains(u.Email, "ü") {
				continue
			}
			code, body := legacyPost(h, "/api/v1/auth/login", fmt.Sprintf(`{"username":%q,"password":%q}`, u.Email, u.Password))
			if code != http.StatusOK {
				t.Fatalf("legacy login for %s after the backfill: %d %s", u.ID, code, body)
			}
			logins++
		}
		for _, tk := range rehearsal.SampleTokens(r.m, 60) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
			req.Header.Set("Authorization", "Bearer "+tk.Raw)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			dead := tk.State == rehearsal.StateExpired || tk.State == rehearsal.StateRevoked
			if tk.State == rehearsal.StateOrphan {
				if rec.Code != http.StatusUnauthorized {
					orphansAccepted++
				}
			} else if dead != (rec.Code == http.StatusUnauthorized) {
				t.Fatalf("legacy token %s (%s) status %d", tk.ID, tk.State, rec.Code)
			}
			tokens++
		}
		after, err := rehearsal.TakeSnapshot(r.ctx, db.DB, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range rehearsal.DiffLegacy(before, after) {
			if !strings.HasPrefix(d, "users changed (") && !strings.HasPrefix(d, "api_tokens changed (") {
				t.Fatalf("legacy table changed during the rollback drill: %s", d)
			}
		}
		for _, tbl := range []string{"users", "api_tokens"} {
			if before.Rows[tbl] != after.Rows[tbl] {
				t.Fatalf("%s row count changed during the rollback drill", tbl)
			}
		}
		t.Logf("rollback drill: %d legacy logins and %d legacy token checks behaved as before the backfill; legacy still accepts %d sampled orphaned-owner tokens that the library rejects", logins, tokens, orphansAccepted)
	})
}

func legacyPost(h http.Handler, path, body string) (int, string) {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func (r *rig) scaleRun(t *testing.T, dir string, cfg rehearsal.Config, scale int) {
	t.Run("scale_timing", func(t *testing.T) {
		big := cfg.Scaled(scale)
		big.Dir = filepath.Join(dir, "scale")
		start := time.Now()
		m, err := rehearsal.Seed(r.ctx, big)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("seeded %dx size (%d users, %d tokens) in %s", scale, m.Expected.Users, len(m.Tokens), time.Since(start).Round(time.Millisecond))
		db, err := store.Open(r.ctx, filepath.Join(big.Dir, rehearsal.StoreFile))
		if err != nil {
			t.Fatal(err)
		}
		if err := rehearsal.ResolveCaseDuplicates(r.ctx, db.DB, m); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
		res := rehearsal.RunBackfill(r.ctx, r.bin, big.Dir, false)
		if res.Err != nil {
			t.Fatalf("scaled backfill failed: %v\n%s", res.Err, res.Output)
		}
		t.Logf("TIMING %dx size: backfill wall %s, peak RSS %d MiB", scale, res.Wall.Round(time.Millisecond), res.PeakRSS>>20)
	})
}
