package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
)

type recordedCall struct {
	Method, Path, RawQuery, Auth string
	Body                         map[string]any
}

type forgeServer struct {
	mu    sync.Mutex
	calls []recordedCall
	// respond decides the reply per request; nil means 200 with "{}".
	respond func(r *http.Request) (status int, header map[string]string, body string)
}

func newForgeServer(t *testing.T, respond func(r *http.Request) (int, map[string]string, string)) (*forgeServer, *httptest.Server) {
	t.Helper()
	fs := &forgeServer{respond: respond}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		fs.mu.Lock()
		fs.calls = append(fs.calls, recordedCall{Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery, Auth: r.Header.Get("Authorization"), Body: body})
		fs.mu.Unlock()
		status, header, out := http.StatusOK, map[string]string(nil), "{}"
		if fs.respond != nil {
			status, header, out = fs.respond(r)
		}
		for k, v := range header {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(out))
	}))
	t.Cleanup(srv.Close)
	return fs, srv
}

func (f *forgeServer) last(t *testing.T) recordedCall {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		t.Fatal("forge received no calls")
	}
	return f.calls[len(f.calls)-1]
}

func forgeFor(kind, url string) *forge {
	switch kind {
	case forgeGitHub:
		return &forge{kind: kind, instanceURL: url, token: "tok", repo: "o/r", github: &githubapp.Client{HTTP: http.DefaultClient}}
	case forgeGitLab:
		return &forge{kind: kind, instanceURL: url, token: "tok", repo: "grp/proj", gitlab: &gitlabapp.Client{HTTP: http.DefaultClient}}
	case forgeGitea:
		return &forge{kind: kind, instanceURL: url, token: "tok", repo: "o/r", gitea: &giteaapp.Client{HTTP: http.DefaultClient}}
	}
	return &forge{kind: forgeBitbucket, token: "tok", repo: "ws/repo", bitbucket: &bitbucketapp.Client{HTTP: http.DefaultClient, APIBaseURL: url}}
}

func TestForgePostStatusPerProvider(t *testing.T) {
	tests := []struct {
		kind      string
		state     string
		wantPath  string
		wantState string
		stateKey  string
	}{
		{forgeGitHub, pipeline.ReportPending, "/api/v3/repos/o/r/statuses/abc", "pending", "state"},
		{forgeGitHub, pipeline.ReportSuccess, "/api/v3/repos/o/r/statuses/abc", "success", "state"},
		{forgeGitHub, pipeline.ReportFailure, "/api/v3/repos/o/r/statuses/abc", "failure", "state"},
		{forgeGitHub, pipeline.ReportError, "/api/v3/repos/o/r/statuses/abc", "error", "state"},
		{forgeGitLab, pipeline.ReportPending, "/api/v4/projects/grp%2Fproj/statuses/abc", "running", "state"},
		{forgeGitLab, pipeline.ReportFailure, "/api/v4/projects/grp%2Fproj/statuses/abc", "failed", "state"},
		{forgeGitLab, pipeline.ReportError, "/api/v4/projects/grp%2Fproj/statuses/abc", "canceled", "state"},
		{forgeGitea, pipeline.ReportSuccess, "/api/v1/repos/o/r/statuses/abc", "success", "state"},
		{forgeGitea, pipeline.ReportError, "/api/v1/repos/o/r/statuses/abc", "error", "state"},
		{forgeBitbucket, pipeline.ReportPending, "/repositories/ws/repo/commit/abc/statuses/build", "INPROGRESS", "state"},
		{forgeBitbucket, pipeline.ReportSuccess, "/repositories/ws/repo/commit/abc/statuses/build", "SUCCESSFUL", "state"},
		{forgeBitbucket, pipeline.ReportError, "/repositories/ws/repo/commit/abc/statuses/build", "STOPPED", "state"},
	}
	for _, tt := range tests {
		t.Run(tt.kind+"/"+tt.state, func(t *testing.T) {
			fs, srv := newForgeServer(t, nil)
			f := forgeFor(tt.kind, srv.URL)
			if err := f.postStatus(context.Background(), "abc", tt.state, "https://dash.test/run/1", "Run #1", "brand/pipeline/ci"); err != nil {
				t.Fatalf("postStatus: %v", err)
			}
			c := fs.last(t)
			if c.Method != http.MethodPost || !strings.HasSuffix(c.Path, strings.ReplaceAll(tt.wantPath, "%2F", "/")) {
				t.Fatalf("call = %+v, want POST %s", c, tt.wantPath)
			}
			if c.Auth != "Bearer tok" {
				t.Errorf("auth = %q", c.Auth)
			}
			if c.Body[tt.stateKey] != tt.wantState {
				t.Errorf("state = %v, want %s (body %v)", c.Body[tt.stateKey], tt.wantState, c.Body)
			}
		})
	}
}

