package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

type gitlabCall struct {
	Method, Path string
	Body         map[string]any
}

// fakeGitLab answers the Environments and Deployments endpoints and keeps
// every call; failOn makes a matching call answer 500.
type fakeGitLab struct {
	mu     sync.Mutex
	calls  []gitlabCall
	envs   map[string]int64
	failOn string
}

func newFakeGitLab(t *testing.T) (*fakeGitLab, string) {
	t.Helper()
	fg := &fakeGitLab{envs: map[string]int64{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fg.mu.Lock()
		defer fg.mu.Unlock()
		c := gitlabCall{Method: r.Method, Path: r.URL.EscapedPath()}
		_ = json.NewDecoder(r.Body).Decode(&c.Body)
		fg.calls = append(fg.calls, c)
		if fg.failOn != "" && strings.Contains(c.Method+" "+c.Path, fg.failOn) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(c.Path, "/environments"):
			name := r.URL.Query().Get("name")
			if id, ok := fg.envs[name]; ok {
				_ = json.NewEncoder(w).Encode([]map[string]any{{"id": id, "name": name, "external_url": ""}})
				return
			}
			_, _ = w.Write([]byte("[]"))
		case r.Method == http.MethodPost && strings.HasSuffix(c.Path, "/environments"):
			id := int64(len(fg.envs) + 1)
			fg.envs[c.Body["name"].(string)] = id
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "name": c.Body["name"]})
		case r.Method == http.MethodPost && strings.HasSuffix(c.Path, "/deployments"):
			_, _ = w.Write([]byte(`{"id":77}`))
		}
	}))
	t.Cleanup(srv.Close)
	return fg, srv.URL
}

func (fg *fakeGitLab) paths() []string {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	var out []string
	for _, c := range fg.calls {
		out = append(out, c.Method+" "+c.Path)
	}
	return out
}

type fakeDeployConns struct {
	GitConnectionsStore
	conn store.GitConnection
}

func (f fakeDeployConns) GetGitConnection(context.Context, string) (store.GitConnection, error) {
	return f.conn, nil
}

type fakeStatusApps struct {
	AppStore
	mu      sync.Mutex
	reports []store.ForgeStatusReport
}

func (f *fakeStatusApps) RecordForgeStatusReport(_ context.Context, r store.ForgeStatusReport) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reports = append(f.reports, r)
	return nil
}

type gitlabDeployFixture struct {
	rt   *Router
	rows *fakeForgeDeployments
	apps *fakeStatusApps
	f    *forge
	gs   store.GitSource
	fg   *fakeGitLab
}

// newGitLabDeployFixture wires a router whose GitLab connection holds scopes
// and whose settings come from env.
func newGitLabDeployFixture(t *testing.T, scopes string, env map[string]string) *gitlabDeployFixture {
	t.Helper()
	fg, url := newFakeGitLab(t)
	rows := &fakeForgeDeployments{rows: map[string]store.ForgeDeployment{}}
	apps := &fakeStatusApps{}
	rt := &Router{
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		forgeDeployments: rows,
		apps:             apps,
		gitConns:         fakeDeployConns{conn: store.GitConnection{ID: "gc_1", Provider: gitProviderGitLab, InstanceURL: url, Scopes: scopes}},
		runtimeSettings: runtimesettings.New(nil, func(k string) (string, bool) {
			v, ok := env[k]
			return v, ok
		}),
	}
	f := &forge{kind: forgeGitLab, instanceURL: url, token: "glpat-secret-token", repo: "grp/proj", gitlab: &gitlabapp.Client{HTTP: http.DefaultClient}}
	return &gitlabDeployFixture{rt: rt, rows: rows, apps: apps, f: f, fg: fg, gs: store.GitSource{ReportStatus: true, ConnectionID: "gc_1", RepoURL: url + "/grp/proj"}}
}

var gitlabDeploymentsOn = map[string]string{"APP_GIT_GITLAB_DEPLOYMENTS": "true"}

