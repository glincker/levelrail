package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// fakeBuildApp is a static Coolify app with fake values only.
func fakeBuildApp() (coolifyApplication, []coolifyEnvVar) {
	app := coolifyApplication{
		UUID: "u-build", Name: "docs", BuildPack: "static",
		GitRepository: "https://git.example.com/acme/docs.git", GitBranch: "main",
		BaseDirectory: "/site", InstallCommand: "npm ci", BuildCommand: "npm run build", PublishDirectory: "/dist",
		PreDeployment: "echo pre", PostDeployment: "echo post",
		Redirect: "www", CustomLabels: "traefik.enable=true", WebhookGithub: "fake-hook-value",
		Tags: json.RawMessage(`[{"name":"web"},{"name":"blue"}]`),
	}
	envs := []coolifyEnvVar{
		{Key: "PUBLIC_API_URL", RealValue: "https://api.example.com", IsBuildTime: true},
		{Key: "RUNTIME_ONLY", RealValue: "fake-runtime"},
		{Key: "NODE_VERSION", RealValue: "20"},
	}
	return app, envs
}

func TestCoolifyBuildSettingsMapToSpec(t *testing.T) {
	app, envs := fakeBuildApp()
	got := mapCoolifyApplication(app, envs, true)
	if got.Blocking {
		t.Fatalf("unexpected blocking: %+v", got.Issues)
	}
	svc := got.Service
	if svc.BaseDirectory != "site" || svc.BuildCommand != "npm ci && npm run build" || svc.PublishDir != "dist" {
		t.Fatalf("build settings = %+v", svc)
	}
	if svc.Hooks != nil {
		t.Fatalf("static app must not carry hooks: %+v", svc.Hooks)
	}
	df := app
	df.BuildPack, df.PortsExposes = "dockerfile", "3000"
	dm := mapCoolifyApplication(df, envs, false)
	if h := dm.Service.Hooks; h == nil || h.PreDeploy != "echo pre" || h.PostDeploy != "echo post" {
		t.Fatalf("dockerfile hooks = %+v", h)
	}
	ddata, err := buildAppYAML(dm)
	if err != nil {
		t.Fatal(err)
	}
	if ddoc, err := spec.Parse(ddata); err != nil || ddoc.Services["docs"].Hooks == nil || ddoc.Services["docs"].Hooks.PostDeploy != "echo post" {
		t.Fatalf("dockerfile spec hooks: %v\n%s", err, ddata)
	}
	if len(svc.BuildTimeKeys) != 1 || svc.BuildTimeKeys[0] != "PUBLIC_API_URL" {
		t.Fatalf("build-time keys = %v", svc.BuildTimeKeys)
	}
	for _, f := range []string{"redirect", "custom_labels", "deploy_webhook", "tags", "node_version"} {
		found := false
		for _, iss := range got.Issues {
			found = found || iss.Field == f
		}
		if !found {
			t.Errorf("no report entry for %s: %+v", f, got.Issues)
		}
	}
	report, _ := json.Marshal(got)
	if strings.Contains(string(report), "fake-hook-value") || strings.Contains(string(report), "fake-runtime") {
		t.Fatalf("report leaked a secret value: %s", report)
	}

	data, err := buildAppYAML(got)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := spec.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	b := doc.Services["docs"].Build
	if b.Type != spec.BuildStatic || b.Command != "npm ci && npm run build" || b.Publish != "dist" || b.BaseDirectory != "site" {
		t.Fatalf("spec build = %+v\n%s", b, data)
	}
	if strings.Contains(string(data), "fake-runtime") || strings.Contains(string(data), "api.example.com") {
		t.Fatalf("app.yaml holds an env value:\n%s", data)
	}
}

func TestCoolifyBuildSettingsRejectUnsafeInput(t *testing.T) {
	app := coolifyApplication{
		Name: "api", BuildPack: "dockerfile", BaseDirectory: "../../etc", DockerfileLocation: "/../Dockerfile",
		StartCommand: "API_TOKEN=fake-inline node server.js", BuildCommand: "make",
	}
	got := mapCoolifyApplication(app, nil, false)
	svc := got.Service
	if svc.BaseDirectory != "" || svc.BuildPath != "" || svc.StartCommand != "" {
		t.Fatalf("unsafe settings carried: %+v", svc)
	}
	for _, iss := range got.Issues {
		if strings.Contains(iss.Detail, "fake-inline") {
			t.Fatalf("issue printed the inline secret: %+v", iss)
		}
	}
	t.Setenv("APP_IMPORT_COMMAND_MAX", "4")
	if got := mapCoolifyApplication(coolifyApplication{Name: "w", BuildPack: "railpack", StartCommand: "node s.js"}, nil, false); got.Service.StartCommand != "" {
		t.Fatalf("command over the env cap was carried: %q", got.Service.StartCommand)
	}
}

