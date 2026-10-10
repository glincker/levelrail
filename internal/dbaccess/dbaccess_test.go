package dbaccess

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateRoleName(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"app_reader", false},
		{"a1b", false},
		{"ab", true},
		{"App", true},
		{"1abc", true},
		{"has-dash", true},
		{`quo"te`, true},
		{"pg_monitor", true},
		{"tmp_abc123", true},
		{"postgres", true},
		{"admin", true},
		{"mydb", true}, // the platform admin role below
		{strings.Repeat("a", 41), true},
		{strings.Repeat("a", 40), false},
	}
	for _, c := range cases {
		err := ValidateRoleName(c.name, "mydb")
		if (err != nil) != c.wantErr {
			t.Errorf("ValidateRoleName(%q) err = %v, wantErr %v", c.name, err, c.wantErr)
		}
		if err != nil && !errors.Is(err, ErrRoleName) {
			t.Errorf("ValidateRoleName(%q) error does not wrap ErrRoleName: %v", c.name, err)
		}
	}
}

func TestProtected(t *testing.T) {
	cases := []struct {
		r    Role
		want bool
	}{
		{Role{Name: "mydb"}, true},
		{Role{Name: "someone", Superuser: true}, true},
		{Role{Name: "pg_read_all_data"}, true},
		{Role{Name: "postgres"}, true},
		{Role{Name: "app_reader"}, false},
	}
	for _, c := range cases {
		if got := Protected(c.r, "mydb"); got != c.want {
			t.Errorf("Protected(%q) = %v, want %v", c.r.Name, got, c.want)
		}
	}
}

func TestCreateRoleSQLPresets(t *testing.T) {
	verifier := "SCRAM-SHA-256$4096:c2FsdA==$c3RvcmVk:c2VydmVy"
	until := time.Date(2026, 10, 9, 12, 30, 0, 0, time.UTC)
	cases := []struct {
		preset  Preset
		want    []string
		wantNot []string
	}{
		{PresetReadOnly, []string{`GRANT SELECT ON ALL TABLES IN SCHEMA public TO "r1"`, `default_transaction_read_only = on`, `CONNECTION LIMIT 5`, `VALID UNTIL '2026-10-09 12:30:00+00'`}, []string{"INSERT", "ALL PRIVILEGES"}},
		{PresetReadWrite, []string{`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO "r1"`}, []string{"ALL PRIVILEGES", "read_only"}},
		{PresetOwner, []string{`GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO "r1"`}, []string{" SUPERUSER", " CREATEROLE"}},
	}
	for _, c := range cases {
		sql, err := CreateRoleSQL(CreateParams{Role: "r1", Database: "my-db", AdminRole: "my-db", Preset: c.preset, Verifier: verifier, ConnLimit: 5, ValidUntil: until})
		if err != nil {
			t.Fatalf("%s: %v", c.preset, err)
		}
		for _, w := range c.want {
			if !strings.Contains(sql, w) {
				t.Errorf("%s: missing %q in\n%s", c.preset, w, sql)
			}
		}
		for _, w := range c.wantNot {
			if strings.Contains(sql, w) {
				t.Errorf("%s: unexpected %q in\n%s", c.preset, w, sql)
			}
		}
		if !strings.Contains(sql, `GRANT CONNECT ON DATABASE "my-db" TO "r1"`) || !strings.HasPrefix(sql, "BEGIN;") || !strings.HasSuffix(sql, "COMMIT;\n") {
			t.Errorf("%s: not a quoted transaction:\n%s", c.preset, sql)
		}
	}
}

func TestCreateRoleSQLRejectsPlaintextAndUnknownPreset(t *testing.T) {
	if _, err := CreateRoleSQL(CreateParams{Role: "r1", Database: "d", AdminRole: "d", Preset: PresetReadOnly, Verifier: "plaintext-secret"}); err == nil {
		t.Fatal("a plaintext password must be refused")
	}
	if _, err := CreateRoleSQL(CreateParams{Role: "r1", Database: "d", AdminRole: "d", Preset: "dba", Verifier: "SCRAM-SHA-256$x"}); !errors.Is(err, ErrPreset) {
		t.Fatalf("unknown preset err = %v", err)
	}
}

func TestIdentifiersAreQuoted(t *testing.T) {
	sql, err := DropRoleSQL(`x"; DROP ROLE admin; --`, "d")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sql, "ALTER ROLE \"x\"; DROP") {
		t.Fatalf("identifier not escaped:\n%s", sql)
	}
	if !strings.Contains(sql, `"x""; DROP ROLE admin; --"`) {
		t.Fatalf("expected doubled quote in\n%s", sql)
	}
}

