package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

const gateSHA = "0123456789abcdef0123456789abcdef01234567"

// rewriteTransport sends every request to the fake GitLab, so the real
// https instance URL still drives repo URL resolution.
type rewriteTransport struct{ target *url.URL }

func (rt rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Scheme = rt.target.Scheme
	r2.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(r2)
}

// gitlabForgeFake adds the changed-file calls the forge interface needs.
type gitlabForgeFake struct{ *fakeGitLabAppClient }

func (gitlabForgeFake) CompareChangedFiles(context.Context, string, string, string, string, string) ([]string, error) {
	return nil, nil
}

func (gitlabForgeFake) MergeRequestChangedFiles(context.Context, string, string, string, int) ([]string, error) {
	return nil, nil
}

type fakeGitLabStatuses struct {
	mu    sync.Mutex
	paths []string
	raw   []string
	body  string
}

func (f *fakeGitLabStatuses) set(body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.body = body
}

func (f *fakeGitLabStatuses) queries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.raw...)
}

func (f *fakeGitLabStatuses) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...)
}

// setUpGitLabCIGate wires a router whose GitLab status reads hit a fake API.
func setUpGitLabCIGate(t *testing.T) (*Router, *store.DB, *fakeGitLabStatuses) {
	t.Helper()
	fake := &fakeGitLabStatuses{body: `[]`}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		fake.paths = append(fake.paths, r.URL.EscapedPath())
		fake.raw = append(fake.raw, r.URL.RawQuery)
		body := fake.body
		fake.mu.Unlock()
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	prev := newGitLabCIClient
	newGitLabCIClient = func() *gitlabapp.Client {
		return &gitlabapp.Client{HTTP: &http.Client{Transport: rewriteTransport{target: target}}}
	}
	t.Cleanup(func() { newGitLabCIClient = prev })
	t.Setenv("APP_CI_GATE_TIMEOUT", "300ms")
	t.Setenv("APP_CI_GATE_POLL_INTERVAL", "20ms")
	t.Setenv("APP_CI_GATE_NO_CHECKS_WAIT", "50ms")

	rt, db := newTestRouterWithGitLabAndGitSource(t, authorizedGitLabSecrets(t), gitlabForgeFake{&fakeGitLabAppClient{}}, newFakeGitSourceSecrets())
	seedGitLabAppConnection(t, db)
	seedApp(t, db, "web")
	return rt, db, fake
}

func gitlabGateSource(repoPath string, checks ...string) store.GitSource {
	return store.GitSource{RepoURL: "https://gitlab.example.com/" + repoPath + ".git", WaitForCI: true, CIRequiredChecks: checks}
}

func TestCIGateGitLab_RequiredChecksPassDeploys(t *testing.T) {
	rt, _, fake := setUpGitLabCIGate(t)
	fake.set(`[{"name":"test","status":"success"},{"name":"lint","status":"success"}]`)
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web", "test"), gateSHA); msg != "" {
		t.Fatalf("gate skipped a passing commit: %q", msg)
	}
}

func TestCIGateGitLab_FailedCheckBlocksAndRecordsSkip(t *testing.T) {
	rt, db, fake := setUpGitLabCIGate(t)
	fake.set(`[{"name":"test","status":"failed"}]`)
	msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web", "test"), gateSHA)
	if !strings.HasPrefix(msg, "not deployed: ") || !strings.Contains(msg, "test") {
		t.Fatalf("message = %q", msg)
	}
	attempts, err := db.ListDeployAttempts(t.Context(), "web")
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %v, err = %v", attempts, err)
	}
	if attempts[0].Status != store.DeployAttemptStatusFailed || !strings.Contains(attempts[0].Error, "skipped by CI gate") {
		t.Errorf("attempt = %+v", attempts[0])
	}
}

func TestCIGateGitLab_PendingTimesOut(t *testing.T) {
	rt, _, fake := setUpGitLabCIGate(t)
	fake.set(`[{"name":"test","status":"running"}]`)
	msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web", "test"), gateSHA)
	if !strings.HasPrefix(msg, "not deployed: ") {
		t.Fatalf("pending check let the deploy through: %q", msg)
	}
	if n := len(fake.seen()); n < 2 {
		t.Errorf("expected repeated polls while pending, got %d", n)
	}
}

