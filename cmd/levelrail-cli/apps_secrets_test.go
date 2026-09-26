package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRun_AppsSecretsList(t *testing.T) {
	var gotPath, gotMethod string
	staleUpdatedAt := time.Now().Add(-120 * 24 * time.Hour).UTC().Format(time.RFC3339)
	freshUpdatedAt := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]secretKeyResource{
			{Key: "API_KEY", Locked: true, UpdatedAt: staleUpdatedAt, Stale: true},
			{Key: "DB_PASSWORD", Locked: false, UpdatedAt: freshUpdatedAt, Stale: false},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "list", "web", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/apps/web/secrets" {
		t.Errorf("request = %s %s, want GET /api/v1/apps/web/secrets", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "API_KEY") || !strings.Contains(stdout, "DB_PASSWORD") {
		t.Errorf("stdout = %q, want both secret keys listed", stdout)
	}
	if strings.Contains(stdout, "secret-value") {
		t.Errorf("stdout = %q, must never contain a secret value", stdout)
	}
	if !strings.Contains(stdout, "AGE") || !strings.Contains(stdout, "STALE") {
		t.Errorf("stdout = %q, want AGE and STALE columns in the header", stdout)
	}
	if !strings.Contains(stdout, "months ago") {
		t.Errorf("stdout = %q, want the stale key's age rendered", stdout)
	}
}

func TestRun_AppsSecretsList_JSON(t *testing.T) {
	staleUpdatedAt := time.Now().Add(-120 * 24 * time.Hour).UTC().Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]secretKeyResource{
			{Key: "API_KEY", Locked: true, UpdatedAt: staleUpdatedAt, Stale: true},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "list", "web", "--json", "--api-url", srv.URL})

	var got []secretKeyResource
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("json.Unmarshal(%q): %v", stdout, err)
	}
	if len(got) != 1 || got[0].UpdatedAt != staleUpdatedAt || !got[0].Stale {
		t.Errorf("got %+v, want a single stale API_KEY with UpdatedAt %q", got, staleUpdatedAt)
	}
}

func TestRun_AppsSecretsList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]secretKeyResource{})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "list", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no secrets set") {
		t.Errorf("stdout = %q, want the empty-state message", stdout)
	}
}

// servePendingChanges answers the follow-up pending-changes lookup a config
// write makes, reporting whether it handled the request.
func servePendingChanges(w http.ResponseWriter, r *http.Request, pending bool) bool {
	if !strings.HasSuffix(r.URL.Path, "/pending-changes") {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	if pending {
		_, _ = w.Write([]byte(`{"pending":true,"changes":[{"kind":"secret","keys":["API_KEY"],"since":"2026-09-25T00:00:00Z"}],"apply_action":"restart"}`))
		return true
	}
	_, _ = w.Write([]byte(`{"pending":false,"changes":[],"apply_action":"restart"}`))
	return true
}

func TestRun_AppsSecretsSet_PendingHintAndApply(t *testing.T) {
	for _, apply := range []bool{false, true} {
		var applied bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case servePendingChanges(w, r, true):
			case strings.HasSuffix(r.URL.Path, "/apply-pending"):
				applied = true
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{}`))
			default:
				w.WriteHeader(http.StatusNoContent)
			}
		}))
		args := []string{"apps", "secrets", "set", "web", "API_KEY", "v", "--api-url", srv.URL}
		if apply {
			args = append(args, "--apply")
		}
		stdout, _ := runCLIExpectOK(t, args)
		srv.Close()
		if apply {
			if !applied || !strings.Contains(stdout, "restarting it") {
				t.Errorf("--apply: applied=%v stdout=%q", applied, stdout)
			}
			continue
		}
		if applied || !strings.Contains(stdout, "1 changes pending") || !strings.Contains(stdout, "apps apply web (or pass --apply)") {
			t.Errorf("hint: applied=%v stdout=%q", applied, stdout)
		}
	}
}

func TestRun_AppsSecretsDelete_ApplyDeniedFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case servePendingChanges(w, r, true):
		case strings.HasSuffix(r.URL.Path, "/apply-pending"):
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden"}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	stderr := runCLIExpectAPIError(t, []string{"apps", "secrets", "delete", "web", "API_KEY", "--apply", "--api-url", srv.URL})
	if !strings.Contains(stderr, "could not apply pending changes") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRun_AppsSecretsDelete(t *testing.T) {
	var gotMethod, gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if servePendingChanges(w, r, false) {
			return
		}
		gotMethod, gotURI = r.Method, r.URL.RequestURI()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "delete", "web", "API_KEY", "--force", "--api-url", srv.URL})
	if gotMethod != http.MethodDelete || gotURI != "/api/v1/apps/web/secrets/API_KEY?force=true" {
		t.Errorf("request = %s %s", gotMethod, gotURI)
	}
	if !strings.Contains(stdout, `secret "API_KEY" deleted for app "web"`) {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_AppsSecretsSet(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if servePendingChanges(w, r, false) {
			return
		}
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "set", "web", "API_KEY", "s3cr3t", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/secrets/API_KEY" {
		t.Errorf("request = %s %s, want PUT /api/v1/apps/web/secrets/API_KEY", gotMethod, gotPath)
	}
	if gotBody["value"] != "s3cr3t" {
		t.Errorf("body value = %v, want s3cr3t", gotBody["value"])
	}
	if gotBody["overwrite_locked"] != false {
		t.Errorf("body overwrite_locked = %v, want false by default", gotBody["overwrite_locked"])
	}
	if !strings.Contains(stdout, `secret "API_KEY" set for app "web"`) {
		t.Errorf("stdout = %q, want a set confirmation", stdout)
	}
}

func TestRun_AppsSecretsSet_Force(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"apps", "secrets", "set", "web", "API_KEY", "s3cr3t", "--force", "--api-url", srv.URL})
	if gotBody["overwrite_locked"] != true {
		t.Errorf("body overwrite_locked = %v, want true with --force", gotBody["overwrite_locked"])
	}
}

func TestRun_AppsSecretsSet_Locked(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusConflict, `{"error":"secret is locked, set overwrite_locked to overwrite"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "secrets", "set", "web", "API_KEY", "s3cr3t", "--api-url", srv.URL})
	if !strings.Contains(stderr, "locked") {
		t.Errorf("stderr = %q, want the server's locked-conflict message", stderr)
	}
}

func TestRun_AppsSecretsSet_MissingArgs(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "secrets", "set", "web", "API_KEY"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_AppsSecretsLock(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "lock", "web", "API_KEY", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/secrets/API_KEY/lock" {
		t.Errorf("request = %s %s, want POST /api/v1/apps/web/secrets/API_KEY/lock", gotMethod, gotPath)
	}
	if gotBody["locked"] != true {
		t.Errorf("body locked = %v, want true by default", gotBody["locked"])
	}
	if !strings.Contains(stdout, `secret "API_KEY" on app "web" locked`) {
		t.Errorf("stdout = %q, want a locked confirmation", stdout)
	}
}

func TestRun_AppsSecretsLock_Unlock(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "lock", "web", "API_KEY", "--locked=false", "--api-url", srv.URL})
	if gotBody["locked"] != false {
		t.Errorf("body locked = %v, want false with --locked=false", gotBody["locked"])
	}
	if !strings.Contains(stdout, `unlocked`) {
		t.Errorf("stdout = %q, want an unlocked confirmation", stdout)
	}
}

