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

const applyResultJSON = `{"repo_url":"https://github.com/acme/mono","branch":"main","dry_run":false,"needs_confirmation":false,"confirm_above":10,
"counts":{"created":1,"failed":1},
"results":[{"package":"api","name":"mono-api","directory":"apps/api","status":"created","app_id":"app_1"},
{"package":"web","name":"mono-web","directory":"apps/web","status":"failed","code":"name_taken","message":"an app with this name already exists"}]}`

type applyServer struct {
	bodies []map[string]any
}

func (a *applyServer) handler(counts string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/plan") {
			_, _ = w.Write([]byte(repoPlanJSON))
			return
		}
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		a.bodies = append(a.bodies, m)
		_, _ = w.Write([]byte(counts))
	}
}

func TestRun_ImportRepoApply_PrintsResultsAndExitsNonZeroOnFailure(t *testing.T) {
	a := &applyServer{}
	srv := httptest.NewServer(a.handler(applyResultJSON))
	defer srv.Close()
	var out, errb bytes.Buffer
	code := run("levelrail-cli-test", []string{"import", "repo", "apply", "https://github.com/acme/mono", "--set", "web.LOG_LEVEL=debug", "--secret", "api.DB_PASSWORD=pw", "--project", "p1", "--api-url", srv.URL}, &out, &errb, envMap())
	if code != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%s)", code, exitValidation, errb.String())
	}
	for _, want := range []string{"NAME", "mono-api", "created", "mono-web", "failed", "name_taken", "1 created, 1 failed"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout missing %q:\n%s", want, out.String())
		}
	}
	if len(a.bodies) != 2 || a.bodies[0]["dry_run"] != true || a.bodies[1]["dry_run"] == true {
		t.Fatalf("expected a dry run then a real apply, got %v", a.bodies)
	}
	pkgs := a.bodies[1]["packages"].([]any)
	web := pkgs[0].(map[string]any)
	if web["project_id"] != "p1" || web["env"].(map[string]any)["LOG_LEVEL"] != "debug" {
		t.Errorf("web row = %v", web)
	}
}

func TestRun_ImportRepoApply_DryRunAndConfirmGate(t *testing.T) {
	a := &applyServer{}
	srv := httptest.NewServer(a.handler(`{"dry_run":true,"needs_confirmation":true,"confirm_above":1,"counts":{"would_create":2},"results":[{"name":"mono-web","directory":"apps/web","status":"would_create"},{"name":"mono-api","directory":"apps/api","status":"would_create"}]}`))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"import", "repo", "apply", "https://github.com/acme/mono", "--dry-run", "--api-url", srv.URL})
	if !strings.Contains(stdout, "dry run: 2 would_create") || len(a.bodies) != 1 {
		t.Fatalf("dry run output = %s, calls = %d", stdout, len(a.bodies))
	}

	a.bodies = nil
	var out, errb bytes.Buffer
	if code := run("levelrail-cli-test", []string{"import", "repo", "apply", "https://github.com/acme/mono", "--api-url", srv.URL}, &out, &errb, envMap()); code != exitValidation || !strings.Contains(errb.String(), "--yes") {
		t.Fatalf("exit = %d stderr = %s, want a --yes hint", code, errb.String())
	}
	if len(a.bodies) != 1 {
		t.Fatalf("unconfirmed import sent %d apply calls, want only the dry run", len(a.bodies))
	}
}

func TestRepoApplyPackages_FiltersAndRejectsStrayOverrides(t *testing.T) {
	var plan = mustRepoPlan(t)
	f := repoApplyFlags{packages: "api", set: stringMapFlag{"api.A": "1"}, secret: stringMapFlag{}}
	got, err := repoApplyPackages(plan, f)
	if err != nil || len(got) != 1 || got[0].Directory != "apps/api" || got[0].Env["A"] != "1" {
		t.Fatalf("got = %+v, err = %v", got, err)
	}
	f.set = stringMapFlag{"ghost.A": "1"}
	if _, err := repoApplyPackages(plan, f); err == nil {
		t.Fatal("override for an unselected package must be rejected")
	}
	if _, err := repoApplyPackages(plan, repoApplyFlags{packages: "nope", set: stringMapFlag{}, secret: stringMapFlag{}}); err == nil {
		t.Fatal("empty selection must be rejected")
	}
}

func mustRepoPlan(t *testing.T) apiclient.RepoPlan {
	t.Helper()
	var p apiclient.RepoPlan
	if err := json.Unmarshal([]byte(repoPlanJSON), &p); err != nil {
		t.Fatal(err)
	}
	return p
}
