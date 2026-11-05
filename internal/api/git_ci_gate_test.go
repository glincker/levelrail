package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cigate"
	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
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
	// block, when set, holds every status read until it closes or the caller hangs up.
	block chan struct{}
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
		body, block := fake.body, fake.block
		fake.mu.Unlock()
		if block != nil {
			select {
			case <-block:
			case <-r.Context().Done():
				return
			}
		}
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
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web", "test"), gateSHA, ""); msg != "" {
		t.Fatalf("gate skipped a passing commit: %q", msg)
	}
}

func TestCIGateGitLab_FailedCheckBlocksAndRecordsSkip(t *testing.T) {
	rt, db, fake := setUpGitLabCIGate(t)
	fake.set(`[{"name":"test","status":"failed"}]`)
	msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web", "test"), gateSHA, "")
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
	msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web", "test"), gateSHA, "")
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
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("grp/sub/web"), gateSHA, ""); msg != "" {
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
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web", "test"), gateSHA, ""); msg == "" {
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
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web", "test"), gateSHA, ""); msg == "" {
		t.Fatal("skipped required job let the deploy through")
	}
}

func TestCIGateGitLab_UnreadableProviderDoesNotDeploy(t *testing.T) {
	rt, db, _ := setUpGitLabCIGate(t)
	gs := store.GitSource{RepoURL: "ssh://host.example/org/web.git", WaitForCI: true}
	msg := rt.gateOnCI(t.Context(), "web", gs, gateSHA, "")
	if !strings.Contains(msg, "cannot read provider status") {
		t.Fatalf("message = %q", msg)
	}
	attempts, err := db.ListDeployAttempts(t.Context(), "web")
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %v, err = %v", attempts, err)
	}
	gs.WaitForCI = false
	if msg := rt.gateOnCI(t.Context(), "web", gs, gateSHA, ""); msg != "" {
		t.Errorf("gate off must not block: %q", msg)
	}
}

func TestCIGateGitLab_BadSHASkips(t *testing.T) {
	rt, _, _ := setUpGitLabCIGate(t)
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web"), "abc123", ""); !strings.HasPrefix(msg, "not deployed: ") {
		t.Fatalf("message = %q", msg)
	}
}

func TestCIGateGitLab_ReadIsScopedToBranchAndForgedSuccessLoses(t *testing.T) {
	rt, _, fake := setUpGitLabCIGate(t)
	fake.set(`[{"name":"test","status":"success"},{"name":"test","status":"running"}]`)
	gs := gitlabGateSource("org/web", "test")
	gs.Branch = "release/1"
	if msg := rt.gateOnCI(t.Context(), "web", gs, gateSHA, ""); msg == "" {
		t.Fatal("forged success beat the pending job")
	}
	if q := fake.queries(); len(q) == 0 || !strings.Contains(q[0], "ref=release%2F1") {
		t.Errorf("queries = %v", q)
	}
}

func TestCIGateGitLab_TooManyStatusesFailsNow(t *testing.T) {
	rt, _, fake := setUpGitLabCIGate(t)
	fake.set("[" + strings.Repeat(`{"name":"j","status":"success"},`, 99) + `{"name":"j","status":"success"}]`)
	if msg := rt.gateOnCI(t.Context(), "web", gitlabGateSource("org/web"), gateSHA, ""); !strings.Contains(msg, "too many") {
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

const gateSHA2 = "fedcba9876543210fedcba9876543210fedcba98"

type gateFetchLog struct {
	mu   sync.Mutex
	shas []string
}

func (l *gateFetchLog) seen() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.shas...)
}

