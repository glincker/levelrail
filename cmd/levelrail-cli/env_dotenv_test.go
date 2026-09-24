package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseEnvFileBytes_Table(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []envFileEntry
	}{
		{"empty", "", nil},
		{"comments and blanks", "# c\n\n  # indented\nA=1", []envFileEntry{{"A", "1"}}},
		{"export prefix", "export A=1\nexport   B=2", []envFileEntry{{"A", "1"}, {"B", "2"}}},
		{"equals inside value", "URL=postgres://u:p@h/db?a=b&c=d", []envFileEntry{{"URL", "postgres://u:p@h/db?a=b&c=d"}}},
		{"empty values", "A=\nB=\"\"\nC=''", []envFileEntry{{"A", ""}, {"B", ""}, {"C", ""}}},
		{"duplicate keys preserved", "A=1\nA=2", []envFileEntry{{"A", "1"}, {"A", "2"}}},
		{"windows line endings", "A=1\r\nB=\"two\"\r\n", []envFileEntry{{"A", "1"}, {"B", "two"}}},
		{"bom", "\xef\xbb\xbfA=1", []envFileEntry{{"A", "1"}}},
		{"double quote escapes", `A="line1\nline2 \"q\" \\"`, []envFileEntry{{"A", "line1\nline2 \"q\" \\"}}},
		{"single quotes are literal", `A='a\nb'`, []envFileEntry{{"A", `a\nb`}}},
		{"multiline double", "A=\"l1\nl2\"\nB=2", []envFileEntry{{"A", "l1\nl2"}, {"B", "2"}}},
		{"multiline single with crlf", "A='l1\r\nl2'\r\nB=2", []envFileEntry{{"A", "l1\nl2"}, {"B", "2"}}},
		{"inline comment unquoted", "A=val # note\nB=a#b", []envFileEntry{{"A", "val"}, {"B", "a#b"}}},
		{"hash kept inside quotes", `A="x # y" # tail`, []envFileEntry{{"A", "x # y"}}},
		{"unmatched quote left as is", "A='\nB=2", []envFileEntry{{"A", "'"}, {"B", "2"}}},
		{"invalid keys skipped", "no-equals\n=nokey\n1BAD=x\nGOOD_1=y", []envFileEntry{{"GOOD_1", "y"}}},
		{"spaces around equals", "A = b ", []envFileEntry{{"A", "b"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseEnvFileBytes([]byte(tc.in))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestRenderDotenv_RoundTripAndSecrets(t *testing.T) {
	env := map[string]string{
		"PLAIN":   "v",
		"SPACED":  "a b",
		"MULTI":   "l1\nl2",
		"QUOTES":  `say "hi"`,
		"EMPTY":   "",
		"API_KEY": "must-not-leak",
	}
	out := renderDotenv(env, []string{"API_KEY", "STORE_ONLY"})
	if strings.Contains(out, "must-not-leak") {
		t.Fatalf("export leaked a secret value: %q", out)
	}
	if !strings.Contains(out, "# secret, value not exported\nAPI_KEY=\n") || !strings.Contains(out, "STORE_ONLY=\n") {
		t.Errorf("secret keys not exported empty with a comment: %q", out)
	}
	got := map[string]string{}
	for _, e := range parseEnvFileBytes([]byte(out)) {
		got[e.Key] = e.Value
	}
	for _, k := range []string{"PLAIN", "SPACED", "MULTI", "QUOTES", "EMPTY"} {
		if got[k] != env[k] {
			t.Errorf("round trip %s = %q, want %q", k, got[k], env[k])
		}
	}
}

func TestPlanEnvImport(t *testing.T) {
	current := map[string]string{"A": "1", "B": "old", "S": "x"}
	entries := []envFileEntry{{"A", "1"}, {"B", "new"}, {"C", "3"}, {"C", "4"}, {"S", "y"}}
	plan := planEnvImport(current, entries, []string{"S"}, false)
	if !reflect.DeepEqual(plan.New, []string{"C"}) || !reflect.DeepEqual(plan.Changed, []string{"B"}) ||
		!reflect.DeepEqual(plan.Unchanged, []string{"A"}) || !reflect.DeepEqual(plan.Secret, []string{"S"}) {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Merged["B"] != "new" || plan.Merged["C"] != "4" || plan.Merged["S"] != "x" {
		t.Errorf("merged = %v", plan.Merged)
	}
	keep := planEnvImport(current, entries, []string{"S"}, true)
	if keep.Merged["B"] != "old" {
		t.Errorf("keep-existing merged B = %q, want old", keep.Merged["B"])
	}
}

// fakeEnvServer serves one app and its secret keys, recording the last PUT.
func fakeEnvServer(t *testing.T, gotPut *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web":
			_, _ = io.WriteString(w, `{"name":"web","image":"nginx","port":80,"bind_address":"private","env":{"KEEP":"1","CHG":"old"},"secret_env":["DECLARED"]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/secrets":
			_, _ = io.WriteString(w, `[{"key":"API_KEY"}]`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/web":
			_ = json.NewDecoder(r.Body).Decode(gotPut)
			_, _ = io.WriteString(w, `{"name":"web","image":"nginx","port":80,"bind_address":"private"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_AppsEnvImport(t *testing.T) {
	var put map[string]any
	srv := fakeEnvServer(t, &put)
	file := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(file, []byte("NEW=n\nCHG=fresh\nKEEP=1\nAPI_KEY=leak\nDECLARED=leak2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, _ := runCLIExpectOK(t, []string{"apps", "env", "import", "web", "--file", file, "--dry-run", "--api-url", srv.URL})
	if put != nil {
		t.Fatalf("dry run must not PUT, got %v", put)
	}
	for _, want := range []string{"+ NEW (new)", "~ CHG (changed)", "= KEEP (unchanged)", "! API_KEY", "! DECLARED", "dry run"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %q", want, stdout)
		}
	}

	runCLIExpectOK(t, []string{"apps", "env", "import", "web", "--file", file, "--api-url", srv.URL})
	env, _ := put["env"].(map[string]any)
	if env["NEW"] != "n" || env["CHG"] != "fresh" || env["KEEP"] != "1" {
		t.Errorf("PUT env = %v", env)
	}
	if _, leaked := env["API_KEY"]; leaked {
		t.Errorf("secret key written to plain env: %v", env)
	}
}

func TestRun_AppsEnvExport(t *testing.T) {
	var put map[string]any
	srv := fakeEnvServer(t, &put)
	stdout, _ := runCLIExpectOK(t, []string{"apps", "env", "export", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "KEEP=1\n") || !strings.Contains(stdout, "# secret, value not exported\nAPI_KEY=\n") || !strings.Contains(stdout, "DECLARED=\n") {
		t.Errorf("export = %q", stdout)
	}

	out := filepath.Join(t.TempDir(), "out.env")
	runCLIExpectOK(t, []string{"apps", "env", "export", "web", "--out", out, "--api-url", srv.URL})
	data, err := os.ReadFile(out) //nolint:gosec // path is a t.TempDir() file
	if err != nil || string(data) != stdout {
		t.Errorf("file = %q err=%v, want %q", data, err, stdout)
	}
}
