package platformimport

import (
	"context"
	"strings"
	"testing"
)

func TestCoolifyEnvironmentsImport(t *testing.T) {
	srv := readOnlyServer(t, map[string]string{
		"/api/v1/projects":                             "coolify/projects.json",
		"/api/v1/projects/p1/environments":             "coolify/environments_two.json",
		"/api/v1/projects/p1/production":               "coolify/env_resources.json",
		"/api/v1/projects/p1/Staging Env":              "coolify/env_resources_empty.json",
		"/api/v1/applications/app-web/envs":            "coolify/envs_web.json",
		"/api/v1/applications/app-web/storages":        "coolify/storages_web.json",
		"/api/v1/applications/app-web/scheduled-tasks": "coolify/tasks_web.json",
		"/api/v1/applications/app-img/envs":            "coolify/empty_list.json",
		"/api/v1/applications/app-img/storages":        "coolify/empty_storages.json",
		"/api/v1/applications/app-img/scheduled-tasks": "coolify/empty_list.json",
		"/api/v1/applications/app-nix/envs":            "coolify/empty_list.json",
		"/api/v1/applications/app-nix/storages":        "coolify/empty_storages.json",
		"/api/v1/applications/app-nix/scheduled-tasks": "coolify/empty_list.json",
	}, "", "Authorization", "Bearer tok-fixture")
	src, err := NewCoolify(srv.URL, "tok-fixture", testOpts)
	if err != nil {
		t.Fatal(err)
	}
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	plan := BuildPlan(d, PlanOptions{})
	if len(plan.Environments) != 2 {
		t.Fatalf("environments: %+v", plan.Environments)
	}
	if e := plan.Environments[1]; e.Project != "Main Project" || e.Name != "Staging Env" {
		t.Errorf("empty environment must still be planned: %+v", e)
	}
	if plan.Apps[0].Environment != "production" || plan.Apps[0].Project != "Main Project" {
		t.Errorf("app environment: %+v", plan.Apps[0])
	}
	dry := 0
	for _, it := range plan.Report.Items {
		if it.Kind == "environment" && it.Status == StatusMapped {
			dry++
		}
	}
	if dry != 2 {
		t.Errorf("dry run lists %d environments, want 2", dry)
	}

	ap := &envRecorder{}
	rep := Apply(context.Background(), plan, ap)
	if len(ap.created) != 2 || rep.Counts[StatusCreated] < 2 {
		t.Errorf("created %v counts %v", ap.created, rep.Counts)
	}

	again := BuildPlan(d, PlanOptions{ExistingEnvironments: map[string]bool{
		EnvironmentKey("Main Project", "production"): true, EnvironmentKey("Main Project", "Staging Env"): true,
	}})
	if len(again.Environments) != 0 || again.Report.Counts[StatusAlready] < 2 {
		t.Errorf("rerun must not recreate environments: %v", again.Report.Counts)
	}

	files, err := EmitStack(plan.EmitInput(), EmitOptions{Stack: "s", Project: "p", ImageRepo: "r.example.com/a"})
	if err != nil {
		t.Fatal(err)
	}
	var proj string
	for _, f := range files {
		if f.Name == EmitProjectFile {
			proj = string(f.Content)
		}
	}
	if !strings.Contains(proj, "name: main-project") || !strings.Contains(proj, "name: staging-env") {
		t.Errorf("emitted project file lacks the environments:\n%s", proj)
	}
}

type envRecorder struct{ created []string }

func (r *envRecorder) CreateEnvironment(_ context.Context, p EnvironmentPlan) ([]string, error) {
	r.created = append(r.created, p.SourceID)
	return nil, nil
}
func (r *envRecorder) CreateApp(context.Context, AppPlan) ([]string, error) { return nil, nil }
func (r *envRecorder) CreateDatabase(context.Context, DatabasePlan) ([]string, error) {
	return nil, nil
}