func TestCIGateGitLab_SubgroupPathIsEncodedFromStoredURL(t *testing.T) {
	rt, _, fake := setUpGitLabCIGate(t)
	fake.set(`[{"name":"test","status":"success"}]`)
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("grp/sub/web"), gateSHA); msg != "" {
		t.Fatalf("message = %q", msg)
	}
	want := "/api/v4/projects/grp%2Fsub%2Fweb/repository/commits/" + gateSHA + "/statuses"
	seen := fake.seen()
	if len(seen) == 0 || seen[0] != want {
		t.Errorf("paths = %v, want first %q", seen, want)
	}
}

// The gate takes only the SHA, so a push body claiming success cannot reach
// it; the API's failed status still blocks and no body-supplied project is read.
func TestCIGateGitLab_PushBodyClaimIgnored(t *testing.T) {
	rt, _, fake := setUpGitLabCIGate(t)
	fake.set(`[{"name":"test","status":"failed"}]`)
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web", "test"), gateSHA); msg == "" {
		t.Fatal("gate trusted something other than the GitLab API")
	}
	for _, p := range fake.seen() {
		if strings.Contains(p, "evil") {
			t.Errorf("read a project taken from the push body: %s", p)
		}
	}
}

func TestCIGateGitLab_SkippedRequiredJobBlocks(t *testing.T) {
	rt, _, fake := setUpGitLabCIGate(t)
	fake.set(`[{"name":"lint","status":"failed"},{"name":"test","status":"skipped"}]`)
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web", "test"), gateSHA); msg == "" {
		t.Fatal("skipped required job let the deploy through")
	}
}

func TestCIGateGitLab_UnreadableProviderDoesNotDeploy(t *testing.T) {
	rt, db, _ := setUpGitLabCIGate(t)
	gs := store.GitSource{RepoURL: "ssh://host.example/org/web.git", WaitForCI: true}
	msg := rt.gateOnCI(t.Context(), "web", gs, gateSHA)
	if !strings.Contains(msg, "cannot read provider status") {
		t.Fatalf("message = %q", msg)
	}
	attempts, err := db.ListDeployAttempts(t.Context(), "web")
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %v, err = %v", attempts, err)
	}
	gs.WaitForCI = false
	if msg := rt.gateOnCI(t.Context(), "web", gs, gateSHA); msg != "" {
		t.Errorf("gate off must not block: %q", msg)
	}
}

func TestCIGateGitLab_BadSHASkips(t *testing.T) {
	rt, _, _ := setUpGitLabCIGate(t)
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web"), "abc123"); !strings.HasPrefix(msg, "not deployed: ") {
		t.Fatalf("message = %q", msg)
	}
}

func TestCIGateGitLab_ReadIsScopedToBranchAndForgedSuccessLoses(t *testing.T) {
	rt, _, fake := setUpGitLabCIGate(t)
	fake.set(`[{"name":"test","status":"success"},{"name":"test","status":"running"}]`)
	gs := gitlabGateSource("org/web", "test")
	gs.Branch = "release/1"
	if msg := rt.gateOnCI(t.Context(), "web", gs, gateSHA); msg == "" {
		t.Fatal("forged success beat the pending job")
	}
	if q := fake.queries(); len(q) == 0 || !strings.Contains(q[0], "ref=release%2F1") {
		t.Errorf("queries = %v", q)
	}
}

func TestCIGateGitLab_TooManyStatusesFailsNow(t *testing.T) {
	rt, _, fake := setUpGitLabCIGate(t)
	fake.set("[" + strings.Repeat(`{"name":"j","status":"success"},`, 99) + `{"name":"j","status":"success"}]`)
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web"), gateSHA); !strings.Contains(msg, "too many") {
		t.Fatalf("message = %q", msg)
	}
	// One read walks 10 pages; a retry loop would multiply that.
	if n := len(fake.seen()); n != 10 {
		t.Errorf("requests = %d, want one read of 10 pages", n)
	}
}

func TestGitlabProjectPathFromURL_RelativeRootInstance(t *testing.T) {
	got, ok := gitlabProjectPathFromURL("https://host.example/gitlab/grp/sub/web.git", "https://host.example/gitlab")
	if !ok || got != "grp/sub/web" {
		t.Errorf("got %q, %v", got, ok)
	}
	if _, ok := gitlabProjectPathFromURL("https://host.example/other/web.git", "https://host.example/gitlab"); ok {
		t.Error("repo outside the instance prefix must not resolve")
	}
}

func TestCIGateTrimCIChecks(t *testing.T) {
	if got := trimCIChecks([]string{" test ", "build"}); got[0] != "test" || got[1] != "build" {
		t.Errorf("got %v", got)
	}
}
