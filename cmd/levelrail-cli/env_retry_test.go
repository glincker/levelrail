package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// stateEnvServer mimics the control plane's env rules: a versioned plain and secret store,
// the plan validator that rejects one key in two homes, and the atomic PUT replace.
type stateEnvServer struct {
	mu       sync.Mutex
	env      map[string]string
	secrets  map[string]string
	version  int
	applies  int
	beforeOp func(s *stateEnvServer, attempt int)
}

func newStateEnvServer(t *testing.T) (*stateEnvServer, *httptest.Server) {
	t.Helper()
	s := &stateEnvServer{env: map[string]string{}, secrets: map[string]string{}, version: 1}
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	envRetrySleep = func(time.Duration) {}
	t.Cleanup(func() { envRetrySleep = time.Sleep })
	return s, srv
}

// bump simulates another writer changing a key.
func (s *stateEnvServer) bump(key, value string) {
	s.env[key] = value
	s.version++
}

func (s *stateEnvServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	reply := func(code int, v any) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/apps/web":
		secretNames := []string{}
		for k := range s.secrets {
			secretNames = append(secretNames, k)
		}
		reply(200, map[string]any{"name": "web", "image": "x:1", "port": 80, "env": s.env, "secret_env": secretNames, "env_version": s.version})
	case "GET /api/v1/apps/web/secrets":
		reply(200, []map[string]any{})
	case "GET /api/v1/apps/web/env/bound":
		reply(200, []map[string]any{})
	case "GET /api/v1/apps/web/pending-changes":
		reply(200, map[string]any{})
	case "POST /api/v1/apps/web/env/apply":
		var req struct {
			ExpectedVersion *int              `json:"expected_version"`
			Set             map[string]string `json:"set"`
			Unset           []string          `json:"unset"`
			SecretSet       map[string]string `json:"secret_set"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.applies++
		if s.beforeOp != nil {
			s.beforeOp(s, s.applies)
		}
		seen := map[string]bool{}
		for k := range req.Set {
			seen[k] = true
		}
		for _, k := range req.Unset {
			if seen[k] {
				reply(400, map[string]string{"error": "key " + k + " appears twice in the plan"})
				return
			}
			seen[k] = true
		}
		for k := range req.SecretSet {
			if seen[k] {
				reply(400, map[string]string{"error": "key " + k + " appears twice in the plan"})
				return
			}
			if _, plain := s.env[k]; plain {
				reply(409, map[string]string{"error": "a key in the plan would be both a plain and a secret value; remove one copy first"})
				return
			}
		}
		if req.ExpectedVersion != nil && *req.ExpectedVersion != s.version {
			reply(409, map[string]any{"error": "the env changed since expected_version; reload and retry", "code": "env_version_conflict", "current_version": s.version})
			return
		}
		for k, v := range req.Set {
			s.env[k] = v
		}
		for _, k := range req.Unset {
			delete(s.env, k)
		}
		for k, v := range req.SecretSet {
			s.secrets[k] = v
		}
		s.version++
		reply(200, map[string]any{"app": "web", "changed": true, "env_version": s.version, "plan": []map[string]string{}})
	default:
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/apps/web/secrets/") {
			var req struct {
				Value string `json:"value"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			key := strings.TrimPrefix(r.URL.Path, "/api/v1/apps/web/secrets/")
			s.secrets[key] = req.Value
			if _, plain := s.env[key]; plain {
				delete(s.env, key)
				s.version++
				reply(200, map[string]bool{"replaced_plain_env": true})
				return
			}
			s.version++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		reply(404, map[string]string{"error": "not found"})
	}
}

func runEnvCLI(srv *httptest.Server, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run("levelrail-cli-test", cliArgs(srv, args...), &out, &errb, envMap())
	return code, out.String(), errb.String()
}

func TestSecretsSetReplacesPlainEnvAtomically(t *testing.T) {
	s, srv := newStateEnvServer(t)
	s.env["DB_PASS"] = "old"
	code, out, errs := runEnvCLI(srv, "apps", "secrets", "set", "web", "DB_PASS", "new")
	if code != exitOK {
		t.Fatalf("exit = %d, stderr=%q", code, errs)
	}
	if !strings.Contains(out, "replaced plain env value") {
		t.Errorf("stdout = %q, want the replaced note", out)
	}
	if _, dual := s.env["DB_PASS"]; dual || s.secrets["DB_PASS"] != "new" {
		t.Errorf("env=%v secrets=%v, want the secret only", s.env, s.secrets)
	}
}