// gateWebhookRig records every checkout the deploy attempts; the fetch fails
// on purpose, since the test only needs to know whether a deploy started.
func gateWebhookRig(t *testing.T) (*Router, *store.DB, *fakeGitLabStatuses, *gateFetchLog) {
	t.Helper()
	rt, db, fake := setUpGitLabCIGate(t)
	t.Setenv("APP_CI_GATE_TIMEOUT", "10s")
	log := &gateFetchLog{}
	rt.builder = &fakeBuilder{tag: "web:x"}
	rt.gitSourceFetch = func(_ context.Context, _, sha, _ string) (string, func(), error) {
		log.mu.Lock()
		log.shas = append(log.shas, sha)
		log.mu.Unlock()
		return "", nil, fmt.Errorf("fetch stubbed")
	}
	return rt, db, fake, log
}

func gatedPushSource() store.GitSource {
	gs := gitlabGateSource("org/web", "test")
	gs.ServiceName, gs.Branch = "web", "main"
	return gs
}

func gitLabPushBody(ref, after, checkout string) []byte {
	return []byte(fmt.Sprintf(`{"ref":%q,"after":%q,"before":%q,"checkout_sha":%q}`, ref, after, zeroGitSHA, checkout))
}

func waitGateBackground(t *testing.T, rt *Router) {
	t.Helper()
	done := make(chan struct{})
	go func() { rt.previewDeploys.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("background gate did not finish")
	}
}

func TestGitWebhookCIGate_AcknowledgesWhileGateWaits(t *testing.T) {
	rt, _, fake, log := gateWebhookRig(t)
	fake.mu.Lock()
	fake.block = make(chan struct{})
	block := fake.block
	fake.mu.Unlock()
	fake.set(`[{"name":"test","status":"success"}]`)

	start := time.Now()
	status, msg := rt.processGitPushWebhookPayload(t.Context(), "web", gatedPushSource(), gitLabPushBody("refs/heads/main", gateSHA, ""), http.Header{})
	if status != http.StatusAccepted || !strings.Contains(msg, "waiting for CI") {
		t.Fatalf("status = %d, msg = %q", status, msg)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("webhook was held up by the gate")
	}
	if got := log.seen(); len(got) != 0 {
		t.Fatalf("deployed before CI passed: %v", got)
	}
	close(block)
	waitGateBackground(t, rt)
	if got := log.seen(); len(got) != 1 || got[0] != gateSHA {
		t.Fatalf("checkouts = %v, want [%s]", got, gateSHA)
	}
	rt.ciGates().mu.Lock()
	left := len(rt.ciGates().slots)
	rt.ciGates().mu.Unlock()
	if left != 0 {
		t.Errorf("gate slots left behind: %d", left)
	}
}

