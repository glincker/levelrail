package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
)

// v082LastMigration is the newest migration the v0.8.1 and v0.8.2 releases ship.
const v082LastMigration = 2830

const (
	schemaTestSMTPUser = "sink-user"
	schemaTestSMTPPass = "sink-pass-0451"
	schemaTestCapBytes = 5368709120
)

// smtpSink is a local SMTP server that accepts any message and records the
// PLAIN credentials it was given, so a test send proves which credential the
// control plane loaded.
type smtpSink struct {
	ln net.Listener

	mu    sync.Mutex
	auths [][2]string
	mails int
}

func newSMTPSink(t *testing.T) *smtpSink {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &smtpSink{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

func (s *smtpSink) port() int { return s.ln.Addr().(*net.TCPAddr).Port }

func (s *smtpSink) lastAuth() (user, pass string, mails int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := len(s.auths); n > 0 {
		user, pass = s.auths[n-1][0], s.auths[n-1][1]
	}
	return user, pass, s.mails
}

func (s *smtpSink) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(30 * time.Second))
	r := bufio.NewReader(c)
	say := func(line string) { _, _ = c.Write([]byte(line + "\r\n")) }
	say("220 sink ready")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		switch verb := strings.ToUpper(strings.Fields(line + " x")[0]); verb {
		case "EHLO", "HELO":
			say("250-sink")
			say("250 AUTH PLAIN")
		case "AUTH":
			parts := strings.Fields(line)
			payload := ""
			if len(parts) >= 3 {
				payload = parts[2]
			} else {
				say("334 ")
				next, err := r.ReadString('\n')
				if err != nil {
					return
				}
				payload = strings.TrimSpace(next)
			}
			raw, _ := base64.StdEncoding.DecodeString(payload)
			if f := strings.Split(string(raw), "\x00"); len(f) == 3 {
				s.mu.Lock()
				s.auths = append(s.auths, [2]string{f[1], f[2]})
				s.mu.Unlock()
			}
			say("235 ok")
		case "DATA":
			say("354 go")
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimSpace(l) == "." {
					break
				}
			}
			s.mu.Lock()
			s.mails++
			s.mu.Unlock()
			say("250 queued")
		case "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func jsonMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return m
}

func (h *harnessEnv) getMap(path string) map[string]any {
	h.t.Helper()
	return jsonMap(h.t, h.mustAPI("GET", path, nil))
}

// sameFields compares the named JSON fields of two responses, treating an
// omitted field and an empty or false one alike.
func sameFields(t *testing.T, label string, a, b map[string]any, keys ...string) {
	t.Helper()
	norm := func(v any) string {
		switch x := v.(type) {
		case nil:
			return ""
		case bool:
			if !x {
				return ""
			}
		case float64:
			if x == 0 {
				return ""
			}
			return strconv.FormatFloat(x, 'f', -1, 64)
		}
		return fmt.Sprint(v)
	}
	for _, k := range keys {
		if norm(a[k]) != norm(b[k]) {
			t.Errorf("%s: %s differs: %v vs %v", label, k, a[k], b[k])
		}
	}
}