func TestForgePostStatusIncludesTargetURLAndContext(t *testing.T) {
	fs, srv := newForgeServer(t, nil)
	f := forgeFor(forgeGitHub, srv.URL)
	if err := f.postStatus(context.Background(), "abc", pipeline.ReportSuccess, "https://dash.test/apps/web/pipelines/runs/r1", "Run #4 succeeded", "brand/pipeline/ci"); err != nil {
		t.Fatal(err)
	}
	b := fs.last(t).Body
	if b["target_url"] != "https://dash.test/apps/web/pipelines/runs/r1" || b["context"] != "brand/pipeline/ci" || b["description"] != "Run #4 succeeded" {
		t.Fatalf("body = %v", b)
	}
}

func TestForgePostStatusFailures(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		header  map[string]string
		body    string
		wantMsg string
	}{
		{"server error", 500, nil, "boom", "500"},
		{"unauthorized", 401, nil, `{"message":"Bad credentials"}`, "401"},
		{"github 429", 429, map[string]string{"Retry-After": "30"}, "slow down", "rate limited by the git provider (retry after 30s)"},
		{"github secondary 403", 403, map[string]string{"X-RateLimit-Remaining": "0"}, "forbidden", "rate limited by the git provider"},
		{"403 body says rate limit", 403, nil, `{"message":"API rate limit exceeded"}`, "rate limited by the git provider"},
		{"plain 403 is not a rate limit", 403, nil, `{"message":"Resource not accessible"}`, "403"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, srv := newForgeServer(t, func(*http.Request) (int, map[string]string, string) { return tt.status, tt.header, tt.body })
			err := forgeFor(forgeGitHub, srv.URL).postStatus(context.Background(), "abc", pipeline.ReportSuccess, "", "d", "c")
			if err == nil {
				t.Fatal("postStatus succeeded against a failing forge")
			}
			if msg := describeForgeError(err); !strings.Contains(msg, tt.wantMsg) {
				t.Fatalf("describeForgeError = %q, want it to contain %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestForgePostStatusTruncatesLongDescriptionAndBitbucketKey(t *testing.T) {
	fs, srv := newForgeServer(t, nil)
	long := strings.Repeat("x", 500)
	if err := forgeFor(forgeGitHub, srv.URL).postStatus(context.Background(), "abc", pipeline.ReportPending, "", long, "c"); err != nil {
		t.Fatal(err)
	}
	if d, _ := fs.last(t).Body["description"].(string); len(d) != githubStatusDescriptionMax {
		t.Errorf("github description length = %d, want %d", len(d), githubStatusDescriptionMax)
	}
	if err := forgeFor(forgeBitbucket, srv.URL).postStatus(context.Background(), "abc", pipeline.ReportPending, "", "d", strings.Repeat("k", 80)); err != nil {
		t.Fatal(err)
	}
	if k, _ := fs.last(t).Body["key"].(string); len(k) != bitbucketKeyMax {
		t.Errorf("bitbucket key length = %d, want %d", len(k), bitbucketKeyMax)
	}
}

func TestForgeChangedFiles(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		q        changeQuery
		reply    string
		wantPath string
		want     []string
	}{
		{"github compare", forgeGitHub, changeQuery{Base: "b1", Head: "h1"}, `{"files":[{"filename":"src/a.go"},{"filename":"new.go","previous_filename":"old.go"}]}`, "/api/v3/repos/o/r/compare/b1...h1", []string{"src/a.go", "new.go", "old.go"}},
		{"github pr", forgeGitHub, changeQuery{PR: 7}, `[{"filename":"web/x.ts"}]`, "/api/v3/repos/o/r/pulls/7/files", []string{"web/x.ts"}},
		{"gitlab compare", forgeGitLab, changeQuery{Base: "b1", Head: "h1"}, `{"diffs":[{"old_path":"a","new_path":"a"},{"old_path":"x","new_path":"y"}]}`, "/api/v4/projects/grp/proj/repository/compare", []string{"a", "y", "x"}},
		{"gitlab mr", forgeGitLab, changeQuery{PR: 3}, `{"changes":[{"old_path":"m","new_path":"m"}]}`, "/api/v4/projects/grp/proj/merge_requests/3/changes", []string{"m"}},
		{"gitea compare", forgeGitea, changeQuery{Base: "b1", Head: "h1"}, `{"commits":[{"files":[{"filename":"a"}]},{"files":[{"filename":"b"}]}]}`, "/api/v1/repos/o/r/compare/b1...h1", []string{"a", "b"}},
		{"gitea pr", forgeGitea, changeQuery{PR: 2}, `[{"filename":"g"}]`, "/api/v1/repos/o/r/pulls/2/files", []string{"g"}},
		{"bitbucket diffstat", forgeBitbucket, changeQuery{Base: "b1", Head: "h1"}, `{"values":[{"new":{"path":"n"},"old":{"path":"o"}},{"new":null,"old":{"path":"gone"}}]}`, "/repositories/ws/repo/diffstat/", []string{"n", "o", "gone"}},
		{"bitbucket pr", forgeBitbucket, changeQuery{PR: 9}, `{"values":[{"new":{"path":"p"},"old":{"path":"p"}}]}`, "/repositories/ws/repo/pullrequests/9/diffstat", []string{"p"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs, srv := newForgeServer(t, func(*http.Request) (int, map[string]string, string) { return 200, nil, tt.reply })
			got, err := forgeFor(tt.kind, srv.URL).changedFiles(context.Background(), tt.q)
			if err != nil {
				t.Fatal(err)
			}
			if c := fs.last(t); c.Method != http.MethodGet || !strings.Contains(c.Path, tt.wantPath) {
				t.Fatalf("call = %+v, want GET containing %s", c, tt.wantPath)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("files = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestForgeChangedFilesNeedsBase(t *testing.T) {
	f := forgeFor(forgeGitHub, "http://unused")
	for _, base := range []string{"", "0000000000000000000000000000000000000000"} {
		if _, err := f.changedFiles(context.Background(), changeQuery{Base: base, Head: "h1"}); !errors.Is(err, errNoChangeBase) {
			t.Errorf("base %q: err = %v, want errNoChangeBase", base, err)
		}
	}
}

func TestForgeChangedFilesRateLimited(t *testing.T) {
	_, srv := newForgeServer(t, func(*http.Request) (int, map[string]string, string) {
		return 429, map[string]string{"Retry-After": "12"}, "slow"
	})
	_, err := forgeFor(forgeGitHub, srv.URL).changedFiles(context.Background(), changeQuery{PR: 1})
	if err == nil || !strings.Contains(describeForgeError(err), "retry after 12s") {
		t.Fatalf("err = %v", err)
	}
}

func TestGitStatusEnabledKillSwitch(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{{"", true}, {"true", true}, {"1", true}, {"false", false}, {"0", false}}
	for _, tt := range tests {
		t.Setenv(EnvGitStatusEnabled, tt.val)
		if got := gitStatusEnabled(); got != tt.want {
			t.Errorf("%s=%q: gitStatusEnabled = %v, want %v", EnvGitStatusEnabled, tt.val, got, tt.want)
		}
	}
}

func TestForgeCommitURL(t *testing.T) {
	tests := []struct{ kind, repo, want string }{
		{forgeGitHub, "https://github.com/o/r.git", "https://github.com/o/r/commit/abc"},
		{forgeGitea, "https://git.test/o/r", "https://git.test/o/r/commit/abc"},
		{forgeGitLab, "https://gl.test/g/p.git", "https://gl.test/g/p/-/commit/abc"},
		{forgeBitbucket, "https://bitbucket.org/w/r", "https://bitbucket.org/w/r/commits/abc"},
	}
	for _, tt := range tests {
		if got := forgeCommitURL(tt.kind, tt.repo, "abc"); got != tt.want {
			t.Errorf("forgeCommitURL(%s) = %s, want %s", tt.kind, got, tt.want)
		}
	}
}

func TestStatusReporterGates(t *testing.T) {
	rt, _ := newConnectedGitSourceRouter(t, `{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"dockerfile"}`)
	rep := rt.PipelineStatusReporter()
	req := pipeline.ReportRequest{App: "web", SHA: "abc", State: pipeline.ReportPending}

	if _, err := rep.ReportRun(context.Background(), req); !errors.Is(err, pipeline.ErrNoReportTarget) {
		t.Fatalf("no github connection: err = %v, want ErrNoReportTarget", err)
	}
	if _, err := rep.ReportRun(context.Background(), pipeline.ReportRequest{App: "missing"}); !errors.Is(err, pipeline.ErrNoReportTarget) {
		t.Fatalf("app without a git source: err = %v", err)
	}
	t.Setenv(EnvGitStatusEnabled, "false")
	if _, err := rep.ReportRun(context.Background(), req); !errors.Is(err, pipeline.ErrNoReportTarget) {
		t.Fatalf("kill switch: err = %v", err)
	}
}

func TestDeploySettingsEndpointAndPathFilteredPush(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"dockerfile"}`)
	if !created.ReportStatus || len(created.DeployPaths) != 0 {
		t.Fatalf("defaults = %+v, want report_status on and no paths", created)
	}

	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source/deploy-settings", body))
		return rec
	}
	if rec := put(`{"deploy_paths":["src/**"],"deploy_paths_ignore":["**/*_test.go"],"report_status":false}`); rec.Code != http.StatusOK {
		t.Fatalf("set: %d %s", rec.Code, rec.Body.String())
	}
	if rec := put(`{"deploy_paths":["src/[a"]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad glob: status = %d, want 400", rec.Code)
	}
	if rec := put(`{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty body: status = %d, want 400", rec.Code)
	}

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb

	push := func(files ...string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{
			"ref": "refs/heads/main", "after": "sha1", "before": "sha0",
			"commits": []map[string]any{{"id": "sha1", "modified": files}},
		})
		return postGitWebhook(t, rt, "X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body), body, nil)
	}
	rec := push("README.md", "src/x_test.go")
	requireStatusOK(t, rec)
	if !strings.Contains(rec.Body.String(), "skipped: every changed path matched paths_ignore") && !strings.Contains(rec.Body.String(), "skipped: no changed path matched paths") {
		t.Fatalf("body = %q, want a skip reason", rec.Body.String())
	}
	if fb.calls != 0 || len(fetchCalls) != 0 {
		t.Fatalf("a filtered push deployed: builder calls %d, fetch calls %d", fb.calls, len(fetchCalls))
	}

	requireStatusOK(t, push("README.md", "src/app.go"))
	if fb.calls != 1 {
		t.Fatalf("matching push: builder called %d times, want 1", fb.calls)
	}
}

type fakeForgeDeployments struct {
	mu   sync.Mutex
	rows map[string]store.ForgeDeployment
}

func (f *fakeForgeDeployments) CreateForgeDeployment(_ context.Context, d store.ForgeDeployment) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[d.ID] = d
	return nil
}

func (f *fakeForgeDeployments) SetForgeDeploymentState(_ context.Context, id, state, warning string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	d := f.rows[id]
	d.State, d.Warning, d.UpdatedAt = state, warning, at
	f.rows[id] = d
	return nil
}

func (f *fakeForgeDeployments) ListForgeDeploymentsByState(_ context.Context, app, env, state, exclude string) ([]store.ForgeDeployment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.ForgeDeployment
	for _, d := range f.rows {
		if d.AppName == app && d.Environment == env && d.State == state && d.ID != exclude {
			out = append(out, d)
		}
	}
	return out, nil
}

func newDeploymentFixture(t *testing.T, respond func(*http.Request) (int, map[string]string, string)) (*forgeServer, *Router, *fakeForgeDeployments, *forge) {
	t.Helper()
	fs, srv := newForgeServer(t, respond)
	rows := &fakeForgeDeployments{rows: map[string]store.ForgeDeployment{}}
	rt := &Router{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), forgeDeployments: rows}
	return fs, rt, rows, forgeFor(forgeGitHub, srv.URL)
}

func newTestDeployment(rt *Router, f *forge, id string, externalID int64) *forgeDeployment {
	rec := store.ForgeDeployment{ID: id, AppName: "web", Environment: forgeEnvProduction, Provider: forgeGitHub, ExternalID: externalID, CommitSHA: "sha", State: "in_progress"}
	_ = rt.forgeDeployments.CreateForgeDeployment(context.Background(), rec)
	return &forgeDeployment{rt: rt, f: f, rec: rec, logURL: "https://dash.test/apps/web/deployments"}
}

func TestForgeDeploymentSupersedesOlderSuccess(t *testing.T) {
	fs, rt, rows, f := newDeploymentFixture(t, nil)
	old := newTestDeployment(rt, f, "old", 11)
	old.finish(context.Background(), githubapp.DeploymentSuccess, "ok")
	next := newTestDeployment(rt, f, "new", 22)
	next.finish(context.Background(), githubapp.DeploymentSuccess, "ok")

	if rows.rows["old"].State != "inactive" || rows.rows["new"].State != "success" {
		t.Fatalf("states old=%s new=%s, want inactive/success", rows.rows["old"].State, rows.rows["new"].State)
	}
	var sawInactive bool
	for _, c := range fs.calls {
		if strings.HasSuffix(c.Path, "/deployments/11/statuses") && c.Body["state"] == "inactive" {
			sawInactive = true
		}
	}
	if !sawInactive {
		t.Fatalf("no inactive status posted for the superseded deployment: %+v", fs.calls)
	}
}

func TestForgeDeploymentFailureKeepsPreviousActive(t *testing.T) {
	_, rt, rows, f := newDeploymentFixture(t, nil)
	old := newTestDeployment(rt, f, "old", 11)
	old.finish(context.Background(), githubapp.DeploymentSuccess, "ok")
	bad := newTestDeployment(rt, f, "bad", 33)
	bad.finish(context.Background(), githubapp.DeploymentFailure, "deploy failed")
	if rows.rows["old"].State != "success" || rows.rows["bad"].State != "failure" {
		t.Fatalf("states old=%s bad=%s, want success/failure", rows.rows["old"].State, rows.rows["bad"].State)
	}
}

func TestForgeDeploymentPostFailureRecordsWarning(t *testing.T) {
	for name, tt := range map[string]struct {
		status int
		header map[string]string
		want   string
	}{
		"server error": {500, nil, "500"},
		"rate limited": {429, map[string]string{"Retry-After": "5"}, "rate limited"},
	} {
		t.Run(name, func(t *testing.T) {
			_, rt, rows, f := newDeploymentFixture(t, func(*http.Request) (int, map[string]string, string) { return tt.status, tt.header, "no" })
			d := newTestDeployment(rt, f, "d1", 1)
			d.finish(context.Background(), githubapp.DeploymentSuccess, "ok")
			got := rows.rows["d1"]
			if got.State != "success" || !strings.Contains(got.Warning, tt.want) {
				t.Fatalf("row = %+v, want success with a warning containing %q", got, tt.want)
			}
		})
	}
}

func TestNilForgeDeploymentIsNoop(_ *testing.T) {
	var d *forgeDeployment
	d.finish(context.Background(), githubapp.DeploymentSuccess, "ok")
}

func TestDeploymentStateFor(t *testing.T) {
	tests := []struct {
		status int
		msg    string
		want   githubapp.DeploymentState
	}{
		{200, "deploy triggered: web:sha1\n", githubapp.DeploymentSuccess},
		{207, "services deploy triggered (1 failed)\n", githubapp.DeploymentSuccess},
		{500, "deploy failed", githubapp.DeploymentFailure},
		{200, "not deployed: superseded by a newer commit\n", githubapp.DeploymentInactive},
	}
	for _, tt := range tests {
		if got := deploymentStateFor(tt.status, tt.msg); got != tt.want {
			t.Errorf("deploymentStateFor(%d, %q) = %s, want %s", tt.status, tt.msg, got, tt.want)
		}
	}
}

func TestGitHubDeploymentCreateAndStatusRequests(t *testing.T) {
	fs, srv := newForgeServer(t, func(r *http.Request) (int, map[string]string, string) {
		if strings.HasSuffix(r.URL.Path, "/deployments") {
			return 201, nil, `{"id":4242}`
		}
		return 201, nil, `{}`
	})
	c := &githubapp.Client{HTTP: http.DefaultClient}
	id, err := c.CreateDeployment(context.Background(), srv.URL, "tok", "o", "r", "sha1", "preview", "Deploy web", false)
	if err != nil || id != 4242 {
		t.Fatalf("CreateDeployment = %d, %v", id, err)
	}
	b := fs.last(t).Body
	if b["environment"] != "preview" || b["ref"] != "sha1" || b["production_environment"] != false || b["transient_environment"] != true {
		t.Fatalf("deployment body = %v", b)
	}
	if rc, ok := b["required_contexts"].([]any); !ok || len(rc) != 0 {
		t.Fatalf("required_contexts = %v, want an empty list so unrelated checks never block", b["required_contexts"])
	}
	if err := c.CreateDeploymentStatus(context.Background(), srv.URL, "tok", "o", "r", 4242, githubapp.DeploymentInProgress, "", "https://dash.test/x", "Deploying"); err != nil {
		t.Fatal(err)
	}
	if call := fs.last(t); !strings.HasSuffix(call.Path, "/deployments/4242/statuses") || call.Body["state"] != "in_progress" || call.Body["log_url"] != "https://dash.test/x" {
		t.Fatalf("status call = %+v", call)
	}
}
