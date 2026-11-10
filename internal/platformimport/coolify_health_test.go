package platformimport

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestCoolifyCommandHealthCheck(t *testing.T) {
	routes := map[string]string{
		"/api/v1/projects":                 "coolify/projects.json",
		"/api/v1/projects/p1/environments": "coolify/environments.json",
		"/api/v1/projects/p1/production":   "coolify/env_resources_health.json",
	}
	for _, id := range []string{"app-cmd", "app-empty", "app-http"} {
		routes["/api/v1/applications/"+id+"/envs"] = "coolify/empty_list.json"
		routes["/api/v1/applications/"+id+"/storages"] = "coolify/empty_storages.json"
		routes["/api/v1/applications/"+id+"/scheduled-tasks"] = "coolify/empty_list.json"
	}
	srv := readOnlyServer(t, routes, "", "Authorization", "Bearer tok-fixture")
	src, err := NewCoolify(srv.URL, "tok-fixture", testOpts)
	if err != nil {
		t.Fatal(err)
	}
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]App{}
	for _, a := range d.Apps {
		byName[a.Name] = a
	}

	cmd := byName["cmd-app"].Health
	want := []string{"/bin/sh", "-c", "pg_isready -U app"}
	if cmd == nil || !reflect.DeepEqual(cmd.Exec, want) || cmd.IntervalSeconds != 30 || cmd.TimeoutSeconds != 5 || cmd.Retries != 3 {
		t.Fatalf("command check: %+v", cmd)
	}
	if e := byName["empty-app"]; e.Health != nil || len(e.Notes) == 0 {
		t.Errorf("empty command must keep the note: %+v", e)
	}
	if h := byName["http-app"].Health; h == nil || h.Path != "/healthz" || h.Exec != nil {
		t.Errorf("http check changed: %+v", h)
	}
	if coolifyHealthExec(coolifyApp{HealthCheckType: "CMD", HealthCheckCommand: strings.Repeat("a", 4097)}) != nil {
		t.Error("oversized command must not map")
	}
	if coolifyHealthExec(coolifyApp{HealthCheckType: "tcp", HealthCheckCommand: "true"}) != nil {
		t.Error("unknown type must not map")
	}

	files, err := EmitStack(BuildPlan(d, PlanOptions{}).EmitInput(), EmitOptions{Stack: "s", Project: "p", ImageRepo: "r.example.com/a"})
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for _, f := range files {
		out.Write(f.Content)
	}
	for _, w := range []string{"exec: [/bin/sh, -c, pg_isready -U app]", "interval: 30s", "timeout: 5s", "failures: 3", "path: /healthz"} {
		if !strings.Contains(out.String(), w) {
			t.Errorf("emitted spec lacks %q:\n%s", w, out.String())
		}
	}
}