func TestCoolifyBuildPathsJoinBaseDirectory(t *testing.T) {
	df := mapCoolifyApplication(coolifyApplication{Name: "web", BuildPack: "dockerfile", BaseDirectory: "/apps/web", DockerfileLocation: "/Dockerfile.prod"}, nil, false)
	if df.Service.BuildPath != "apps/web/Dockerfile.prod" || df.Service.BaseDirectory != "apps/web" {
		t.Fatalf("dockerfile paths = %+v", df.Service)
	}
	st := mapCoolifyApplication(coolifyApplication{Name: "site", BuildPack: "static", BaseDirectory: "/apps/web", PublishDirectory: "/public"}, nil, false)
	if st.Service.BuildPath != "apps/web/public" {
		t.Fatalf("static path = %q", st.Service.BuildPath)
	}
}

func TestCoolifyCommandWithFetchedValueDropped(t *testing.T) {
	envs := []coolifyEnvVar{{Key: "DEPLOY_KEY", RealValue: "fake-value-123"}}
	got := mapCoolifyApplication(coolifyApplication{Name: "w", BuildPack: "railpack", StartCommand: "node s.js fake-value-123"}, envs, true)
	if got.Service.StartCommand != "" {
		t.Fatal("a command embedding an env value was carried")
	}
	report, _ := json.Marshal(got.Issues)
	if strings.Contains(string(report), "fake-value-123") {
		t.Fatalf("issues printed the value: %s", report)
	}
}

// recordingTarget answers the target API and records each request body by method and path.
func recordingTarget(t *testing.T) (*httptest.Server, func() map[string]string) {
	t.Helper()
	var mu sync.Mutex
	bodies := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies[r.Method+" "+r.URL.Path] = string(b)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps" {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(b)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() map[string]string {
		mu.Lock()
		defer mu.Unlock()
		return bodies
	}
}

func TestApplyGitBuildNeverLeaksUserinfo(t *testing.T) {
	srv, bodies := recordingTarget(t)
	fakeURL := (&url.URL{Scheme: "https", User: url.UserPassword("user", "fake_pat"), Host: "git.example.com", Path: "/acme/api.git"}).String()
	dok := mapDokployApplication(dokployApplication{Name: "api", SourceType: "git", BuildType: "dockerfile",
		CustomGitURL: fakeURL, CustomGitBranch: "main"}, nil, false)
	report := &migrationReport{Apps: []mappedApp{dok}}
	applyMigration(context.Background(), NewClient(srv.URL, "target-token"), report)
	got := bodies()
	all, _ := json.Marshal(report)
	for _, b := range got {
		all = append(all, b...)
	}
	if strings.Contains(string(all), "fake_pat") {
		t.Fatalf("credential leaked: %s", all)
	}
}

func TestCoolifyBuildSettingsApplyCalls(t *testing.T) {
	var mu sync.Mutex
	bodies := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies[r.Method+" "+r.URL.Path] = string(b)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"docs","image":"pending"}`))
		case strings.Contains(r.URL.Path, "/secrets/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	app, envs := fakeBuildApp()
	report := &migrationReport{Apps: []mappedApp{mapCoolifyApplication(app, envs, true)}}
	applyMigration(context.Background(), NewClient(srv.URL, "target-token"), report)
	if !report.Apps[0].Applied {
		t.Fatalf("not applied: %+v", report.Apps[0])
	}
	if b := bodies["POST /api/v1/apps"]; strings.Contains(b, `"hooks"`) {
		t.Errorf("static create body carries hooks: %s", b)
	}
	git := bodies["PUT /api/v1/apps/docs/git-source"]
	for _, w := range []string{`"build_command":"npm ci `, `npm run build"`, `"publish_dir":"dist"`, `"base_directory":"site"`, `"build_type":"static"`} {
		if !strings.Contains(git, w) {
			t.Errorf("git-source body lacks %s: %s", w, git)
		}
	}
	if b := bodies["PUT /api/v1/apps/docs/secrets/PUBLIC_API_URL"]; !strings.Contains(b, `"build_time":true`) {
		t.Errorf("build-time secret body = %s", b)
	}
	if b := bodies["PUT /api/v1/apps/docs/secrets/RUNTIME_ONLY"]; !strings.Contains(b, `"build_time":false`) {
		t.Errorf("runtime secret body = %s", b)
	}
	verify := false
	for _, iss := range report.Apps[0].Issues {
		verify = verify || strings.HasPrefix(iss.Detail, "verify repository access")
	}
	if !verify {
		t.Error("no verify repository access item")
	}
}
