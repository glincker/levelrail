package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/pipeline"
)

func TestRun_AppsGitSourceSettings(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.GitDeploySettings{DeployPaths: []string{"src/**", "go.mod"}, DeployPathsIgnore: []string{}, ReportStatus: false})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "git-source", "settings", "web", "--paths", "src/**,go.mod", "--paths-ignore", "", "--report-status=false", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/git-source/deploy-settings" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if !reflect.DeepEqual(gotBody["deploy_paths"], []any{"src/**", "go.mod"}) || !reflect.DeepEqual(gotBody["deploy_paths_ignore"], []any{}) || gotBody["report_status"] != false {
		t.Errorf("body = %v", gotBody)
	}
	if !strings.Contains(stdout, "src/**, go.mod") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_AppsGitSourceSettings_OnlyGivenFlagsSent(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deploy_paths":[],"deploy_paths_ignore":[],"report_status":true}`))
	}))
	defer srv.Close()
	runCLIExpectOK(t, []string{"apps", "git-source", "settings", "web", "--report-status=true", "--api-url", srv.URL})
	if _, ok := gotBody["deploy_paths"]; ok {
		t.Errorf("body = %v, want only report_status", gotBody)
	}
}

func TestRun_AppsGitSourceSettings_NeedsAFlag(t *testing.T) {
	var stdout, stderr strings.Builder
	if got := run("levelrail-cli-test", []string{"apps", "git-source", "settings", "web"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestFilterEditApply(t *testing.T) {
	src := "version: 1\nname: ci\non:\n  push:\n    branches: [main]\njobs:\n  t:\n    image: a\n    steps:\n      - run: echo\n"
	paths, ignore, off := []string{"src/**"}, []string{"**/*.md"}, false
	out, err := filterEdit{paths: &paths, ignore: &ignore, report: &off}.apply([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	def, issues := pipeline.Validate(out)
	if len(issues) > 0 {
		t.Fatalf("issues: %v\n%s", issues, out)
	}
	if !reflect.DeepEqual([]string(def.On.Push.Paths), paths) || !reflect.DeepEqual([]string(def.On.Push.PathsIgnore), ignore) || def.ReportsStatus() {
		t.Fatalf("definition = %+v", def.On.Push)
	}
	if same, err := (filterEdit{}).apply([]byte(src)); err != nil || string(same) != src {
		t.Fatalf("no-op edit changed the file: %v", err)
	}
	manual := "version: 1\nname: ci\non:\n  manual:\njobs:\n  t:\n    image: a\n    steps:\n      - run: echo\n"
	if _, err := (filterEdit{paths: &paths}).apply([]byte(manual)); err == nil {
		t.Fatal("paths on a manual-only pipeline was accepted")
	}
}

func TestReportSummary(t *testing.T) {
	tests := []struct {
		r    *apiclient.PipelineReport
		want string
	}{
		{nil, "-"},
		{&apiclient.PipelineReport{Provider: "github", State: "success"}, "github:success"},
		{&apiclient.PipelineReport{Provider: "github", Warning: "rate limited"}, "warning"},
	}
	for _, tt := range tests {
		if got := reportSummary(tt.r); got != tt.want {
			t.Errorf("reportSummary(%+v) = %q, want %q", tt.r, got, tt.want)
		}
	}
}