// rowMap reads the single row of query as column to text.
func rowMap(t *testing.T, db *sql.DB, query string, args ...any) map[string]string {
	t.Helper()
	rows, err := db.Query(query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer func() { _ = rows.Close() }()
	cols, _ := rows.Columns()
	if !rows.Next() {
		t.Fatalf("%s: no row", query)
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for i, c := range cols {
		out[c] = fmt.Sprint(vals[i])
	}
	return out
}

// hasEmailProvidersTable reports whether the email_providers table is there.
func hasEmailProvidersTable(t *testing.T, db *sql.DB) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'email_providers'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

type migrationRow struct {
	Version   int
	Name      string
	AppliedAt string
}

func appliedMigrations(t *testing.T, db *sql.DB) []migrationRow {
	t.Helper()
	rows, err := db.Query(`SELECT version, name, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []migrationRow
	for rows.Next() {
		var m migrationRow
		if err := rows.Scan(&m.Version, &m.Name, &m.AppliedAt); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

// shippedMigrations lists the migration files of this tree as version to name.
func shippedMigrations(t *testing.T) map[int]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "internal", "store", "migrations", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migration files found: %v", err)
	}
	out := map[int]string{}
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), ".sql")
		num, name, ok := strings.Cut(base, "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil {
			t.Fatalf("unexpected migration file name %q", f)
		}
		if prev, dup := out[v]; dup {
			t.Fatalf("migration version %d shipped twice: %s and %s", v, prev, name)
		}
		out[v] = name
	}
	return out
}

func assertMigrationsMatchShipped(t *testing.T, got []migrationRow, label string) {
	t.Helper()
	shipped := shippedMigrations(t)
	seen := map[int]bool{}
	for _, m := range got {
		if seen[m.Version] {
			t.Errorf("%s: migration %d recorded twice", label, m.Version)
		}
		seen[m.Version] = true
		name, ok := shipped[m.Version]
		switch {
		case !ok:
			t.Errorf("%s: migration %d (%s) is applied but not shipped", label, m.Version, m.Name)
		case name != m.Name:
			t.Errorf("%s: migration %d is recorded as %q, shipped as %q", label, m.Version, m.Name, name)
		}
	}
	var missing []int
	for v := range shipped {
		if !seen[v] {
			missing = append(missing, v)
		}
	}
	sort.Ints(missing)
	if len(missing) > 0 {
		t.Errorf("%s: shipped migrations never applied: %v", label, missing)
	}
}

func dbDigest(t *testing.T, dbPath string) string {
	t.Helper()
	sum := sha256.New()
	for _, p := range []string{dbPath, dbPath + "-wal"} {
		if raw, err := os.ReadFile(p); err == nil { //nolint:gosec // test temp path
			sum.Write(raw)
		}
	}
	return hex.EncodeToString(sum.Sum(nil))
}

const emailCarryColumns = `backend, smtp_host, smtp_port, smtp_username, smtp_from, ses_region, ses_access_key_id, ses_from,
	resend_from, tls_mode, tls_skip_verify, ses_smtp_port, ses_smtp_username, mailgun_domain, mailgun_region`

// schemaFlow holds what the v0.8.x release wrote, for comparison after the upgrade.
type schemaFlow struct {
	h          *harnessEnv
	sink       *smtpSink
	oldSchema  int
	oldRows    []migrationRow
	legacy     map[string]any
	emailRow   map[string]string
	capJSON    map[string]any
	capRow     map[string]string
	oldBinary  []byte
	upgradedAt time.Time
}

func (f *schemaFlow) seedEmailAndCap() {
	t, h := f.h.t, f.h
	h.mustAPI("PUT", "/api/v1/settings/email", map[string]any{
		"backend": "smtp", "smtp_host": "127.0.0.1", "smtp_port": f.sink.port(), "smtp_username": schemaTestSMTPUser,
		"smtp_password": schemaTestSMTPPass, "smtp_from": "sender@example.test", "tls_mode": "none",
		"from_name": "Harness Sender", "from_address": "sender@example.test", "reply_to": "reply@example.test",
		"sends_per_minute": 30, "sends_per_day": 500,
	})
	h.mustAPI("PUT", "/api/v1/apps/"+harnessAppName+"/budget", map[string]any{
		"monthly_egress_bytes": schemaTestCapBytes, "monthly_cost_ceiling": 25.5, "thresholds": []int{50, 80, 100}, "action": "none",
	})
	f.legacy = h.getMap("/api/v1/settings/email")
	if f.legacy["smtp_password_set"] != true {
		t.Fatalf("v0.8.x does not report the stored credential: %v", f.legacy)
	}
	f.capJSON = h.getMap("/api/v1/apps/" + harnessAppName + "/budget")
	db := h.sql()
	f.emailRow = rowMap(t, db, `SELECT `+emailCarryColumns+` FROM email_settings WHERE id = 1`)
	f.capRow = rowMap(t, db, `SELECT app_name, monthly_egress_bytes, monthly_cost_ceiling, thresholds, action, throttle_rps FROM app_spend_caps WHERE app_name = ?`, harnessAppName)
	f.testSendPre()
}

// testSendPre proves the sink and the stored credential work on the old release, so a later failure is the upgrade's.
func (f *schemaFlow) testSendPre() {
	f.h.mustAPI("POST", "/api/v1/settings/email/test", map[string]string{"to": "rcpt@example.test"})
	user, pass, mails := f.sink.lastAuth()
	if user != schemaTestSMTPUser || pass != schemaTestSMTPPass || mails < 1 {
		f.h.t.Fatalf("old release did not send with the stored credential: user=%q pass=%q mails=%d", user, pass, mails)
	}
}

func (f *schemaFlow) assertEmailMigrated(t *testing.T, label string) {
	h := f.h
	t.Helper()
	db := h.sql()
	if !hasEmailProvidersTable(t, db) {
		t.Fatalf("%s: email_providers table is missing", label)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM email_providers`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("%s: email_providers has %d rows, want exactly the backfilled primary: %v", label, n, err)
	}
	prov := rowMap(t, db, `SELECT id, role, enabled, `+emailCarryColumns+` FROM email_providers WHERE id = 'prov_primary'`)
	if prov["role"] != "primary" || prov["enabled"] != "1" {
		t.Errorf("%s: prov_primary role=%s enabled=%s", label, prov["role"], prov["enabled"])
	}
	for col, want := range f.emailRow {
		if prov[col] != want {
			t.Errorf("%s: prov_primary.%s = %q, legacy row had %q", label, col, prov[col], want)
		}
	}
	legacy := rowMap(t, db, `SELECT `+emailCarryColumns+` FROM email_settings WHERE id = 1`)
	if !reflect.DeepEqual(legacy, f.emailRow) {
		t.Errorf("%s: the legacy email_settings row changed: %v vs %v", label, legacy, f.emailRow)
	}

	now := h.getMap("/api/v1/settings/email")
	sameFields(t, label+" legacy GET vs before upgrade", f.legacy, now,
		"backend", "smtp_host", "smtp_port", "smtp_username", "smtp_from", "tls_mode", "from_name", "from_address",
		"reply_to", "sends_per_minute", "sends_per_day", "smtp_password_set")
	var list struct {
		Providers []map[string]any `json:"providers"`
	}
	if err := json.Unmarshal(h.mustAPI("GET", "/api/v1/settings/email/providers", nil), &list); err != nil || len(list.Providers) != 1 {
		t.Fatalf("%s: providers = %v, %v", label, list, err)
	}
	p := list.Providers[0]
	if p["id"] != "prov_primary" || p["role"] != "primary" || p["enabled"] != true {
		t.Errorf("%s: provider = %v", label, p)
	}
	sameFields(t, label+" provider vs legacy GET", now, p,
		"backend", "smtp_host", "smtp_port", "smtp_username", "smtp_from", "tls_mode", "smtp_password_set")
	if p["smtp_password_set"] != true {
		t.Errorf("%s: the primary provider does not report the migrated credential: %v", label, p)
	}

	_, _, before := f.sink.lastAuth()
	sent := jsonMap(t, h.mustAPI("POST", "/api/v1/settings/email/providers/prov_primary/test", map[string]string{"to": "rcpt@example.test"}))
	user, pass, after := f.sink.lastAuth()
	if sent["provider_id"] != "prov_primary" || after != before+1 || pass != schemaTestSMTPPass || user != f.emailUser() {
		t.Errorf("%s: test send via prov_primary: resp=%v user=%q pass=%q mails %d to %d", label, sent, user, pass, before, after)
	}
}

func (f *schemaFlow) emailUser() string { return f.emailRow["smtp_username"] }

// assertLegacyShim drives the deprecated PUT and checks both views follow it without losing the credential.
func (f *schemaFlow) assertLegacyShim(t *testing.T) {
	h := f.h
	t.Helper()
	h.mustAPI("PUT", "/api/v1/settings/email", map[string]any{
		"backend": "smtp", "smtp_host": "127.0.0.1", "smtp_port": f.sink.port(), "smtp_username": "sink-user-2",
		"smtp_from": "sender@example.test", "tls_mode": "none", "from_name": "Harness Sender Two", "from_address": "sender@example.test",
		"reply_to": "reply2@example.test", "sends_per_minute": 30, "sends_per_day": 500,
	})
	legacy := h.getMap("/api/v1/settings/email")
	if legacy["smtp_username"] != "sink-user-2" || legacy["from_name"] != "Harness Sender Two" || legacy["reply_to"] != "reply2@example.test" {
		t.Errorf("legacy PUT did not round-trip: %v", legacy)
	}
	if legacy["smtp_password_set"] != true {
		t.Errorf("legacy PUT without a password dropped the stored credential: %v", legacy)
	}
	var list struct {
		Providers []map[string]any `json:"providers"`
	}
	if err := json.Unmarshal(h.mustAPI("GET", "/api/v1/settings/email/providers", nil), &list); err != nil || len(list.Providers) != 1 {
		t.Fatalf("providers after legacy PUT = %v, %v", list, err)
	}
	sameFields(t, "provider after legacy PUT", legacy, list.Providers[0], "backend", "smtp_host", "smtp_port", "smtp_username", "smtp_from", "tls_mode", "smtp_password_set")
	h.mustAPI("POST", "/api/v1/settings/email/test", map[string]string{"to": "rcpt@example.test"})
	if user, pass, _ := f.sink.lastAuth(); user != "sink-user-2" || pass != schemaTestSMTPPass {
		t.Errorf("send after legacy PUT used user=%q pass=%q, want the new user and the kept credential", user, pass)
	}
	// Put the seeded values back (without resending the password) so later checks compare against the seed.
	h.mustAPI("PUT", "/api/v1/settings/email", map[string]any{
		"backend": "smtp", "smtp_host": "127.0.0.1", "smtp_port": f.sink.port(), "smtp_username": schemaTestSMTPUser,
		"smtp_from": "sender@example.test", "tls_mode": "none", "from_name": "Harness Sender", "from_address": "sender@example.test",
		"reply_to": "reply@example.test", "sends_per_minute": 30, "sends_per_day": 500,
	})
	sameFields(t, "legacy GET after restoring the seed", f.legacy, h.getMap("/api/v1/settings/email"),
		"backend", "smtp_username", "from_name", "reply_to", "smtp_password_set")
}

func (f *schemaFlow) assertSpendCapMigrated(t *testing.T, label string) {
	h := f.h
	t.Helper()
	db := h.sql()
	row := rowMap(t, db, `SELECT app_name, monthly_egress_bytes, monthly_cost_ceiling, thresholds, action, throttle_rps FROM app_spend_caps WHERE app_name = ?`, harnessAppName)
	if !reflect.DeepEqual(row, f.capRow) {
		t.Errorf("%s: spend cap row changed: %v vs %v", label, row, f.capRow)
	}
	if mode := rowMap(t, db, `SELECT guard_mode FROM app_spend_caps WHERE app_name = ?`, harnessAppName)["guard_mode"]; mode != "alert" {
		t.Errorf("%s: guard_mode = %q, want the default alert", label, mode)
	}
	var blocking int
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM app_spend_caps WHERE guard_mode <> 'alert') + (SELECT COUNT(*) FROM cost_budgets WHERE guard_mode <> 'alert') + (SELECT COUNT(*) FROM spend_guard_overrides)`).Scan(&blocking); err != nil || blocking != 0 {
		t.Errorf("%s: %d blocking guard rows or overrides after upgrade: %v", label, blocking, err)
	}
	var events int
	if err := db.QueryRow(`SELECT COUNT(*) FROM app_spend_cap_events WHERE app_name = ? AND kind = 'configured'`, harnessAppName).Scan(&events); err != nil || events < 1 {
		t.Errorf("%s: the configured event of the cap was lost: %d %v", label, events, err)
	}
	cp := h.getMap("/api/v1/apps/" + harnessAppName + "/budget")
	sameFields(t, label+" budget GET", f.capJSON, cp, "app", "monthly_egress_bytes", "monthly_cost_ceiling", "action", "throttle_rps")
	if !reflect.DeepEqual(f.capJSON["thresholds"], cp["thresholds"]) {
		t.Errorf("%s: thresholds changed: %v vs %v", label, f.capJSON["thresholds"], cp["thresholds"])
	}
	if cp["guard_mode"] != "alert" {
		t.Errorf("%s: API guard_mode = %v, want alert", label, cp["guard_mode"])
	}
	guard, _ := cp["guard"].(map[string]any)
	if guard == nil || guard["blocked"] == true || guard["allowed"] != true {
		t.Errorf("%s: deploys of %s are blocked by the guard: %v", label, harnessAppName, cp["guard"])
	}
}

func TestReleaseUpgradeSchemaMigration(t *testing.T) {
	for _, tag := range harnessTags(t) {
		t.Run(tag, func(t *testing.T) { runSchemaMigration(t, tag) })
	}
}

// newSchemaFlow starts the old release on an empty data directory and seeds the legacy email backend and a cap.
func newSchemaFlow(t *testing.T, tag string) *schemaFlow {
	t.Helper()
	h := newHarnessEnv(t, tag)
	f := &schemaFlow{h: h, sink: newSMTPSink(t), oldBinary: readFile(t, h.bin)}
	if got := h.runningVersion(); got != tag {
		t.Fatalf("installed release reports %s, want %s", got, tag)
	}
	h.startService()
	h.seed()
	f.seedEmailAndCap()
	f.oldSchema = h.schema()
	if f.oldSchema > v082LastMigration {
		t.Skipf("%s already ships schema %d, past the %d this test expects; pick an older tag", tag, f.oldSchema, v082LastMigration)
	}
	f.oldRows = appliedMigrations(t, h.sql())
	if hasEmailProvidersTable(t, h.sql()) {
		t.Fatalf("%s already has email_providers, so it cannot show the backfill", tag)
	}
	return f
}

func runSchemaMigration(t *testing.T, tag string) {
	to := harnessTarget()
	f := newSchemaFlow(t, tag)
	h := f.h
	h.publishCurrent(to)
	f.upgradedAt = time.Now().UTC().Add(-2 * time.Second)
	out, err := h.selfUpgrade(to)
	if err != nil {
		log, _ := os.ReadFile(filepath.Join(h.dir, "server.log"))
		t.Fatalf("upgrade %s to %s failed: %v\n%s\nserver log:\n%s", tag, to, err, out, log)
	}
	if got := h.runningVersion(); got != to {
		t.Fatalf("version after upgrade = %s, want %s", got, to)
	}
	if h.schema() <= f.oldSchema {
		t.Fatalf("schema did not advance: %d -> %d", f.oldSchema, h.schema())
	}

	t.Run("email providers backfilled", func(t *testing.T) {
		f.assertEmailMigrated(t, "after upgrade")
	})
	t.Run("legacy email PUT shim round trips", func(t *testing.T) {
		f.assertLegacyShim(t)
	})
	t.Run("spend caps survive", func(t *testing.T) {
		f.assertSpendCapMigrated(t, "after upgrade")
	})
	t.Run("migrations applied exactly once", func(t *testing.T) {
		rows := appliedMigrations(t, h.sql())
		assertMigrationsMatchShipped(t, rows, "after upgrade")
		old := map[int]migrationRow{}
		for _, m := range f.oldRows {
			old[m.Version] = m
		}
		fresh := 0
		for _, m := range rows {
			if prev, ok := old[m.Version]; ok {
				if prev != m {
					t.Errorf("migration %d was applied again: %v then %v", m.Version, prev, m)
				}
				continue
			}
			fresh++
			if m.AppliedAt < f.upgradedAt.Format("2006-01-02T15:04:05") {
				t.Errorf("migration %d (%s) carries applied_at %s from before the upgrade", m.Version, m.Name, m.AppliedAt)
			}
		}
		if want := len(shippedMigrations(t)) - len(f.oldRows); fresh != want || fresh == 0 {
			t.Errorf("%d migrations were newly applied, want %d", fresh, want)
		}
		pointsBefore := len(h.restorePoints())
		_ = exec.Command(h.stopSh).Run() //nolint:gosec // script this test wrote
		h.startService()
		if again := appliedMigrations(t, h.sql()); !reflect.DeepEqual(again, rows) {
			t.Errorf("restarting the current release changed schema_migrations:\n%v\n%v", rows, again)
		}
		if got := len(h.restorePoints()); got != pointsBefore {
			t.Errorf("a no-op start took a snapshot: restore points %d -> %d", pointsBefore, got)
		}
	})

	pts := h.restorePoints()
	if len(pts) != 1 || pts[0].Version != tag || !pts[0].HasBinary {
		t.Fatalf("restore points = %+v, want one holding the %s binary", pts, tag)
	}
	point := pts[0]
	rp := selfupgrade.NewRestorePoints(h.dataDir)

	t.Run("restore point holds the old state", func(t *testing.T) {
		if !bytes.Equal(readFile(t, rp.BinaryPath(point.ID)), f.oldBinary) {
			t.Error("restore point binary is not the previous release byte for byte")
		}
		assertSnapshotHoldsSeed(t, rp.DBPath(point.ID))
		snap := filepath.Join(t.TempDir(), "snapshot.db")
		if err := os.WriteFile(snap, readFile(t, rp.DBPath(point.ID)), 0o600); err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite", "file:"+snap+"?mode=ro")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		if hasEmailProvidersTable(t, db) {
			t.Error("the restore point already has email_providers: it is not the pre-upgrade database")
		}
		if got := rowMap(t, db, `SELECT `+emailCarryColumns+` FROM email_settings WHERE id = 1`); !reflect.DeepEqual(got, f.emailRow) {
			t.Errorf("restore point email row = %v, want %v", got, f.emailRow)
		}
		if got := appliedMigrations(t, db); len(got) != len(f.oldRows) {
			t.Errorf("restore point has %d migrations, the old release had %d", len(got), len(f.oldRows))
		}
	})

	t.Run("older release refuses the upgraded data", func(t *testing.T) {
		installed := readFile(t, h.bin)
		if out, err := h.selfUpgrade(tag); err == nil || !strings.Contains(out, "refused") {
			t.Fatalf("self-upgrade to the older %s was not refused: %v\n%s", tag, err, out)
		}
		if !bytes.Equal(readFile(t, h.bin), installed) {
			t.Fatal("binary changed although the downgrade was refused")
		}

		_ = exec.Command(h.stopSh).Run() //nolint:gosec // script this test wrote
		dbPath := filepath.Join(h.dataDir, "levelrail.db")
		rows := appliedMigrations(t, h.sql())
		digest := dbDigest(t, dbPath)
		oldPath := filepath.Join(h.dir, "old-release")
		if err := os.WriteFile(oldPath, f.oldBinary, 0o755); err != nil { //nolint:gosec // executable test fixture
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
			t.Fatalf("%s on the upgraded database: exit %v, want %d\n%s", tag, runErr, selfupgrade.ExitDowngradeRefused, stderr.String())
		}
		for _, want := range []string{"refusing to start", "older than the database", "forward-only", "the data is intact"} {
			if !strings.Contains(stderr.String(), want) {
				t.Errorf("refusal lacks %q:\n%s", want, stderr.String())
			}
		}
		if got := dbDigest(t, dbPath); got != digest {
			t.Error("the refused start modified the database file")
		}
		if got := appliedMigrations(t, h.sql()); !reflect.DeepEqual(got, rows) {
			t.Error("the refused start changed schema_migrations")
		}
		h.startService()
		assertSeeded(t, h.capture(), "after refused downgrade")
		f.assertEmailMigrated(t, "after refused downgrade")
	})

	t.Run("manual restore returns the old schema and data", func(t *testing.T) {
		out, err := h.run("restore-point", "--id", point.ID, "--stop-cmd", h.stopSh, "--start-cmd", h.startSh, "--health-url", h.health())
		if err != nil {
			t.Fatalf("restore-point: %v\n%s", err, out)
		}
		t.Logf("restore-point output:\n%s", out)
		if !strings.Contains(out, "database restored") {
			t.Errorf("expected the migrated database to be restored:\n%s", out)
		}
		f.assertOldState(t, tag)
	})
}

func TestReleaseUpgradeSchemaAutoRestore(t *testing.T) {
	for _, tag := range harnessTags(t) {
		t.Run(tag, func(t *testing.T) { runSchemaAutoRestore(t, tag) })
	}
}

// schemaOf reads the schema version without failing the test, for use from a server goroutine.
func schemaOf(dataDir string) int {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "levelrail.db")+"?mode=ro")
	if err != nil {
		return -1
	}
	defer func() { _ = db.Close() }()
	var v int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil {
		return -1
	}
	return v
}

// runSchemaAutoRestore upgrades to the real current build, whose migrations run for real, then makes the
// post-upgrade health check report the migrated schema as unhealthy so the old release's own rollback has to put the old schema back.
func runSchemaAutoRestore(t *testing.T, tag string) {
	to := harnessTarget()
	f := newSchemaFlow(t, tag)
	h := f.h
	h.publishCurrent(to)
	installed := readFile(t, h.bin)
	gate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if schemaOf(h.dataDir) > f.oldSchema {
			http.Error(w, "migrated schema", http.StatusServiceUnavailable)
			return
		}
		resp, err := http.Get(h.health() + r.URL.Path) //nolint:noctx,gosec // loopback test proxy
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		w.WriteHeader(resp.StatusCode)
	}))
	defer gate.Close()
	out, err := h.selfUpgrade(to, "--health-url", gate.URL, "--timeout", "10s")
	if err == nil || !strings.Contains(out, "rolled_back") {
		t.Fatalf("an upgrade that never verified healthy was not rolled back: %v\n%s", err, out)
	}
	if !strings.Contains(out, "database restored") {
		t.Errorf("rollback did not report restoring the database:\n%s", out)
	}
	if !bytes.Equal(readFile(t, h.bin), installed) {
		t.Error("previous binary was not restored byte for byte")
	}
	f.assertOldState(t, tag)

	f.upgradedAt = time.Now().UTC().Add(-2 * time.Second)
	if out, err := h.selfUpgrade(to); err != nil {
		t.Fatalf("upgrade after the rollback: %v\n%s", err, out)
	}
	f.assertEmailMigrated(t, "after upgrade following a rollback")
	f.assertSpendCapMigrated(t, "after upgrade following a rollback")
	assertMigrationsMatchShipped(t, appliedMigrations(t, h.sql()), "after upgrade following a rollback")
}

// assertOldState checks the data directory is back at what the old release wrote, running on the old release.
func (f *schemaFlow) assertOldState(t *testing.T, tag string) {
	t.Helper()
	h := f.h
	if got := h.runningVersion(); got != tag {
		t.Fatalf("version after restore = %s, want %s", got, tag)
	}
	if got := h.schema(); got != f.oldSchema {
		t.Fatalf("schema after restore = %d, want %d", got, f.oldSchema)
	}
	db := h.sql()
	if hasEmailProvidersTable(t, db) {
		t.Error("email_providers survived the restore")
	}
	if got := appliedMigrations(t, db); !reflect.DeepEqual(got, f.oldRows) {
		t.Errorf("schema_migrations after restore differs from the old release's:\n%v\n%v", got, f.oldRows)
	}
	if got := rowMap(t, db, `SELECT `+emailCarryColumns+` FROM email_settings WHERE id = 1`); !reflect.DeepEqual(got, f.emailRow) {
		t.Errorf("email row after restore = %v, want %v", got, f.emailRow)
	}
	if got := rowMap(t, db, `SELECT app_name, monthly_egress_bytes, monthly_cost_ceiling, thresholds, action, throttle_rps FROM app_spend_caps WHERE app_name = ?`, harnessAppName); !reflect.DeepEqual(got, f.capRow) {
		t.Errorf("spend cap after restore = %v, want %v", got, f.capRow)
	}
	if got := h.planted(); got != harnessProbeValue {
		t.Errorf("probe row after restore = %q", got)
	}
	sameFields(t, "legacy email GET after restore", f.legacy, h.getMap("/api/v1/settings/email"),
		"backend", "smtp_host", "smtp_port", "smtp_username", "from_name", "reply_to", "smtp_password_set")
	sameFields(t, "budget GET after restore", f.capJSON, h.getMap("/api/v1/apps/"+harnessAppName+"/budget"),
		"monthly_egress_bytes", "monthly_cost_ceiling", "action")
	f.testSendPre()
	names := names(h.mustAPI("GET", "/api/v1/apps", nil), "name")
	if !contains(names, harnessAppName) {
		t.Errorf("seeded app missing after restore: %v", names)
	}
}

// TestUpgradeSchemaMigrationInProcess is the fast form of the live test: it builds the schema v0.8.2 shipped
// with the real migration files, seeds the legacy email row and a spend cap, then migrates to the current tree.
func TestUpgradeSchemaMigrationInProcess(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "levelrail.db")
	old, err := store.Open(ctx, path, store.WithMigrationCeiling(v082LastMigration))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := old.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`PRAGMA foreign_keys = OFF`,
		`UPDATE email_settings SET backend = 'smtp', smtp_host = 'smtp.example.test', smtp_port = 2525, smtp_username = 'u', smtp_from = 'a@example.test', tls_mode = 'starttls', tls_skip_verify = 1 WHERE id = 1`,
		`INSERT INTO app_spend_caps (app_name, monthly_egress_bytes, monthly_cost_ceiling, thresholds, action, throttle_rps, updated_at) VALUES ('web', 1000, 5, '[50,100]', 'none', 0, '2026-01-01T00:00:00Z')`,
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	_ = conn.Close()
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}

	cur, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("migrate to the current tree: %v", err)
	}
	prov, err := cur.GetEmailProvider(ctx, store.EmailPrimaryProviderID)
	if err != nil {
		t.Fatal(err)
	}
	if prov.Role != store.EmailRolePrimary || !prov.Enabled || prov.Backend != "smtp" || prov.SMTPHost != "smtp.example.test" ||
		prov.SMTPPort != 2525 || prov.SMTPUsername != "u" || prov.SMTPFrom != "a@example.test" || prov.TLSMode != "starttls" || !prov.TLSSkipVerify {
		t.Errorf("backfilled primary lost fields: %+v", prov)
	}
	var mode string
	var bytesCap int64
	if err := cur.QueryRowContext(ctx, `SELECT guard_mode, monthly_egress_bytes FROM app_spend_caps WHERE app_name = 'web'`).Scan(&mode, &bytesCap); err != nil || mode != "alert" || bytesCap != 1000 {
		t.Errorf("spend cap after migration: mode=%q bytes=%d err=%v", mode, bytesCap, err)
	}
	rows := appliedMigrations(t, cur.DB)
	assertMigrationsMatchShipped(t, rows, "in process")
	if err := cur.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = again.Close() }()
	if got := appliedMigrations(t, again.DB); !reflect.DeepEqual(got, rows) {
		t.Errorf("reopening the migrated database changed schema_migrations")
	}
}