func TestSecretsSetPlainKeyFallsBackWhenReadWasStale(t *testing.T) {
	s, srv := newStateEnvServer(t)
	s.beforeOp = func(s *stateEnvServer, _ int) { s.env["TOKEN"] = "added-after-read" }
	code, out, errs := runEnvCLI(srv, "apps", "secrets", "set", "web", "TOKEN", "v")
	if code != exitOK || !strings.Contains(out, "replaced plain env value") {
		t.Fatalf("exit = %d, out=%q, stderr=%q", code, out, errs)
	}
	if _, dual := s.env["TOKEN"]; dual {
		t.Errorf("plain copy left behind: %v", s.env)
	}
}

func TestSecretsSetNewKeyUsesApplyPlan(t *testing.T) {
	s, srv := newStateEnvServer(t)
	code, out, errs := runEnvCLI(srv, "apps", "secrets", "set", "web", "FRESH", "v")
	if code != exitOK || strings.Contains(out, "replaced") {
		t.Fatalf("exit = %d, out=%q, stderr=%q", code, out, errs)
	}
	if s.applies != 1 || s.secrets["FRESH"] != "v" {
		t.Errorf("applies=%d secrets=%v", s.applies, s.secrets)
	}
}

func TestSecretsSetEnvFileReplacesPlainKey(t *testing.T) {
	s, srv := newStateEnvServer(t)
	s.env["A"] = "plain"
	path := t.TempDir() + "/app.env"
	if err := writeTestFile(path, "A=sec\nB=sec2\n"); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runEnvCLI(srv, "apps", "secrets", "set", "web", "--env-file", path)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr=%q", code, errs)
	}
	if !strings.Contains(out, `secret "A" set for app "web" (replaced plain env value)`) || strings.Contains(out, `"B" set for app "web" (replaced`) {
		t.Errorf("stdout = %q", out)
	}
	if len(s.env) != 0 || s.secrets["A"] != "sec" || s.secrets["B"] != "sec2" {
		t.Errorf("env=%v secrets=%v", s.env, s.secrets)
	}
}

func TestEnvSetRetriesWhenOnlyOtherKeysChanged(t *testing.T) {
	s, srv := newStateEnvServer(t)
	s.beforeOp = func(s *stateEnvServer, attempt int) {
		if attempt <= 2 {
			s.bump("OTHER", "x")
		}
	}
	code, _, errs := runEnvCLI(srv, "env", "set", "web", "MINE=1")
	if code != exitOK {
		t.Fatalf("exit = %d, stderr=%q", code, errs)
	}
	if s.env["MINE"] != "1" || s.env["OTHER"] != "x" || s.applies != 3 {
		t.Errorf("env=%v applies=%d, want MINE set after 2 retries", s.env, s.applies)
	}
}

func TestEnvSetSameKeyConflictExits8(t *testing.T) {
	s, srv := newStateEnvServer(t)
	s.env["MINE"] = "before"
	s.beforeOp = func(s *stateEnvServer, _ int) { s.bump("MINE", "theirs") }
	code, _, errs := runEnvCLI(srv, "env", "set", "web", "MINE=1")
	if code != exitEnvConflict {
		t.Fatalf("exit = %d, want %d, stderr=%q", code, exitEnvConflict, errs)
	}
	if !strings.Contains(errs, "MINE") || s.applies != 1 || s.env["MINE"] != "theirs" {
		t.Errorf("stderr=%q applies=%d env=%v", errs, s.applies, s.env)
	}
}

