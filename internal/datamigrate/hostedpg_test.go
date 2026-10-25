package datamigrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

const testSecret = "s3cr3t/p@ss"

func TestParseHostedURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		cfg     HostedConfig
		wantErr string
		mode    string
		tls     bool
	}{
		{"heroku style defaults to require", "postgres://u:pw@ec2-1.compute-1.amazonaws.com:5432/d1", HostedConfig{}, "", "require", true},
		{"verify-full kept", "postgresql://u:pw@db.example.com/d1?sslmode=verify-full", HostedConfig{}, "", "verify-full", true},
		{"prefer refused on public host", "postgres://u:pw@db.example.com/d1?sslmode=prefer", HostedConfig{}, "unencrypted", "", false},
		{"disable refused on public host", "postgres://u:pw@db.example.com/d1?sslmode=disable", HostedConfig{}, "unencrypted", "", false},
		{"disable allowed by policy", "postgres://u:pw@db.example.com/d1?sslmode=disable", HostedConfig{AllowInsecure: true}, "", "", false},
		{"private host may skip tls", "postgres://u:pw@10.0.0.5/d1?sslmode=disable", HostedConfig{}, "", "", false},
		{"bare name is private", "postgres://u:pw@pg/d1", HostedConfig{}, "", "", false},
		{"unknown sslmode", "postgres://u:pw@db.example.com/d1?sslmode=bogus", HostedConfig{}, "unknown sslmode", "", false},
		{"wrong scheme", "mysql://u:pw@db.example.com/d1", HostedConfig{}, "must start with postgres", "", false},
		{"no user", "postgres://db.example.com/d1", HostedConfig{}, "include a user", "", false},
		{"no database", "postgres://u:pw@db.example.com", HostedConfig{}, "database is required", "", false},
		{"link local blocked", "postgres://u:pw@169.254.169.254/d1", HostedConfig{}, "link-local", "", false},
		{"unsupported param", "postgres://u:pw@db.example.com/d1?options=-c%20x", HostedConfig{}, "not supported", "", false},
		{"not a url", "postgres://u:" + testSecret + "@db.example.com/d1%zz", HostedConfig{}, "not valid", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hs, err := ParseHostedURL(tc.raw, tc.cfg)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				if strings.Contains(err.Error(), "pw") || strings.Contains(err.Error(), testSecret) {
					t.Fatalf("error leaks credentials: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if hs.Source.SSLMode != tc.mode || hs.Source.TLS != tc.tls {
				t.Errorf("mode=%q tls=%v, want %q %v", hs.Source.SSLMode, hs.Source.TLS, tc.mode, tc.tls)
			}
			if strings.Contains(hs.Redacted, "pw") || strings.Contains(hs.String(), "pw") {
				t.Errorf("redacted form leaks: %s", hs.Redacted)
			}
		})
	}
}

