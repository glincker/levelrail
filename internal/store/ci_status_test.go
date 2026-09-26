package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestGitSourceDeploySettings(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveGitSource(ctx, GitSource{ServiceName: "web", RepoURL: "https://github.com/o/r", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetGitSource(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if !got.ReportStatus || len(got.DeployPaths) != 0 || len(got.DeployPathsIgnore) != 0 {
		t.Fatalf("defaults = %+v, want report on and no filters", got)
	}

	if err := db.SetGitSourceDeploySettings(ctx, "web", []string{"src/**"}, []string{"**/*.md"}, false); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetGitSource(ctx, "web")
	if !reflect.DeepEqual(got.DeployPaths, []string{"src/**"}) || !reflect.DeepEqual(got.DeployPathsIgnore, []string{"**/*.md"}) || got.ReportStatus {
		t.Fatalf("after set = %+v", got)
	}

	if err := db.SaveGitSource(ctx, GitSource{ServiceName: "web", RepoURL: "https://github.com/o/r", Branch: "dev", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetGitSource(ctx, "web")
	if len(got.DeployPaths) != 1 || got.ReportStatus {
		t.Fatalf("an ordinary source edit reset the deploy settings: %+v", got)
	}

	if err := db.SetGitSourceDeploySettings(ctx, "missing", nil, nil, true); !errors.Is(err, ErrGitSourceNotFound) {
		t.Fatalf("missing source: err = %v", err)
	}
}

func TestPipelineRunReport(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	pl, err := db.SavePipeline(ctx, Pipeline{ID: "p1", AppName: "web", Name: "ci", YAML: "a", Enabled: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	run, err := db.CreatePipelineRun(ctx, PipelineRun{ID: "r1", PipelineID: pl.ID, AppName: "web", TriggerKind: "manual", Definition: "x", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetPipelineRunReport(ctx, run.ID, "github", "pending", "https://forge/o/r/commit/abc", ""); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPipelineRunReport(ctx, run.ID, "github", "", "", "rate limited"); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetPipelineRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReportProvider != "github" || got.ReportState != "pending" || got.ReportURL == "" || got.ReportWarning != "rate limited" {
		t.Fatalf("a blank state must keep the last posted one: %+v", got)
	}
	if err := db.SetPipelineRunReport(ctx, run.ID, "github", "success", "", ""); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetPipelineRun(ctx, run.ID)
	if got.ReportState != "success" || got.ReportWarning != "" {
		t.Fatalf("a later success must clear the warning: %+v", got)
	}
	if err := db.SetPipelineRunReport(ctx, "nope", "github", "", "", ""); !errors.Is(err, ErrPipelineRunNotFound) {
		t.Fatalf("missing run: err = %v", err)
	}
}

func TestForgeDeployments(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for i, id := range []string{"a", "b", "c"} {
		env := "production"
		if id == "c" {
			env = "preview"
		}
		d := ForgeDeployment{ID: id, AppName: "web", Environment: env, Provider: "github", ExternalID: int64(i + 1), CommitSHA: "s", State: "success", CreatedAt: now.Add(time.Duration(i) * time.Second), UpdatedAt: now}
		if err := db.CreateForgeDeployment(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.ListForgeDeploymentsByState(ctx, "web", "production", "success", "b")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("list = %+v, want only the other production deployment", got)
	}
	if err := db.SetForgeDeploymentState(ctx, "a", "inactive", "warn", now); err != nil {
		t.Fatal(err)
	}
	got, _ = db.ListForgeDeploymentsByState(ctx, "web", "production", "success", "b")
	if len(got) != 0 {
		t.Fatalf("an inactive deployment is still listed as success: %+v", got)
	}
}