func TestGitLabDeploymentLifecycle(t *testing.T) {
	x := newGitLabDeployFixture(t, "api", gitlabDeploymentsOn)
	d := x.rt.beginOnGitLab(context.Background(), x.f, x.gs, "web", "abc123", forgeEnvProduction, forgeEnvProduction, "https://web.example.com")
	if d == nil {
		t.Fatal("no deployment was created")
	}
	d.finish(context.Background(), githubapp.DeploymentSuccess, "ok")

	want := []string{
		"GET /api/v4/projects/grp%2Fproj/environments",
		"POST /api/v4/projects/grp%2Fproj/environments",
		"POST /api/v4/projects/grp%2Fproj/deployments",
		"PUT /api/v4/projects/grp%2Fproj/deployments/77",
	}
	got := x.fg.paths()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if env := x.fg.calls[1].Body; env["name"] != "web/production" || env["tier"] != "production" || env["external_url"] != "https://web.example.com" {
		t.Errorf("environment body = %v", env)
	}
	if dep := x.fg.calls[2].Body; dep["status"] != "running" || dep["sha"] != "abc123" || dep["environment"] != "web/production" {
		t.Errorf("deployment body = %v", dep)
	}
	if x.fg.calls[3].Body["status"] != "success" {
		t.Errorf("final status = %v", x.fg.calls[3].Body)
	}
	if st := x.rows.rows[d.rec.ID].State; st != "success" {
		t.Errorf("row state = %s", st)
	}
	if len(x.apps.reports) != 2 || x.apps.reports[1].State != "success" || x.apps.reports[1].Source != forgeStatusSourceDeployment || x.apps.reports[1].Error != "" {
		t.Errorf("reports = %+v", x.apps.reports)
	}
}

func TestGitLabDeploymentFailureStates(t *testing.T) {
	for state, want := range map[githubapp.DeploymentState]string{githubapp.DeploymentFailure: "failed", githubapp.DeploymentInactive: "canceled"} {
		x := newGitLabDeployFixture(t, "api", gitlabDeploymentsOn)
		d := x.rt.beginOnGitLab(context.Background(), x.f, x.gs, "web", "abc", forgeEnvProduction, forgeEnvProduction, "")
		d.finish(context.Background(), state, "x")
		if got := x.fg.calls[len(x.fg.calls)-1].Body["status"]; got != want {
			t.Errorf("%s reported as %v, want %s", state, got, want)
		}
	}
}

func TestGitLabEnvironmentIsReusedAcrossDeploys(t *testing.T) {
	x := newGitLabDeployFixture(t, "api", gitlabDeploymentsOn)
	x.rt.beginOnGitLab(context.Background(), x.f, x.gs, "web", "a", forgeEnvProduction, forgeEnvProduction, "")
	x.rt.beginOnGitLab(context.Background(), x.f, x.gs, "web", "b", forgeEnvProduction, forgeEnvProduction, "")
	creates := 0
	for _, p := range x.fg.paths() {
		if p == "POST /api/v4/projects/grp%2Fproj/environments" {
			creates++
		}
	}
	if creates != 1 {
		t.Fatalf("environment created %d times, want once", creates)
	}
}

func TestGitLabDeploymentMissingScopeCallsNothing(t *testing.T) {
	x := newGitLabDeployFixture(t, "read_api read_repository", gitlabDeploymentsOn)
	if d := x.rt.beginForgeDeployment(context.Background(), "web", x.gs, "abc", forgeEnvProduction, forgeEnvProduction, ""); d != nil {
		t.Fatal("a deployment was created without the api scope")
	}
	if len(x.fg.calls) != 0 || len(x.rows.rows) != 0 {
		t.Fatalf("calls = %v rows = %v, want none", x.fg.paths(), x.rows.rows)
	}
	caps := gitlabCapabilities([]string{"read_api"}, true)
	for _, c := range caps {
		if c.ID == capDeploymentsReprt && c.State != gitCapNeedsScope {
			t.Errorf("capability state = %s, want needs_scope", c.State)
		}
	}
}

func TestGitLabDeploymentIsOffUntilEnabled(t *testing.T) {
	x := newGitLabDeployFixture(t, "api", nil)
	if _, ok := x.rt.gitlabDeploymentsAllowed(context.Background(), x.f, x.gs); ok {
		t.Fatal("reporting is on by default")
	}
	on := newGitLabDeployFixture(t, "api", map[string]string{"APP_GIT_GITLAB_DEPLOYMENTS": "true", "APP_GIT_GITLAB_DEPLOYMENTS_CONNECTIONS": "gc_other"})
	if _, ok := on.rt.gitlabDeploymentsAllowed(context.Background(), on.f, on.gs); ok {
		t.Fatal("a connection outside the list reported")
	}
	listed := newGitLabDeployFixture(t, "api", map[string]string{"APP_GIT_GITLAB_DEPLOYMENTS": "true", "APP_GIT_GITLAB_DEPLOYMENTS_CONNECTIONS": "gc_1"})
	if _, ok := listed.rt.gitlabDeploymentsAllowed(context.Background(), listed.f, listed.gs); !ok {
		t.Fatal("a listed connection did not report")
	}
}