func TestRedactionEverywhere(t *testing.T) {
	raw := hostedRaw
	hs, err := ParseHostedURL(raw, HostedConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if hs.Source.Password != testSecret {
		t.Fatalf("password = %q", hs.Source.Password)
	}
	text := hs.Scrub("could not connect to " + raw + " using password " + testSecret + " escaped s3cr3t%2Fp%40ss")
	if strings.Contains(text, "s3cr3t") {
		t.Errorf("Scrub leaked: %s", text)
	}
	plan := HostedPlan{URL: SecretURL(raw), TargetName: "app"}
	for _, out := range []string{plan.URL.String(), strings.TrimSpace(strings.Join([]string{plan.URL.String()}, "")), hs.String()} {
		if strings.Contains(out, "s3cr3t") {
			t.Errorf("leak: %s", out)
		}
	}
	b, _ := plan.URL.MarshalText()
	if strings.Contains(string(b), "s3cr3t") || !strings.Contains(RedactURL(raw), "u:***@db.example.com:5432/d1") {
		t.Errorf("marshal/redact = %s / %s", b, RedactURL(raw))
	}
}

func TestHostedConfigFromEnv(t *testing.T) {
	get := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cfg, err := HostedConfigFromEnv(get(map[string]string{EnvHostedMaxBytes: "1000", EnvHostedAllowInsecure: "true"}))
	if err != nil || cfg.MaxBytes != 1000 || !cfg.AllowInsecure {
		t.Fatalf("cfg = %+v err = %v", cfg, err)
	}
	if cfg, _ := HostedConfigFromEnv(get(nil)); cfg.MaxBytes != DefaultHostedMaxBytes {
		t.Errorf("default max = %d", cfg.MaxBytes)
	}
	for _, bad := range []map[string]string{{EnvHostedMaxBytes: "-1"}, {EnvHostedMaxBytes: "x"}, {EnvHostedAllowInsecure: "maybe"}} {
		if _, err := HostedConfigFromEnv(get(bad)); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestParseHostedProbe(t *testing.T) {
	p := ParseHostedProbe("NOTICE: hi\nversion_num|160004\nversion|16.4\nsize|2048\ntables|7\next|uuid-ossp\next|pg_trgm\npolicyrole|app_user\n")
	if p.Major != 16 || p.VersionNum != 160004 || p.SizeBytes != 2048 || p.Tables != 7 || len(p.Extensions) != 2 || p.Extensions[0] != "pg_trgm" || len(p.PolicyRoles) != 1 {
		t.Errorf("probe = %+v", p)
	}
	if q := ParseHostedProbe("version_num|170001\n"); q.Major != 17 {
		t.Errorf("major from version_num = %d", q.Major)
	}
}

func TestHostedPreflight(t *testing.T) {
	hs, _ := ParseHostedURL("postgres://u:pw@db.example.com/d1", HostedConfig{})
	base := HostedPreflightInput{
		Plan:   HostedPlan{TargetName: "app", WritesFrozen: true},
		Source: hs, Config: HostedConfig{MaxBytes: 1 << 30}, DiskMargin: 1.5,
		Probe: HostedProbe{Major: 16, SizeBytes: 100 << 20, Extensions: []string{"uuid-ossp"}, FreeBytes: 10 << 30},
	}
	tests := []struct {
		name  string
		mut   func(*HostedPreflightInput)
		check string
		want  string
	}{
		{"ready", func(*HostedPreflightInput) {}, "writes_frozen", SeverityOK},
		{"not frozen blocks", func(i *HostedPreflightInput) { i.Plan.WritesFrozen = false }, "writes_frozen", SeverityBlock},
		{"large blocks", func(i *HostedPreflightInput) { i.Probe.SizeBytes = 2 << 30 }, "size_limit", SeverityBlock},
		{"large with override warns", func(i *HostedPreflightInput) { i.Probe.SizeBytes, i.Plan.AllowLarge = 2<<30, true }, "size_limit", SeverityWarn},
		{"missing extension blocks", func(i *HostedPreflightInput) { i.Probe.Extensions = []string{"postgis"} }, "extensions", SeverityBlock},
		{"provider extension warns", func(i *HostedPreflightInput) { i.Probe.Extensions = []string{"neon"} }, "provider_extensions", SeverityWarn},
		{"policy roles block", func(i *HostedPreflightInput) { i.Probe.PolicyRoles = []string{"authenticated"} }, "policy_roles", SeverityBlock},
		{"older target blocks", func(i *HostedPreflightInput) { i.Plan.TargetVersion = "15" }, "version", SeverityBlock},
		{"tls ok", func(*HostedPreflightInput) {}, "tls", SeverityOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			tc.mut(&in)
			if got := severityOf(HostedPreflight(in), tc.check); got != tc.want {
				t.Fatalf("%s = %s, want %s", tc.check, got, tc.want)
			}
		})
	}
	res := HostedPreflight(base)
	if res.Blocked || res.TargetVersion != "16" {
		t.Errorf("ready plan = blocked %v version %q", res.Blocked, res.TargetVersion)
	}
}

func TestDumpFilter(t *testing.T) {
	in := strings.Join([]string{
		"CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA heroku_ext;",
		"COMMENT ON EXTENSION pgcrypto IS 'crypto';",
		"CREATE EXTENSION IF NOT EXISTS neon WITH SCHEMA public;",
		"CREATE TABLE public.t (id uuid DEFAULT heroku_ext.gen_random_uuid());",
		"COPY public.t (id) FROM stdin;",
		"COMMENT ON EXTENSION x IS y",
		"heroku_ext.keep",
		"\\.",
		"COMMENT ON EXTENSION after IS 'x';",
		"",
	}, "\n")
	f := &dumpFilter{}
	got, err := io.ReadAll(f.wrap(strings.NewReader(in)))
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	for _, want := range []string{"WITH SCHEMA public;", "public.gen_random_uuid()", "COMMENT ON EXTENSION x IS y\nheroku_ext.keep\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	for _, bad := range []string{"CREATE EXTENSION IF NOT EXISTS neon", "COMMENT ON EXTENSION pgcrypto", "COMMENT ON EXTENSION after"} {
		if strings.Contains(out, bad) {
			t.Errorf("output kept %q", bad)
		}
	}
	ids := map[string]bool{}
	for _, c := range f.Findings() {
		ids[c.ID] = true
	}
	for _, id := range []string{"extension_schema", "provider_extensions"} {
		if !ids[id] {
			t.Errorf("missing finding %s", id)
		}
	}
}

func TestDumpFilterQuietCommentsAndCleansRewrittenSchema(t *testing.T) {
	f := &dumpFilter{}
	b, _ := io.ReadAll(f.wrap(strings.NewReader("CREATE EXTENSION pg_trgm WITH SCHEMA heroku_ext;\nCOMMENT ON EXTENSION pg_trgm IS 'x';\n")))
	if !strings.Contains(string(b), `DROP SCHEMA IF EXISTS "heroku_ext" RESTRICT`) {
		t.Errorf("no cleanup of the emptied schema:\n%s", b)
	}
	for _, c := range f.Findings() {
		if c.ID == "extension_comment" {
			t.Error("extension_comment must not be reported")
		}
	}
}

func TestDumpFilterLongLineAndNoTrailingNewline(t *testing.T) {
	long := "INSERT INTO t VALUES ('" + strings.Repeat("x", 200<<10) + "');\nCOMMENT ON EXTENSION a IS 'b'"
	got, err := io.ReadAll((&dumpFilter{}).wrap(strings.NewReader(long)))
	if err != nil || len(got) != len(long)-len("\nCOMMENT ON EXTENSION a IS 'b'")+1 {
		t.Fatalf("len=%d err=%v", len(got), err)
	}
}

type hostedRuntime struct {
	fakeRuntime
	probe string
	// target is the output of TargetStateCommand; execs records role creation.
	target string
	objs   string
	execs  []string
}

func (h *hostedRuntime) Exec(ctx context.Context, c string, cmd []string) (io.ReadCloser, error) {
	switch cmd[len(cmd)-1] {
	case HostedProbeCommand()[2]:
		return io.NopCloser(strings.NewReader(h.probe)), nil
	case DiskFreeCommand()[2]:
		return io.NopCloser(strings.NewReader("free|107374182400\n")), nil
	case TargetStateCommand()[2]:
		return io.NopCloser(strings.NewReader(h.target)), nil
	case ObjectCountsCommand()[2]:
		return io.NopCloser(strings.NewReader(h.objs)), nil
	}
	if strings.Contains(cmd[len(cmd)-1], "CREATE ROLE") {
		h.execs = append(h.execs, cmd[len(cmd)-1])
		return io.NopCloser(strings.NewReader("")), nil
	}
	return h.fakeRuntime.Exec(ctx, c, cmd)
}

//nolint:gosec // fake credential for redaction tests
const hostedRaw = "postgres://u:s3cr3t%2Fp%40ss@db.example.com:5432/d1"

func newHosted(probe string, dumpErr error) (*Copier, *hostedRuntime, *fakeRestorer) {
	rt := &hostedRuntime{probe: probe, fakeRuntime: fakeRuntime{running: true, dumpErr: dumpErr,
		dumpOutput: "CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA heroku_ext;\nCREATE TABLE t();\n",
		srcCounts:  "public.users|5\n", tgtCounts: "public.users|5\n"}}
	rs := &fakeRestorer{}
	return &Copier{Runtime: rt, Restorer: rs, VerifyAttempts: 1, VerifyInterval: time.Millisecond}, rt, rs
}

const rlsProbe = okProbe + "policyrole|readonly_hosted\n"

const okProbe = "version_num|160004\nversion|16.4\nsize|1048576\ntables|1\next|pgcrypto\n"

func TestMoveHosted(t *testing.T) {
	cfg := HostedConfig{MaxBytes: 1 << 30}
	plan := HostedPlan{URL: SecretURL(hostedRaw), TargetName: "app", WritesFrozen: true}

	t.Run("moves and verifies", func(t *testing.T) {
		c, rt, rs := newHosted(okProbe, nil)
		res, err := c.MoveHosted(t.Context(), plan, cfg, 1.5, 20)
		if err != nil {
			t.Fatal(err)
		}
		if res.Verification.Checked != 1 || res.Verification.Mismatched != 0 || !strings.Contains(rs.got, "WITH SCHEMA public;") {
			t.Errorf("res = %+v restored %q", res.Verification, rs.got)
		}
		if len(res.Restore) == 0 || strings.Contains(res.Redacted, "s3cr3t") {
			t.Errorf("findings %v redacted %q", res.Restore, res.Redacted)
		}
		last := rt.created[len(rt.created)-1]
		if last.Image != "postgres:16" || last.Env["SRC_SSLMODE"] != "require" || last.Env["SRC_TLS"] != "1" {
			t.Errorf("copy helper = %+v", last)
		}
	})
	t.Run("refuses without writes_frozen", func(t *testing.T) {
		c, rt, _ := newHosted(okProbe, nil)
		p := plan
		p.WritesFrozen = false
		if _, err := c.MoveHosted(t.Context(), p, cfg, 1.5, 20); !errors.Is(err, ErrWritesNotFrozen) {
			t.Fatalf("err = %v", err)
		}
		if len(rt.created) != 0 {
			t.Error("a helper started before the acknowledgement")
		}
	})
	t.Run("blocked preflight stops before dump", func(t *testing.T) {
		c, rt, rs := newHosted("version|16.4\nsize|1\next|postgis\n", nil)
		_, err := c.MoveHosted(t.Context(), plan, cfg, 1.5, 20)
		if !errors.Is(err, ErrPreflightBlocked) || rs.got != "" || len(rt.created) != 1 {
			t.Fatalf("err = %v restored %q helpers %d", err, rs.got, len(rt.created))
		}
	})
	t.Run("large needs the override", func(t *testing.T) {
		big := "version|16.4\nsize|2147483648\n"
		c, _, _ := newHosted(big, nil)
		if _, err := c.MoveHosted(t.Context(), plan, cfg, 1.5, 20); !errors.Is(err, ErrPreflightBlocked) {
			t.Fatalf("err = %v", err)
		}
		p := plan
		p.AllowLarge = true
		c, _, _ = newHosted(big, nil)
		if _, err := c.MoveHosted(t.Context(), p, cfg, 1.5, 20); err != nil {
			t.Fatalf("override failed: %v", err)
		}
	})
	t.Run("errors are scrubbed", func(t *testing.T) {
		c, _, _ := newHosted(okProbe, errors.New("exec exited 1: could not connect to "+hostedRaw+" password "+testSecret))
		_, err := c.MoveHosted(t.Context(), plan, cfg, 1.5, 20)
		if err == nil || strings.Contains(err.Error(), "s3cr3t") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("bad url", func(t *testing.T) {
		c, _, _ := newHosted(okProbe, nil)
		p := plan
		p.URL = "mysql://u:" + testSecret + "@h/d"
		if _, err := c.MoveHosted(t.Context(), p, cfg, 1.5, 20); err == nil || strings.Contains(err.Error(), "s3cr3t") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestHostedHelperEnvCarriesSSLMode(t *testing.T) {
	env := helperEnv(Source{Host: "h", Port: 5432, TLS: true, SSLMode: "verify-full"})
	if env["SRC_SSLMODE"] != "verify-full" || env["SRC_TLS"] != "1" {
		t.Errorf("env = %v", env)
	}
	var _ docker.Runtime = (*hostedRuntime)(nil)
}

func TestHostedRolesAgainstTarget(t *testing.T) {
	cfg := HostedConfig{MaxBytes: 1 << 30}
	plan := HostedPlan{URL: SecretURL(hostedRaw), TargetName: "app", WritesFrozen: true, TargetExists: true, OwnTarget: true}

	t.Run("blocks while the target lacks the role", func(t *testing.T) {
		c, rt, _ := newHosted(rlsProbe, nil)
		rt.target = "role|postgres\nusertables|0\n"
		_, res, err := c.PreflightHosted(t.Context(), plan, cfg, 1.5, 20)
		if err != nil || severityOf(res.Preflight, "policy_roles") != SeverityBlock || len(res.MissingRoles) != 1 {
			t.Fatalf("err=%v res=%+v", err, res)
		}
	})
	t.Run("clears once the role exists on the target", func(t *testing.T) {
		c, rt, _ := newHosted(rlsProbe, nil)
		rt.target = "role|postgres\nrole|readonly_hosted\nusertables|0\n"
		_, res, err := c.PreflightHosted(t.Context(), plan, cfg, 1.5, 20)
		if err != nil || res.Preflight.Blocked || severityOf(res.Preflight, "policy_roles") != SeverityOK {
			t.Fatalf("err=%v blocked=%v checks=%+v", err, res.Preflight.Blocked, res.Preflight.Checks)
		}
	})
	t.Run("role names are compared exactly", func(t *testing.T) {
		got := MissingRoles([]string{"Reader", "writer"}, []string{"reader", "writer"})
		if len(got) != 1 || got[0] != "Reader" {
			t.Errorf("missing = %v", got)
		}
	})
	t.Run("unreadable target blocks instead of guessing", func(t *testing.T) {
		in := HostedPreflightInput{Plan: HostedPlan{TargetName: "app", WritesFrozen: true}, Config: cfg, DiskMargin: 1.5,
			Probe: HostedProbe{Major: 16, SizeBytes: 1 << 20, FreeBytes: 10 << 30, PolicyRoles: []string{"r"}}}
		if got := severityOf(HostedPreflight(in), "policy_roles"); got != SeverityBlock {
			t.Errorf("severity = %s", got)
		}
	})
	t.Run("dry run says which roles would be created", func(t *testing.T) {
		c, rt, _ := newHosted(rlsProbe, nil)
		rt.target = "role|postgres\nusertables|0\n"
		p := plan
		p.CreateMissingRoles = true
		_, res, err := c.PreflightHosted(t.Context(), p, cfg, 1.5, 20)
		if err != nil || res.Preflight.Blocked || severityOf(res.Preflight, "policy_roles") != SeverityWarn {
			t.Fatalf("err=%v res=%+v", err, res.Preflight.Checks)
		}
		for _, ch := range res.Preflight.Checks {
			if ch.ID == "policy_roles" && !strings.Contains(ch.Message, "readonly_hosted") {
				t.Errorf("message = %q", ch.Message)
			}
		}
		if len(rt.execs) != 0 {
			t.Error("dry run created a role")
		}
	})
	t.Run("move creates the roles NOLOGIN NOINHERIT and reports it", func(t *testing.T) {
		c, rt, _ := newHosted(rlsProbe, nil)
		rt.target = "role|postgres\nusertables|0\n"
		rt.objs = "sequences|2\nviews|1\nfunctions|3\nextensions|1\n"
		p := plan
		p.CreateMissingRoles = true
		res, err := c.MoveHosted(t.Context(), p, cfg, 1.5, 20)
		if err != nil {
			t.Fatal(err)
		}
		if len(rt.execs) != 1 || !strings.Contains(rt.execs[0], `CREATE ROLE "readonly_hosted" NOLOGIN NOINHERIT`) {
			t.Fatalf("execs = %v", rt.execs)
		}
		found := false
		for _, f := range res.Verification.Findings {
			found = found || f.ID == "roles_created"
		}
		if !found || res.Verification.Objects == nil || res.Verification.Objects.Functions != 3 || res.Verification.Objects.Sequences != 2 {
			t.Errorf("verification = %+v", res.Verification)
		}
	})
	t.Run("unsafe role names are refused", func(t *testing.T) {
		if _, err := CreateRolesCommand([]string{`x"; DROP DATABASE y; --`}); err == nil {
			t.Error("unsafe name accepted")
		}
	})
}

func TestHostedCollisionOnlyWhenTargetHasData(t *testing.T) {
	base := HostedPreflightInput{
		Plan:   HostedPlan{TargetName: "app", WritesFrozen: true, TargetExists: true, OwnTarget: true},
		Config: HostedConfig{MaxBytes: 1 << 30}, DiskMargin: 1.5,
		Probe: HostedProbe{Major: 16, SizeBytes: 1 << 20, FreeBytes: 10 << 30},
	}
	empty := base
	empty.Target = TargetState{Known: true}
	if got := severityOf(HostedPreflight(empty), "collision"); got != SeverityOK {
		t.Errorf("empty target collision = %s", got)
	}
	used := base
	used.Target = TargetState{Known: true, UserTables: 4}
	if got := severityOf(HostedPreflight(used), "collision"); got != SeverityWarn {
		t.Errorf("imported target collision = %s", got)
	}
}

func TestDumpFilterDropsDeclaredSchemas(t *testing.T) {
	in := "CREATE SCHEMA heroku_ext;\nCREATE SCHEMA IF NOT EXISTS \"Other\";\nCREATE SCHEMA public;\nCREATE SCHEMA pg_x;\n"
	var runs [2]string
	for i := range runs {
		b, err := io.ReadAll((&dumpFilter{}).wrap(strings.NewReader(in)))
		if err != nil {
			t.Fatal(err)
		}
		runs[i] = string(b)
	}
	out := runs[0]
	for _, want := range []string{"DROP SCHEMA IF EXISTS heroku_ext CASCADE;\nCREATE SCHEMA heroku_ext;", `DROP SCHEMA IF EXISTS "Other" CASCADE;`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "DROP SCHEMA IF EXISTS public") || strings.Contains(out, "DROP SCHEMA IF EXISTS pg_") || runs[0] != runs[1] {
		t.Errorf("system schema dropped or output unstable:\n%s", out)
	}
}

func TestTidyExecErrorKeepsOnlyTheCause(t *testing.T) {
	script := strings.Repeat("export PGHOST=x\n", 30)
	err := fmt.Errorf("probe the hosted database: %w", &docker.ExecExitError{
		Cmd: []string{"sh", "-c", script}, Container: "c", ExitCode: 2,
		Stderr: "noise\npsql: error: connection to server failed\n\tpassword authentication failed\n",
	})
	got := tidyExecError(err).Error()
	if strings.Contains(got, "PGHOST") || !strings.Contains(got, "password authentication failed") || !strings.Contains(got, "exited 2") {
		t.Errorf("got %q", got)
	}
}
