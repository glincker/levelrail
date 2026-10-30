package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/platformimport/fakeheroku"
)

func readEmitted(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // test temp dir
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(b)
	}
	return out
}

func validateEmittedDir(t *testing.T, files map[string]string) {
	t.Helper()
	var srcs []iac.Source
	for n, c := range files {
		if strings.HasSuffix(n, ".yaml") && n != "stack.yaml" {
			srcs = append(srcs, iac.Source{Name: n, Data: []byte(c)})
		}
	}
	docs, issues := iac.ParseDocuments(srcs)
	if len(issues) == 0 {
		_, issues = iac.Build(docs, iac.Options{})
	}
	if len(issues) > 0 {
		t.Fatalf("emitted documents do not validate: %v", issues)
	}
}

func TestImportConfigEmitStack(t *testing.T) {
	src := writeConfigDir(t, map[string]string{
		"Procfile":    "web: node server.js\n",
		"vercel.json": `{"framework":"nextjs","crons":[{"path":"/api/tick","schedule":"0 * * * *"}]}`,
	})
	out := filepath.Join(t.TempDir(), "stack")
	code, stdout, stderr := runConfigCmd(t, nil, src, "--emit", "stack", "--out", out, "--stack", "shop", "--image-repo", "ghcr.io/acme")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	files := readEmitted(t, out)
	validateEmittedDir(t, files)
	for _, want := range []string{"name: web\n", "name: web-vercel\n"} {
		if !strings.Contains(files["apps.yaml"], want) {
			t.Errorf("apps.yaml misses %q:\n%s", want, files["apps.yaml"])
		}
	}
	if !strings.Contains(files["stack.yaml"], "name: shop") || files["SECRETS.md"] == "" || files["MIGRATION.md"] == "" {
		t.Errorf("incomplete output: %v", files)
	}
	if !strings.Contains(stdout, "stack plan shop") {
		t.Errorf("missing next steps:\n%s", stdout)
	}

	code, _, stderr = runConfigCmd(t, nil, src, "--emit", "stack", "--out", out, "--image-repo", "ghcr.io/acme")
	if code != exitValidation || !strings.Contains(stderr, "--force") {
		t.Errorf("rewrite without --force: code=%d stderr=%q", code, stderr)
	}
	if code, _, _ = runConfigCmd(t, nil, src, "--emit", "stack", "--out", out, "--image-repo", "ghcr.io/acme", "--force"); code != exitOK {
		t.Errorf("--force code=%d", code)
	}
}

func TestImportConfigEmitNeedsImageRepoAndOut(t *testing.T) {
	src := writeConfigDir(t, map[string]string{"Procfile": "web: node server.js\n"})
	out := filepath.Join(t.TempDir(), "stack")
	code, _, stderr := runConfigCmd(t, nil, src, "--emit", "stack", "--out", out)
	if code != exitValidation || !strings.Contains(stderr, "--image-repo") {
		t.Errorf("want an --image-repo error, code=%d stderr=%q", code, stderr)
	}
	if code, _, _ = runConfigCmd(t, nil, src, "--emit", "stack", "--image-repo", "r/x"); code != exitValidation {
		t.Errorf("missing --out code=%d", code)
	}
	if code, _, _ = runConfigCmd(t, nil, src, "--emit", "bogus", "--out", out); code != exitValidation {
		t.Errorf("bad --emit code=%d", code)
	}
}

func TestImportPlatformEmitStackHeroku(t *testing.T) {
	srv := httptest.NewServer(fakeheroku.New().Handler())
	defer srv.Close()
	out := filepath.Join(t.TempDir(), "stack")
	var stdout, stderr bytes.Buffer
	env := map[string]string{envImportSourceToken: fakeheroku.Token}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	code := runImportPlatform("levelrail-cli", []string{"heroku", "--url", srv.URL, "--allow-loopback", "--emit", "stack", "--out", out,
		"--image-repo", "ghcr.io/acme", "--db-version", "postgres=16,redis=7.4", "--json"}, strings.NewReader(""), &stdout, &stderr, lookup)
	if code != exitOK {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var res importEmitResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil || res.Stack != "heroku-import" || len(res.Files) == 0 {
		t.Fatalf("result %+v err=%v", res, err)
	}
	files := readEmitted(t, out)
	validateEmittedDir(t, files)
	if strings.Contains(files["SECRETS.md"], "postgres://") || !strings.Contains(files["SECRETS.md"], "`DATABASE_URL`") {
		t.Errorf("SECRETS.md should list names only:\n%s", files["SECRETS.md"])
	}
}