func TestRun_AppsSecretsSet_EnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "app.env")
	content := "# a comment\n\nAPI_KEY=s3cr3t\nexport DB_PASSWORD='p@ss'\nQUOTED=\"has spaces\"\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	type setCall struct {
		path  string
		value string
	}
	var calls []setCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if servePendingChanges(w, r, false) {
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		calls = append(calls, setCall{path: r.URL.Path, value: fmt.Sprint(body["value"])})
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "set", "web", "--env-file", envPath, "--api-url", srv.URL})

	want := map[string]string{
		"/api/v1/apps/web/secrets/API_KEY":     "s3cr3t",
		"/api/v1/apps/web/secrets/DB_PASSWORD": "p@ss",
		"/api/v1/apps/web/secrets/QUOTED":      "has spaces",
	}
	if len(calls) != len(want) {
		t.Fatalf("got %d PUT calls, want %d: %+v", len(calls), len(want), calls)
	}
	for _, c := range calls {
		if want[c.path] != c.value {
			t.Errorf("call %s value = %q, want %q", c.path, c.value, want[c.path])
		}
	}
	if !strings.Contains(stdout, `secret "API_KEY" set for app "web"`) {
		t.Errorf("stdout missing API_KEY confirmation: %q", stdout)
	}
	if strings.Contains(stdout, "s3cr3t") || strings.Contains(stdout, "p@ss") || strings.Contains(stdout, "has spaces") {
		t.Errorf("stdout must never contain a secret value: %q", stdout)
	}
}

func TestRun_AppsSecretsSet_EnvFile_MalformedLines(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "app.env")
	content := "GOOD=value\nno-equals-sign-here\n=no-key-here\nANOTHER=ok\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	var gotKeys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if servePendingChanges(w, r, false) {
			return
		}
		gotKeys = append(gotKeys, strings.TrimPrefix(r.URL.Path, "/api/v1/apps/web/secrets/"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"apps", "secrets", "set", "web", "--env-file", envPath, "--api-url", srv.URL})

	want := []string{"GOOD", "ANOTHER"}
	if len(gotKeys) != len(want) {
		t.Fatalf("got keys %v, want %v", gotKeys, want)
	}
	for i, k := range want {
		if gotKeys[i] != k {
			t.Errorf("key[%d] = %q, want %q", i, gotKeys[i], k)
		}
	}
}

func TestRun_AppsSecretsSet_EnvFile_MissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.env")
	stderr := runCLIExpectValidationError(t, []string{"apps", "secrets", "set", "web", "--env-file", missing})
	if !strings.Contains(stderr, missing) {
		t.Errorf("stderr = %q, want the missing file path", stderr)
	}
}

func TestRun_AppsSecretsSet_EnvFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "empty.env")
	if err := os.WriteFile(envPath, []byte("# only comments\n\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "set", "web", "--env-file", envPath})
	if !strings.Contains(stdout, "nothing set") {
		t.Errorf("stdout = %q, want a nothing-set message", stdout)
	}
}

func TestRun_AppsSecretsSet_EnvFile_MissingAppName(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "secrets", "set", "--env-file", "app.env"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestParseEnvFileBytes(t *testing.T) {
	input := "# comment\n\nexport FOO=bar\nDOUBLE=\"quoted value\"\nSINGLE='quoted value'\nUNQUOTED=plain\nno-equals\n=no-key\nSPACED = trimmed \n"
	got := parseEnvFileBytes([]byte(input))

	want := []envFileEntry{
		{Key: "FOO", Value: "bar"},
		{Key: "DOUBLE", Value: "quoted value"},
		{Key: "SINGLE", Value: "quoted value"},
		{Key: "UNQUOTED", Value: "plain"},
		{Key: "SPACED", Value: "trimmed"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i, e := range want {
		if got[i] != e {
			t.Errorf("entry[%d] = %+v, want %+v", i, got[i], e)
		}
	}
}

func TestRun_AppsSecrets_UnknownSubcommand(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "secrets", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps secrets subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

func TestRun_AppsSecrets_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "-h"})
	if !strings.Contains(stdout, "apps secrets") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}