func TestGitLabDeploymentAPIErrorIsRecordedAndMasked(t *testing.T) {
	x := newGitLabDeployFixture(t, "api", gitlabDeploymentsOn)
	x.fg.failOn = "POST /api/v4/projects/grp%2Fproj/deployments"
	if d := x.rt.beginOnGitLab(context.Background(), x.f, x.gs, "web", "abc", forgeEnvProduction, forgeEnvProduction, ""); d != nil {
		t.Fatal("a failed create returned a deployment")
	}
	if len(x.rows.rows) != 1 {
		t.Fatalf("rows = %v, want one error row", x.rows.rows)
	}
	for _, r := range x.rows.rows {
		if r.State != "error" || !strings.Contains(r.Warning, "deployment not created") || strings.Contains(r.Warning, "glpat-secret-token") {
			t.Errorf("row = %+v", r)
		}
	}
	if len(x.apps.reports) != 1 || x.apps.reports[0].Error == "" || x.apps.reports[0].State != "error" || strings.Contains(x.apps.reports[0].Error, "glpat-secret-token") {
		t.Errorf("reports = %+v", x.apps.reports)
	}
}

func TestGitLabStatusUpdateErrorIsRecorded(t *testing.T) {
	x := newGitLabDeployFixture(t, "api", gitlabDeploymentsOn)
	d := x.rt.beginOnGitLab(context.Background(), x.f, x.gs, "web", "abc", forgeEnvProduction, forgeEnvProduction, "")
	x.fg.failOn = "PUT "
	d.finish(context.Background(), githubapp.DeploymentSuccess, "ok")
	last := x.apps.reports[len(x.apps.reports)-1]
	if last.Error == "" || !strings.Contains(x.rows.rows[d.rec.ID].Warning, "not reported") {
		t.Fatalf("report = %+v, row = %+v", last, x.rows.rows[d.rec.ID])
	}
}

func TestGitLabPreviewCloseStopsTheEnvironment(t *testing.T) {
	x := newGitLabDeployFixture(t, "api", gitlabDeploymentsOn)
	d := x.rt.beginOnGitLab(context.Background(), x.f, x.gs, "web", "abc", forgeEnvPreview, previewScope(7), "https://p7.example.com")
	d.finish(context.Background(), githubapp.DeploymentSuccess, "ok")

	x.rt.stopGitLabEnvironment(context.Background(), x.f, x.gs, "web", previewScope(7))

	if got := x.fg.paths()[len(x.fg.paths())-1]; got != "POST /api/v4/projects/grp%2Fproj/environments/1/stop" {
		t.Fatalf("last call = %s", got)
	}
	if x.rows.rows[d.rec.ID].State != "inactive" {
		t.Errorf("row state = %s, want inactive", x.rows.rows[d.rec.ID].State)
	}
	if x.fg.calls[1].Body["name"] != "web/preview-7" || x.fg.calls[1].Body["tier"] != "development" {
		t.Errorf("environment body = %v", x.fg.calls[1].Body)
	}
}

func TestGitLabSupersedeNeedsNoCall(t *testing.T) {
	x := newGitLabDeployFixture(t, "api", gitlabDeploymentsOn)
	first := x.rt.beginOnGitLab(context.Background(), x.f, x.gs, "web", "a", forgeEnvProduction, forgeEnvProduction, "")
	first.finish(context.Background(), githubapp.DeploymentSuccess, "ok")
	second := x.rt.beginOnGitLab(context.Background(), x.f, x.gs, "web", "b", forgeEnvProduction, forgeEnvProduction, "")
	n := len(x.fg.calls)
	second.finish(context.Background(), githubapp.DeploymentSuccess, "ok")
	if len(x.fg.calls) != n+1 {
		t.Fatalf("finishing made %d calls, want one status update", len(x.fg.calls)-n)
	}
	if x.rows.rows[first.rec.ID].State != "inactive" {
		t.Errorf("older row state = %s", x.rows.rows[first.rec.ID].State)
	}
}