func TestScramVerifierShape(t *testing.T) {
	v, err := scramVerifierWithSalt("pw", []byte("0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(v, "SCRAM-SHA-256$4096:MDEyMzQ1Njc4OWFiY2RlZg==$") {
		t.Fatalf("verifier = %q", v)
	}
	if strings.Contains(v, "pw") && strings.Contains(v, "pw=") {
		t.Fatal("verifier must not carry the password")
	}
	again, _ := scramVerifierWithSalt("pw", []byte("0123456789abcdef"))
	if again != v {
		t.Fatal("verifier must be deterministic for a fixed salt")
	}
}

func TestTTLClamp(t *testing.T) {
	l := DefaultTTLLimits()
	cases := []struct {
		in      time.Duration
		want    time.Duration
		clamped bool
	}{
		{0, time.Hour, false},
		{-time.Minute, time.Hour, false},
		{time.Minute, 15 * time.Minute, true},
		{15 * time.Minute, 15 * time.Minute, false},
		{2 * time.Hour, 2 * time.Hour, false},
		{24 * time.Hour, 24 * time.Hour, false},
		{48 * time.Hour, 24 * time.Hour, true},
	}
	for _, c := range cases {
		got, clamped := l.Clamp(c.in)
		if got != c.want || clamped != c.clamped {
			t.Errorf("Clamp(%v) = %v,%v want %v,%v", c.in, got, clamped, c.want, c.clamped)
		}
	}
}

func TestTTLLimitsFromEnv(t *testing.T) {
	env := map[string]string{EnvTTLMinMinutes: "5", EnvTTLMaxMinutes: "120", EnvTTLDefaultMinutes: "500"}
	l := TTLLimitsFromEnv(func(k string) string { return env[k] })
	if l.Min != 5*time.Minute || l.Max != 2*time.Hour || l.Default != 2*time.Hour {
		t.Fatalf("limits = %+v", l)
	}
	bad := map[string]string{EnvTTLMinMinutes: "600", EnvTTLMaxMinutes: "10"}
	if got := TTLLimitsFromEnv(func(k string) string { return bad[k] }); got != DefaultTTLLimits() {
		t.Fatalf("inconsistent env must fall back, got %+v", got)
	}
}

func TestParseRoles(t *testing.T) {
	roles, err := ParseRoles([][]string{
		{"mydb", "t", "t", "t", "t", "f", "-1", "", "3"},
		{"reader", "t", "f", "f", "f", "f", "5", "2026-10-09T12:00:00Z", "0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !roles[0].Superuser || roles[0].Connections != 3 || roles[1].ConnLimit != 5 || roles[1].ValidUntil == "" {
		t.Fatalf("roles = %+v", roles)
	}
	if _, err := ParseRoles([][]string{{"short"}}); err == nil {
		t.Fatal("short row must fail")
	}
}

func TestScopeDryRun(t *testing.T) {
	db := Placement{Name: "main", ProjectID: "p1", EnvironmentID: "e-prod"}
	apps := []Placement{
		{Name: "web", ProjectID: "p1", EnvironmentID: "e-prod"},
		{Name: "worker", ProjectID: "p1", EnvironmentID: "e-staging"},
		{Name: "other", ProjectID: "p2"},
	}
	cases := []struct {
		scope Scope
		lost  []string
	}{
		{ScopePlatform, nil},
		{ScopeProject, []string{"other"}},
		{ScopeEnvironment, []string{"worker", "other"}},
	}
	for _, c := range cases {
		vs, err := DryRun(c.scope, db, apps)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, v := range Lost(vs) {
			got = append(got, v.App.Name)
			if v.Reason == "" {
				t.Errorf("%s: lost app %s has no reason", c.scope, v.App.Name)
			}
		}
		if strings.Join(got, ",") != strings.Join(c.lost, ",") {
			t.Errorf("%s: lost = %v, want %v", c.scope, got, c.lost)
		}
	}
	if _, err := DryRun(ScopeProject, Placement{Name: "x"}, nil); !errors.Is(err, ErrScopeUnplaced) {
		t.Errorf("unplaced project err = %v", err)
	}
	if _, err := DryRun(ScopeEnvironment, Placement{Name: "x", ProjectID: "p"}, nil); !errors.Is(err, ErrScopeUnplaced) {
		t.Errorf("unplaced env err = %v", err)
	}
	if _, err := DryRun("galaxy", db, nil); err == nil {
		t.Error("unknown scope must fail")
	}
}

func TestTLSScripts(t *testing.T) {
	on := tlsSetScript(true)
	off := tlsSetScript(false)
	if !strings.Contains(on, `s/^host(`) || !strings.Contains(on, `)/hostssl\1/`) {
		t.Errorf("require script wrong: %s", on)
	}
	if !strings.Contains(off, `s/^hostssl(`) || !strings.Contains(off, `)/host\1/`) {
		t.Errorf("relax script wrong: %s", off)
	}
}
