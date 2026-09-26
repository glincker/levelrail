package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type importFake struct {
	plan     apiclient.ImportPlan
	requests []string
	bodies   map[string][]byte
}

func newImportServer(t *testing.T, f *importFake) *httptest.Server {
	t.Helper()
	f.bodies = map[string][]byte{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		key := r.Method + " " + r.URL.Path
		f.requests = append(f.requests, key)
		f.bodies[key] = b
		w.Header().Set("Content-Type", "application/json")
		switch key {
		case "POST /api/v1/imports/plan":
			var req apiclient.ImportPlanRequest
			_ = json.Unmarshal(b, &req)
			p := f.plan
			if len(req.Env) > 0 {
				p.MissingRequiredEnv = []string{}
				for _, k := range f.plan.MissingRequiredEnv {
					if req.Env[k] == "" {
						p.MissingRequiredEnv = append(p.MissingRequiredEnv, k)
					}
				}
			}
			_ = json.NewEncoder(w).Encode(p)
		case "POST /api/v1/apps":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(b)
		case "POST /api/v1/apps/site/builds":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"att1"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestImportPrintsPlanWithoutCreating(t *testing.T) {
	f := &importFake{plan: apiclient.ImportPlan{
		Source: "docker_run", SuggestedName: "pg", Deploy: "app",
		Services: []apiclient.ImportService{{Name: "pg", Image: "postgres:16", Build: "image", Port: 5432,
			Env: []apiclient.ImportEnvVar{{Key: "POSTGRES_PASSWORD", Required: true, Secret: true}}}},
		Warnings:           []apiclient.ImportWarning{{Code: "unsupported_flag", Message: "--privileged: not supported"}},
		MissingRequiredEnv: []string{"POSTGRES_PASSWORD"},
	}}
	srv := newImportServer(t, f)
	defer srv.Close()

	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "--docker-run", "docker run postgres:16", "--api-url", srv.URL}, &out, &errb, envMap())
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	for _, want := range []string{"postgres:16", "POSTGRES_PASSWORD", "required, secret", "--privileged", "required env with no value"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
	if len(f.requests) != 1 {
		t.Errorf("requests = %v, want only the plan call", f.requests)
	}
}

func TestImportDeployBlockedByMissingEnv(t *testing.T) {
	f := &importFake{plan: apiclient.ImportPlan{Source: "image", SuggestedName: "x", Deploy: "app",
		Services:           []apiclient.ImportService{{Name: "x", Image: "x:1", Build: "image", Port: 80}},
		MissingRequiredEnv: []string{"API_TOKEN", "DB_PASS"}}}
	srv := newImportServer(t, f)
	defer srv.Close()
	orig := importPromptFn
	importPromptFn = func(string, bool, io.Writer) (string, bool, error) { return "", false, nil }
	defer func() { importPromptFn = orig }()

	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "x:1", "--deploy", "--api-url", srv.URL}, &out, &errb, envMap())
	if code == exitOK {
		t.Fatal("expected failure")
	}
	if !strings.Contains(errb.String(), "API_TOKEN, DB_PASS") {
		t.Errorf("stderr = %s", errb.String())
	}
	for _, r := range f.requests {
		if r == "POST /api/v1/apps" {
			t.Error("app must not be created")
		}
	}
}

func TestImportDeployBuildWithEnvFlag(t *testing.T) {
	f := &importFake{plan: apiclient.ImportPlan{
		Source: "repo", SuggestedName: "site", Deploy: "build", RepoURL: "https://github.com/a/site", Ref: "main",
		Services: []apiclient.ImportService{{Name: "site", Build: "railpack", Port: 3000, HealthPath: "/",
			Env: []apiclient.ImportEnvVar{{Key: "API_TOKEN", Required: true, Secret: true}, {Key: "MODE", HasDefault: true, Value: "prod"}}}},
		MissingRequiredEnv: []string{"API_TOKEN"},
	}}
	srv := newImportServer(t, f)
	defer srv.Close()

	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "https://github.com/a/site", "--deploy", "--env", "API_TOKEN=abc", "--api-url", srv.URL, "--json"}, &out, &errb, envMap())
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	var app apiclient.AppResource
	if err := json.Unmarshal(f.bodies["POST /api/v1/apps"], &app); err != nil {
		t.Fatal(err)
	}
	if app.Name != "site" || !strings.HasSuffix(app.Image, ":pending") || app.Secrets["API_TOKEN"] != "abc" || app.Env["MODE"] != "prod" || app.Health == nil {
		t.Errorf("app body = %+v", app)
	}
	var build apiclient.BuildTriggerRequest
	if err := json.Unmarshal(f.bodies["POST /api/v1/apps/site/builds"], &build); err != nil {
		t.Fatal(err)
	}
	if build.RepoURL != "https://github.com/a/site" || build.Ref != "main" || build.Build.Type != "railpack" {
		t.Errorf("build body = %+v", build)
	}
}

func TestImportPromptsForMissingEnv(t *testing.T) {
	f := &importFake{plan: apiclient.ImportPlan{Source: "image", SuggestedName: "site", Deploy: "app",
		Services:           []apiclient.ImportService{{Name: "site", Image: "x:1", Build: "image", Port: 80, Env: []apiclient.ImportEnvVar{{Key: "TOKEN", Required: true, Secret: true}}}},
		MissingRequiredEnv: []string{"TOKEN"}}}
	srv := newImportServer(t, f)
	defer srv.Close()
	orig := importPromptFn
	importPromptFn = func(key string, _ bool, _ io.Writer) (string, bool, error) { return "typed-" + key, true, nil }
	defer func() { importPromptFn = orig }()

	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "x:1", "--deploy", "--api-url", srv.URL}, &out, &errb, envMap())
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	if !strings.Contains(string(f.bodies["POST /api/v1/apps"]), "typed-TOKEN") {
		t.Errorf("app body = %s", f.bodies["POST /api/v1/apps"])
	}
}

func TestImportInputValidation(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run("cli", []string{"import"}, &out, &errb, envMap()); code == exitOK {
		t.Error("no input must fail")
	}
	if code := run("cli", []string{"import", "nginx", "--docker-run", "docker run x"}, &out, &errb, envMap()); code == exitOK {
		t.Error("two inputs must fail")
	}
}