func TestGitWebhookCIGate_NewerPushSupersedesOlderWait(t *testing.T) {
	rt, db, fake, log := gateWebhookRig(t)
	fake.mu.Lock()
	fake.block = make(chan struct{})
	block := fake.block
	fake.mu.Unlock()
	fake.set(`[{"name":"test","status":"success"}]`)
	gs := gatedPushSource()

	for _, sha := range []string{gateSHA, gateSHA2} {
		if status, _ := rt.processGitPushWebhookPayload(t.Context(), "web", gs, gitLabPushBody("refs/heads/main", sha, ""), http.Header{}); status != http.StatusAccepted {
			t.Fatalf("status = %d", status)
		}
	}
	// The older wait ends on its own, before anything unblocks the status read.
	deadline := time.Now().Add(5 * time.Second)
	for {
		attempts, err := db.ListDeployAttempts(t.Context(), "web")
		if err == nil && len(attempts) == 1 {
			if !strings.Contains(attempts[0].Error, "superseded") {
				t.Fatalf("attempt = %+v", attempts[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("superseded wait never recorded: %v %v", attempts, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	close(block)
	waitGateBackground(t, rt)
	if got := log.seen(); len(got) != 1 || got[0] != gateSHA2 {
		t.Fatalf("checkouts = %v, want only the newer %s", got, gateSHA2)
	}
}

func TestGitWebhookCIGate_FailedGateDoesNotDeploy(t *testing.T) {
	rt, db, fake, log := gateWebhookRig(t)
	fake.set(`[{"name":"test","status":"failed"}]`)
	if status, _ := rt.processGitPushWebhookPayload(t.Context(), "web", gatedPushSource(), gitLabPushBody("refs/heads/main", gateSHA, ""), http.Header{}); status != http.StatusAccepted {
		t.Fatalf("status = %d", status)
	}
	waitGateBackground(t, rt)
	if got := log.seen(); len(got) != 0 {
		t.Fatalf("failed gate deployed: %v", got)
	}
	attempts, err := db.ListDeployAttempts(t.Context(), "web")
	if err != nil || len(attempts) != 1 || !strings.Contains(attempts[0].Error, "skipped by CI gate") {
		t.Fatalf("attempts = %+v, err = %v", attempts, err)
	}
}

func TestGitWebhookCIGate_TagReadsCommitStatusesScopedToTag(t *testing.T) {
	rt, _, fake, log := gateWebhookRig(t)
	fake.set(`[{"name":"test","status":"success"}]`)
	gs := gatedPushSource()
	gs.TriggerMode = spec.TriggerModeRelease
	const tagObject = "1111111111111111111111111111111111111111"
	if status, _ := rt.processGitPushWebhookPayload(t.Context(), "web", gs, gitLabPushBody("refs/tags/v1.2.0", tagObject, gateSHA), http.Header{}); status != http.StatusAccepted {
		t.Fatalf("status = %d", status)
	}
	waitGateBackground(t, rt)
	seen, queries := fake.seen(), fake.queries()
	if len(seen) == 0 || !strings.Contains(seen[0], gateSHA) || strings.Contains(seen[0], tagObject) {
		t.Fatalf("paths = %v, want the commit %s", seen, gateSHA)
	}
	if len(queries) == 0 || !strings.Contains(queries[0], "ref=v1.2.0") {
		t.Fatalf("queries = %v, want ref=v1.2.0", queries)
	}
	if got := log.seen(); len(got) != 1 || got[0] != gateSHA {
		t.Fatalf("checkouts = %v, want the gated commit %s, not the tag object", got, gateSHA)
	}
}

func TestGitWebhookCIGate_CancelledRequestStillDeploysAfterPass(t *testing.T) {
	rt, _, fake, log := gateWebhookRig(t)
	fake.mu.Lock()
	fake.block = make(chan struct{})
	block := fake.block
	fake.mu.Unlock()
	fake.set(`[{"name":"test","status":"success"}]`)
	ctx, cancel := context.WithCancel(t.Context())
	if status, _ := rt.processGitPushWebhookPayload(ctx, "web", gatedPushSource(), gitLabPushBody("refs/heads/main", gateSHA, ""), http.Header{}); status != http.StatusAccepted {
		t.Fatalf("status = %d", status)
	}
	cancel()
	close(block)
	waitGateBackground(t, rt)
	if got := log.seen(); len(got) != 1 || got[0] != gateSHA {
		t.Fatalf("checkouts = %v, want the deploy to run after the request hung up", got)
	}
}

func TestGitWebhookCIGate_CancelledRequestStillRecordsFailedGate(t *testing.T) {
	rt, db, fake, log := gateWebhookRig(t)
	fake.mu.Lock()
	fake.block = make(chan struct{})
	block := fake.block
	fake.mu.Unlock()
	fake.set(`[{"name":"test","status":"failed"}]`)
	ctx, cancel := context.WithCancel(t.Context())
	if status, _ := rt.processGitPushWebhookPayload(ctx, "web", gatedPushSource(), gitLabPushBody("refs/heads/main", gateSHA, ""), http.Header{}); status != http.StatusAccepted {
		t.Fatalf("status = %d", status)
	}
	cancel()
	close(block)
	waitGateBackground(t, rt)
	attempts, err := db.ListDeployAttempts(t.Context(), "web")
	if err != nil || len(attempts) != 1 || !strings.Contains(attempts[0].Error, "skipped by CI gate") {
		t.Fatalf("attempts = %+v, err = %v", attempts, err)
	}
	if got := log.seen(); len(got) != 0 {
		t.Fatalf("failed gate deployed: %v", got)
	}
}

func TestGitWebhookCIGate_BranchDeletionIsNotGatedOrDeployed(t *testing.T) {
	rt, _, _, log := gateWebhookRig(t)
	status, msg := rt.processGitPushWebhookPayload(t.Context(), "web", gatedPushSource(), gitLabPushBody("refs/heads/main", zeroGitSHA, ""), http.Header{})
	if status != http.StatusOK || !strings.Contains(msg, "deleted") {
		t.Fatalf("status = %d, msg = %q", status, msg)
	}
	waitGateBackground(t, rt)
	if got := log.seen(); len(got) != 0 {
		t.Fatalf("deletion deployed: %v", got)
	}
}

func TestCIGateRef(t *testing.T) {
	release := store.GitSource{Branch: "main", TriggerMode: spec.TriggerModeRelease}
	push := store.GitSource{Branch: "main"}
	meta := withPushCommitMeta(t.Context(), webhook.PushEvent{Ref: "refs/heads/feat"})
	cases := []struct {
		name string
		ctx  context.Context
		gs   store.GitSource
		tag  string
		want string
	}{
		{"tag wins", t.Context(), release, "v1", "v1"},
		{"tag beats metadata branch", meta, push, "v2", "v2"},
		{"push metadata branch", meta, push, "", "feat"},
		{"release without tag reads every ref", t.Context(), release, "", ""},
		{"configured branch fallback", t.Context(), push, "", "main"},
	}
	for _, c := range cases {
		if got := ciGateRef(c.ctx, c.gs, c.tag); got != c.want {
			t.Errorf("%s: ref = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestLazyCIReaderRetriesBuildFailures(t *testing.T) {
	_, _, fake := setUpGitLabCIGate(t)
	fake.set(`[{"name":"test","status":"success"}]`)
	builds := 0
	l := &lazyCIReader{build: func(context.Context) (cigate.Provider, error) {
		if builds++; builds < 3 {
			return nil, fmt.Errorf("token mint failed")
		}
		return &cigate.GitLab{Client: newGitLabCIClient(), InstanceURL: "https://gitlab.example.com", ProjectPath: "org/web"}, nil
	}}
	res, err := cigate.Wait(t.Context(), l, gateSHA, cigate.Policy{Required: []string{"test"}}, cigate.Config{Timeout: 5 * time.Second, PollInterval: 5 * time.Millisecond, NoChecksWait: time.Millisecond})
	if err != nil || res.Outcome != cigate.OutcomePass || builds != 3 {
		t.Fatalf("res = %+v, err = %v, builds = %d", res, err, builds)
	}
}

func TestSaveCIGateTransientResolveFailureIsNot422(t *testing.T) {
	rt, db, _ := setUpGitLabCIGate(t)
	if err := db.SaveGitSource(t.Context(), store.GitSource{ServiceName: "web", RepoURL: "https://unknown.example/org/web.git", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if rt.saveCIGate(rec, httptest.NewRequest(http.MethodPatch, "/", nil), "web", gitDeploySettings{WaitForCI: true}) || rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 for an unresolved https source", rec.Code)
	}
}

func TestSaveCIGateRefusesSourcesWithoutAReader(t *testing.T) {
	rt, db, _ := setUpGitLabCIGate(t)
	ctx := t.Context()
	for repo, want := range map[string]bool{
		"https://gitlab.example.com/org/web.git": true,
		"ssh://host.example/org/web.git":         false,
	} {
		if err := db.SaveGitSource(ctx, store.GitSource{ServiceName: "web", RepoURL: repo, Branch: "main", BuildType: "dockerfile"}); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		ok := rt.saveCIGate(rec, httptest.NewRequest(http.MethodPatch, "/", nil).WithContext(ctx), "web", gitDeploySettings{WaitForCI: true})
		if ok != want {
			t.Errorf("%s: saveCIGate = %v (status %d), want %v", repo, ok, rec.Code, want)
		}
		if !want && rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d", repo, rec.Code)
		}
	}
}

// gateWebhookRigSafety wires the router like production: a deploy safety store
// makes admitGitPush queue every push, which is the path that once skipped the gate.
func gateWebhookRigSafety(t *testing.T) (*Router, *store.DB, *fakeGitLabStatuses, *gateFetchLog) {
	t.Helper()
	rt, db, fake, log := gateWebhookRig(t)
	rt.deploySafety = db
	gs := gatedPushSource()
	if err := db.SaveGitSource(t.Context(), store.GitSource{ServiceName: "web", RepoURL: gs.RepoURL, Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	return rt, db, fake, log
}

func anyCheckout(log *gateFetchLog, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if len(log.seen()) > 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestGitWebhookCIGate_RedGateQueuesNothingWithDeploySafety(t *testing.T) {
	rt, db, fake, log := gateWebhookRigSafety(t)
	fake.set(`[{"name":"test","status":"failed"}]`)
	rt.processGitPushWebhookPayload(t.Context(), "web", gatedPushSource(), gitLabPushBody("refs/heads/main", gateSHA, ""), http.Header{})
	waitGateBackground(t, rt)
	if anyCheckout(log, 500*time.Millisecond) {
		t.Fatalf("red commit was deployed: %v", log.seen())
	}
	attempts, err := db.ListDeployAttempts(t.Context(), "web")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range attempts {
		if !strings.Contains(a.Error, "skipped by CI gate") {
			t.Errorf("attempt %s status %s error %q is not a gate skip", a.ID, a.Status, a.Error)
		}
	}
}

func TestGitWebhookCIGate_GreenGateQueuesExactlyOneWithDeploySafety(t *testing.T) {
	rt, db, fake, log := gateWebhookRigSafety(t)
	fake.set(`[{"name":"test","status":"success"}]`)
	rt.processGitPushWebhookPayload(t.Context(), "web", gatedPushSource(), gitLabPushBody("refs/heads/main", gateSHA, ""), http.Header{})
	waitGateBackground(t, rt)
	if !anyCheckout(log, 5*time.Second) {
		t.Fatal("green commit was never deployed")
	}
	attempts, err := db.ListDeployAttempts(t.Context(), "web")
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %+v, err = %v", attempts, err)
	}
}

func gitLabPushBodyBefore(ref, after, before string) []byte {
	return []byte(fmt.Sprintf(`{"ref":%q,"after":%q,"before":%q}`, ref, after, before))
}

func blockGate(fake *fakeGitLabStatuses) chan struct{} {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.block = make(chan struct{})
	return fake.block
}

func TestGitWebhookCIGate_OlderDeliveryCannotSupersedeNewerWait(t *testing.T) {
	rt, _, fake, log := gateWebhookRig(t)
	block := blockGate(fake)
	fake.set(`[{"name":"test","status":"success"}]`)
	gs := gatedPushSource()
	// B is the child of A and arrives first; A is delivered late.
	if status, _ := rt.processGitPushWebhookPayload(t.Context(), "web", gs, gitLabPushBodyBefore("refs/heads/main", gateSHA2, gateSHA), http.Header{}); status != http.StatusAccepted {
		t.Fatalf("B status = %d", status)
	}
	status, msg := rt.processGitPushWebhookPayload(t.Context(), "web", gs, gitLabPushBodyBefore("refs/heads/main", gateSHA, zeroGitSHA), http.Header{})
	if status != http.StatusOK || !strings.Contains(msg, "older than the push already waiting") {
		t.Fatalf("A status = %d, msg = %q", status, msg)
	}
	close(block)
	waitGateBackground(t, rt)
	if got := log.seen(); len(got) != 1 || got[0] != gateSHA2 {
		t.Fatalf("checkouts = %v, want only %s", got, gateSHA2)
	}
}

func TestGitWebhookCIGate_ReplayedBodyDoesNotRestartTheWait(t *testing.T) {
	rt, _, fake, log := gateWebhookRig(t)
	block := blockGate(fake)
	fake.set(`[{"name":"test","status":"success"}]`)
	gs := gatedPushSource()
	body := gitLabPushBody("refs/heads/main", gateSHA, "")
	ctx, marked := withCIGateMark(t.Context())
	if status, _ := rt.processGitPushWebhookPayload(ctx, "web", gs, body, http.Header{}); status != http.StatusAccepted || !marked.Load() {
		t.Fatalf("first push status = %d, marked = %v", status, marked.Load())
	}
	ctx2, marked2 := withCIGateMark(t.Context())
	status, msg := rt.processGitPushWebhookPayload(ctx2, "web", gs, body, http.Header{})
	if status != http.StatusOK || !strings.Contains(msg, "already waiting") || marked2.Load() {
		t.Fatalf("replay status = %d, msg = %q, marked = %v", status, msg, marked2.Load())
	}
	close(block)
	waitGateBackground(t, rt)
	if got := log.seen(); len(got) != 1 {
		t.Fatalf("checkouts = %v, want exactly one", got)
	}
}

func TestGitWebhookCIGate_ShutdownRecordsInterruption(t *testing.T) {
	rt, db, fake, log := gateWebhookRig(t)
	blockGate(fake)
	rt.processGitPushWebhookPayload(t.Context(), "web", gatedPushSource(), gitLabPushBody("refs/heads/main", gateSHA, ""), http.Header{})
	rt.CancelCIGates()
	waitGateBackground(t, rt)
	attempts, err := db.ListDeployAttempts(t.Context(), "web")
	if err != nil || len(attempts) != 1 || !strings.Contains(attempts[0].Error, "interrupted by shutdown") {
		t.Fatalf("attempts = %+v, err = %v", attempts, err)
	}
	if got := log.seen(); len(got) != 0 {
		t.Fatalf("deployed after shutdown: %v", got)
	}
}

func TestGitWebhookCIGate_ReleaseEventFailsClosedWhenTagCannotBeResolved(t *testing.T) {
	rt, _, _, log := gateWebhookRig(t)
	gs := gatedPushSource()
	gs.TriggerMode = spec.TriggerModeRelease
	body := []byte(`{"action":"published","release":{"tag_name":"v1.0.0"}}`)
	status, _ := rt.processGitHubReleaseWebhookEvent(t.Context(), "web", gs, body)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
	if got := log.seen(); len(got) != 0 {
		t.Fatalf("ungated release deployed: %v", got)
	}
}

func TestPolledHeadIsGatedWhenWaitForCIIsOn(t *testing.T) {
	for _, tc := range []struct {
		status string
		deploy bool
	}{{"failed", false}, {"success", true}} {
		rt, db, fake, log := gateWebhookRigSafety(t)
		fake.set(`[{"name":"test","status":"` + tc.status + `"}]`)
		if err := db.NoteGitSourceHead(t.Context(), "web", gateSHA2, time.Now(), false); err != nil {
			t.Fatal(err)
		}
		if err := rt.handlePolledHead(t.Context(), "web", gatedPushSource(), gateSHA); err != nil {
			t.Fatal(err)
		}
		waitGateBackground(t, rt)
		if got := anyCheckout(log, time.Second); got != tc.deploy {
			t.Fatalf("%s: deployed = %v, want %v", tc.status, got, tc.deploy)
		}
	}
}

func TestGitWebhookCIGate_ShutdownCIGatesWaitsForSkipRow(t *testing.T) {
	rt, db, fake, _ := gateWebhookRig(t)
	blockGate(fake)
	rt.processGitPushWebhookPayload(t.Context(), "web", gatedPushSource(), gitLabPushBody("refs/heads/main", gateSHA, ""), http.Header{})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if !rt.ShutdownCIGates(ctx) {
		t.Fatal("ShutdownCIGates hit its deadline")
	}
	attempts, err := db.ListDeployAttempts(t.Context(), "web")
	if err != nil || len(attempts) != 1 || !strings.Contains(attempts[0].Error, "interrupted by shutdown") {
		t.Fatalf("attempts = %+v, err = %v", attempts, err)
	}
}
