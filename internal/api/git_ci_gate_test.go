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
	body  string
}

func (f *fakeGitLabStatuses) set(body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.body = body
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
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web", "test"), "abc123"); msg != "" {
		t.Fatalf("gate skipped a passing commit: %q", msg)
	}
}

func TestCIGateGitLab_FailedCheckBlocksAndRecordsSkip(t *testing.T) {
	rt, db, fake := setUpGitLabCIGate(t)
	fake.set(`[{"name":"test","status":"failed"}]`)
	msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web", "test"), "abc123")
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
	msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web", "test"), "abc123")
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
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("grp/sub/web"), "abc123"); msg != "" {
		t.Fatalf("message = %q", msg)
	}
	want := "/api/v4/projects/grp%2Fsub%2Fweb/repository/commits/abc123/statuses"
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
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web", "test"), "abc123"); msg == "" {
		t.Fatal("gate trusted something other than the GitLab API")
	}
	for _, p := range fake.seen() {
		if strings.Contains(p, "evil") {
			t.Errorf("read a project taken from the push body: %s", p)
		}
	}
}