func TestEnvSetExhaustedRetriesExits8(t *testing.T) {
	s, srv := newStateEnvServer(t)
	s.beforeOp = func(s *stateEnvServer, _ int) { s.bump("OTHER", "x") }
	code, _, errs := runEnvCLI(srv, "env", "set", "web", "MINE=1", "--retries", "2")
	if code != exitEnvConflict {
		t.Fatalf("exit = %d, want %d, stderr=%q", code, exitEnvConflict, errs)
	}
	if s.applies != 3 || !strings.Contains(errs, "MINE") || !strings.Contains(errs, "gave up after 2 retries") {
		t.Errorf("applies=%d stderr=%q", s.applies, errs)
	}
}

func TestEnvSetNoRetryExits8Immediately(t *testing.T) {
	s, srv := newStateEnvServer(t)
	s.beforeOp = func(s *stateEnvServer, _ int) { s.bump("OTHER", "x") }
	code, _, _ := runEnvCLI(srv, "env", "set", "web", "MINE=1", "--no-retry")
	if code != exitEnvConflict || s.applies != 1 {
		t.Errorf("exit = %d applies=%d, want 8 after one attempt", code, s.applies)
	}
}

func TestEnvApplyRefusesOversizedValueBeforeSending(t *testing.T) {
	s, srv := newStateEnvServer(t)
	big := strings.Repeat("a", 2<<20)
	code, _, errs := runEnvCLI(srv, "env", "apply", "web", "--set", "BIG="+big)
	if code != exitValidation || s.applies != 0 {
		t.Fatalf("exit = %d applies=%d, stderr=%.200q", code, s.applies, errs)
	}
	if !strings.Contains(errs, "BIG") || !strings.Contains(errs, "limit is 65536 bytes") {
		t.Errorf("stderr = %.200q", errs)
	}
}

func TestReadableNetErrorMentionsClosedConnection(t *testing.T) {
	err := readableNetError(errString("read tcp: connection reset by peer"))
	if !strings.Contains(err.Error(), "closed the connection") {
		t.Errorf("err = %v", err)
	}
}

func TestAppsEnvImportPrintsReport(t *testing.T) {
	s, srv := newStateEnvServer(t)
	path := t.TempDir() + "/app.env"
	if err := writeTestFile(path, "GOOD=1\nthis is junk\nDUP=1\nDUP=2\nLD_PRELOAD=/x.so\n"); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runEnvCLI(srv, "apps", "env", "import", "web", "--file", path, "--all-plain", "--dry-run")
	if code != exitOK {
		t.Fatalf("exit = %d, stderr=%q", code, errs)
	}
	for _, want := range []string{"2 key(s) accepted, 2 rejected, 1 warning(s)", "x line 2: not a KEY=VALUE line", "LD_PRELOAD", "DUP: defined 2 times (lines 3, 4)"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "junk") || strings.Contains(out, "/x.so") {
		t.Errorf("report leaked content: %q", out)
	}
	if s.applies != 0 {
		t.Errorf("dry run applied")
	}
	_, jsonOut, _ := runEnvCLI(srv, "apps", "env", "import", "web", "--file", path, "--all-plain", "--dry-run", "--json")
	if !strings.Contains(jsonOut, `"env_report"`) || !strings.Contains(jsonOut, `"rejected"`) {
		t.Errorf("json lacks env_report: %q", jsonOut)
	}
}

func TestAppsEnvImportStrictAndNothingValid(t *testing.T) {
	s, srv := newStateEnvServer(t)
	path := t.TempDir() + "/app.env"
	if err := writeTestFile(path, "GOOD=1\nbad line\n"); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runEnvCLI(srv, "apps", "env", "import", "web", "--file", path, "--all-plain", "--strict"); code != exitValidation || s.applies != 0 {
		t.Errorf("strict exit = %d applies=%d, want validation and no write", code, s.applies)
	}
	if code, _, errs := runEnvCLI(srv, "apps", "env", "import", "web", "--file", path, "--all-plain"); code != exitOK || s.env["GOOD"] != "1" {
		t.Errorf("lenient exit = %d env=%v stderr=%q, want valid key imported", code, s.env, errs)
	}
	bad := t.TempDir() + "/bad.env"
	if err := writeTestFile(bad, "only junk\n"); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runEnvCLI(srv, "apps", "env", "import", "web", "--file", bad, "--all-plain"); code != exitValidation {
		t.Errorf("nothing-valid exit = %d, want %d", code, exitValidation)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
