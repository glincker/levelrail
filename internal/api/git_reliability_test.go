package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestIsPrivateWebhookHost(t *testing.T) {
	tests := map[string]bool{
		"localhost": true, "127.0.0.1": true, "::1": true, "10.0.0.5": true, "192.168.1.20": true, "172.16.4.1": true,
		"169.254.1.1": true, "100.64.2.2": true, "0.0.0.0": true, "myserver": true, "nas.local": true, "box.internal": true,
		"example.com": false, "deploy.example.com": false, "8.8.8.8": false, "203.0.113.9": false,
	}
	for host, want := range tests {
		if got := isPrivateWebhookHost(host); got != want {
			t.Errorf("isPrivateWebhookHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestGitWebhookBaseResolution(t *testing.T) {
	ctx := context.Background()
	reqCtx := func(host string) context.Context {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Host = host
		return withRequestOrigin(ctx, r)
	}
	tests := []struct {
		name          string
		primary       string
		dashboard     string
		origin        string
		allowPrivate  bool
		wantSource    string
		wantWarning   string
		wantBasePrefx string
	}{
		{name: "primary domain wins", primary: "deploy.example.com", origin: "localhost:9000", wantSource: webhookSourcePrimary, wantBasePrefx: "https://deploy.example.com"},
		{name: "public dashboard url", dashboard: "https://paas.example.com", origin: "localhost:9000", wantSource: webhookSourceDashboard, wantBasePrefx: "https://paas.example.com"},
		{name: "private dashboard url warns", dashboard: "http://192.168.1.5:9000", wantSource: webhookSourceDashboard, wantWarning: webhookWarnPrivate},
		{name: "localhost origin warns instead of silently falling back", origin: "localhost:9000", wantSource: webhookSourceOrigin, wantWarning: webhookWarnPrivate},
		{name: "public origin is fine", origin: "cp.example.org", wantSource: webhookSourceOrigin, wantBasePrefx: "http://cp.example.org"},
		{name: "explicit override silences the warning", origin: "localhost:9000", allowPrivate: true, wantSource: webhookSourceOrigin},
		{name: "nothing known", wantSource: webhookSourceRelative, wantWarning: webhookWarnNoPublic},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			if tt.primary != "" {
				s, _ := db.GetIngressSettings(ctx)
				s.PrimaryDomain = tt.primary
				if err := db.UpdateIngressSettings(ctx, s); err != nil {
					t.Fatal(err)
				}
			}
			if tt.dashboard != "" {
				if err := db.SetDashboardURL(ctx, tt.dashboard); err != nil {
					t.Fatal(err)
				}
			}
			if tt.allowPrivate {
				t.Setenv(envGitWebhookAllowPrivate, "true")
			}
			c := ctx
			if tt.origin != "" {
				c = reqCtx(tt.origin)
			}
			st := rt.gitSourceWebhookStatus(c, "web")
			if st.Source != tt.wantSource || st.Warning != tt.wantWarning {
				t.Fatalf("status = %+v, want source %q warning %q", st, tt.wantSource, tt.wantWarning)
			}
			if !strings.HasPrefix(st.URL, tt.wantBasePrefx) || !strings.HasSuffix(st.URL, "/api/v1/webhooks/github/web") {
				t.Fatalf("url = %q", st.URL)
			}
		})
	}
}

func TestWebhookPublicURLFix(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	fix := rt.webhookPublicURLFix()

	a, err := fix.Detect(ctx)
	if err != nil || a.Failing {
		t.Fatalf("with no git sources there is nothing to fix: %+v, %v", a, err)
	}
	seedWebApp(t, db)
	if err := db.SaveGitSource(ctx, store.GitSource{ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	a, err = fix.Detect(ctx)
	if err != nil || !a.Failing || a.Param == nil || a.Param.Name != "url" {
		t.Fatalf("detect = %+v, %v, want a failing fix asking for a url", a, err)
	}
	for _, bad := range []string{"", "http://localhost:9000", "http://10.0.0.4", "not a url", "https://x.example.com/path"} {
		if err := fix.Run(ctx, fixit.Input{Params: map[string]string{"url": bad}}); err == nil {
			t.Errorf("Run(%q) accepted a url git hosts cannot use", bad)
		}
	}
	if err := fix.Run(ctx, fixit.Input{Params: map[string]string{"url": "https://paas.example.com"}}); err != nil {
		t.Fatalf("Run(public) = %v", err)
	}
	if a, _ = fix.Detect(ctx); a.Failing {
		t.Fatalf("still failing after the fix: %+v", a)
	}
	if st := rt.gitSourceWebhookStatus(ctx, "web"); st.Source != webhookSourceDashboard || st.Warning != "" {
		t.Fatalf("webhook url did not follow the saved public url: %+v", st)
	}
}

func TestWebhookRegistrationFailureIsVisibleAndRetryable(t *testing.T) {
	fakeSecrets := newFakeGitHubAppSecrets()
	fake := &fakeGitHubAppClient{
		mintToken:     githubapp.InstallationToken{Token: "ghs"},
		repo:          githubapp.Repo{FullName: "acme/web", Name: "web", OwnerLogin: "acme", DefaultBranch: "main"},
		createHookErr: githubapp.ErrPermissionDenied,
	}
	rt, db := newTestRouterWithGitHubApp(t, fakeSecrets, fake)
	rt.gitSourceSecrets = newFakeGitSourceSecrets()
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)
	seedWebApp(t, db)
	ctx := context.Background()
	s, _ := db.GetIngressSettings(ctx)
	s.PrimaryDomain = "deploy.example.com"
	if err := db.UpdateIngressSettings(ctx, s); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/github-app/repos/acme/web/use-as-source", `{"app_name":"web"}`))
	if rec.Code >= 300 {
		t.Fatalf("use-as-source = %d %s", rec.Code, rec.Body.String())
	}
	got := getGitSourceResource(t, rt, cookie, "web")
	if got.WebhookRegistration == nil || got.WebhookRegistration.State != store.GitWebhookFailed || got.WebhookRegistration.Error == "" {
		t.Fatalf("registration = %+v, want a visible failure", got.WebhookRegistration)
	}

	fake.createHookErr = nil
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/git-source/webhook/register", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("register again = %d %s", rec.Code, rec.Body.String())
	}
	var again gitSourceResource
	if err := json.Unmarshal(rec.Body.Bytes(), &again); err != nil {
		t.Fatal(err)
	}
	if again.WebhookRegistration == nil || again.WebhookRegistration.State != store.GitWebhookRegistered || !fake.createHookCall {
		t.Fatalf("after retry = %+v (hook called %v)", again.WebhookRegistration, fake.createHookCall)
	}
	if !strings.HasPrefix(fake.createHookURL, "https://deploy.example.com/api/v1/webhooks/github/web") {
		t.Fatalf("hook url = %q", fake.createHookURL)
	}
}

func TestWebhookRegisterAgainRefusesAnUnreachableURL(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.gitSourceSecrets = newFakeGitSourceSecrets()
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)
	if err := db.SaveGitSource(context.Background(), store.GitSource{ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	req := authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/git-source/webhook/register", "")
	req.Host = "localhost:9000"
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	var res gitSourceResource
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if rec.Code != http.StatusOK || res.WebhookRegistration == nil || res.WebhookRegistration.State != store.GitWebhookFailed || res.WebhookURLWarning != webhookWarnPrivate {
		t.Fatalf("status %d res %+v", rec.Code, res)
	}
}

func getGitSourceResource(t *testing.T, rt *Router, cookie *http.Cookie, name string) gitSourceResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/"+name+"/git-source", ""))
	var res gitSourceResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return res
}

func TestProcessPushMultiServiceAppIsQueuedNotBuiltInline(t *testing.T) {
	rt, db, _, gs := newWebhookQueueRouter(t)
	var fetches atomic.Int32
	inner := rt.gitSourceFetch
	rt.gitSourceFetch = func(ctx context.Context, repoURL, ref, token string) (string, func(), error) {
		fetches.Add(1)
		return inner(ctx, repoURL, ref, token)
	}
	gs.Services = map[string]spec.Service{"api": {}, "worker": {}}
	if err := db.SaveGitSource(context.Background(), gs); err != nil {
		t.Fatal(err)
	}

	pctx, outcome := withWebhookOutcome(context.Background())
	start := time.Now()
	status, msg := rt.processGitPushWebhookPayload(pctx, "web", gs, pushPayloadFor("f00dbabe", "org/web"), http.Header{})
	if status != http.StatusAccepted || !strings.HasPrefix(msg, "deploy queued: ") || outcome.AttemptID == "" {
		t.Fatalf("got %d %q outcome %+v, want 202 queued", status, msg, outcome)
	}
	if time.Since(start) > time.Second || fetches.Load() != 0 {
		t.Fatalf("the handler cloned or built inline: %v, %d fetches", time.Since(start), fetches.Load())
	}
	if a := attemptStatus(t, db, outcome.AttemptID); a.Status != store.DeployAttemptStatusQueued || a.CommitSHA != "f00dbabe" {
		t.Fatalf("attempt = %+v", a)
	}
}

func pushWith(before, after, at string) []byte {
	body := `{"ref":"refs/heads/main","before":"` + before + `","after":"` + after + `","repository":{"full_name":"org/web"}`
	if at != "" {
		body += `,"head_commit":{"id":"` + after + `","timestamp":"` + at + `","message":"m","author":{"name":"a"}}`
	}
	return []byte(body + `}`)
}

func TestProcessPushOlderDeliveryAfterNewerIsSkipped(t *testing.T) {
	t.Run("proven by the parent link", func(t *testing.T) {
		rt, db, _, gs := newWebhookQueueRouter(t)
		ctx := context.Background()
		if status, _ := rt.processGitPushWebhookPayload(ctx, "web", gs, pushWith("aaa111", "bbb222", ""), http.Header{}); status != http.StatusAccepted {
			t.Fatalf("newer push = %d", status)
		}
		status, msg := rt.processGitPushWebhookPayload(ctx, "web", gs, pushWith("000", "aaa111", ""), http.Header{})
		if status != http.StatusOK || !strings.Contains(msg, "older than what is already deployed or queued") {
			t.Fatalf("late delivery = %d %q, want a visible skip", status, msg)
		}
		queued := queuedFor(t, db)
		if len(queued) != 1 || queued[0].CommitSHA != "bbb222" {
			t.Fatalf("queued = %+v, want only the newer commit", queued)
		}
	})
	t.Run("proven by the host timestamps", func(t *testing.T) {
		rt, db, _, gs := newWebhookQueueRouter(t)
		ctx := context.Background()
		if status, _ := rt.processGitPushWebhookPayload(ctx, "web", gs, pushWith("x1", "bbb222", "2026-10-20T10:00:00Z"), http.Header{}); status != http.StatusAccepted {
			t.Fatalf("newer push = %d", status)
		}
		status, msg := rt.processGitPushWebhookPayload(ctx, "web", gs, pushWith("x2", "ccc333", "2026-10-20T09:00:00Z"), http.Header{})
		if status != http.StatusOK || !strings.Contains(msg, "older than") {
			t.Fatalf("older-timestamp delivery = %d %q", status, msg)
		}
		if queued := queuedFor(t, db); len(queued) != 1 {
			t.Fatalf("queued = %+v", queued)
		}
	})
	t.Run("a normal fast-forward and a force push still deploy", func(t *testing.T) {
		rt, db, _, gs := newWebhookQueueRouter(t)
		ctx := context.Background()
		_, _ = rt.processGitPushWebhookPayload(ctx, "web", gs, pushWith("aaa", "bbb", "2026-10-20T10:00:00Z"), http.Header{})
		if status, _ := rt.processGitPushWebhookPayload(ctx, "web", gs, pushWith("bbb", "ccc", "2026-10-20T11:00:00Z"), http.Header{}); status != http.StatusAccepted {
			t.Fatalf("fast-forward = %d", status)
		}
		if status, _ := rt.processGitPushWebhookPayload(ctx, "web", gs, pushWith("zzz", "ddd", ""), http.Header{}); status != http.StatusAccepted {
			t.Fatalf("force push with unknown order = %d, want it processed", status)
		}
		if queued := queuedFor(t, db); len(queued) == 0 {
			t.Fatal("nothing queued")
		}
	})
}

func newPollRouter(t *testing.T, head *atomic.Value) (*Router, *store.DB, store.GitSource) {
	t.Helper()
	rt, db, _, gs := newWebhookQueueRouter(t)
	rt.resolveBranchSHA = func(_ context.Context, _, _, _ string) (string, error) {
		h, _ := head.Load().(string)
		return h, nil
	}
	return rt, db, gs
}

func pollOnce(rt *Router, state *gitPollState) {
	rt.pollGitSourcesOnce(context.Background(), gitPollConfig{Enabled: true, Interval: time.Minute, Concurrency: 2, MaxBackoff: time.Hour}, state, time.Now())
}

func queuedFor(t *testing.T, db *store.DB) []store.DeployAttempt {
	t.Helper()
	all, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	var q []store.DeployAttempt
	for _, a := range all {
		if a.Status == store.DeployAttemptStatusQueued || a.Status == store.DeployAttemptStatusRunning {
			q = append(q, a)
		}
	}
	return q
}

func TestGitPollerQueuesAMissedHeadOnceAndNeverBuildsInline(t *testing.T) {
	var head atomic.Value
	head.Store("sha1")
	rt, db, _ := newPollRouter(t, &head)
	release := make(chan struct{})
	inner := rt.gitSourceFetch
	rt.gitSourceFetch = func(ctx context.Context, repoURL, ref, token string) (string, func(), error) {
		<-release
		return inner(ctx, repoURL, ref, token)
	}
	defer close(release)

	state := newGitPollState()
	pollOnce(rt, state)
	if len(queuedFor(t, db)) != 0 {
		t.Fatal("the first head seen must only be remembered, it could be a deliberate rollback")
	}

	head.Store("sha2")
	state = newGitPollState()
	returned := make(chan struct{})
	go func() {
		pollOnce(rt, state)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("the poller blocked on the clone; builds must run in the queue, not inline")
	}
	q := queuedFor(t, db)
	if len(q) != 1 || q[0].CommitSHA != "sha2" || q[0].Source != store.DeployAttemptSourcePoll || deploymentReasonCode(q[0]) != store.DeployReasonPolled {
		t.Fatalf("queued = %+v, want one poll deploy for sha2 with the reason recorded", q)
	}
	if deploymentReasonText(deploymentReasonCode(q[0])) != "Detected by poll, webhook not received" {
		t.Fatalf("reason text = %q", deploymentReasonText(deploymentReasonCode(q[0])))
	}

	state = newGitPollState()
	pollOnce(rt, state)
	if got := len(queuedFor(t, db)); got != 1 {
		t.Fatalf("polling the same head again queued %d deploys", got)
	}
	rel, _ := db.GetGitSourceReliability(context.Background(), "web")
	if rel.LastSeenHead != "sha2" || rel.LastPolledAt.IsZero() || rel.PollError != "" {
		t.Fatalf("reliability = %+v", rel)
	}
}

func TestGitPollerAndWebhookNeverDoubleDeployTheSameCommit(t *testing.T) {
	t.Run("webhook first", func(t *testing.T) {
		var head atomic.Value
		head.Store("sha1")
		rt, db, gs := newPollRouter(t, &head)
		pollOnce(rt, newGitPollState())
		if status, _ := rt.processGitPushWebhookPayload(context.Background(), "web", gs, pushWith("sha1", "sha2", ""), http.Header{}); status != http.StatusAccepted {
			t.Fatalf("webhook = %d", status)
		}
		head.Store("sha2")
		pollOnce(rt, newGitPollState())
		if q := queuedFor(t, db); len(q) != 1 || q[0].Source != store.DeployAttemptSourceWebhook {
			t.Fatalf("queued = %+v, want only the webhook's deploy", q)
		}
	})
	t.Run("poller first", func(t *testing.T) {
		var head atomic.Value
		head.Store("sha1")
		rt, db, gs := newPollRouter(t, &head)
		pollOnce(rt, newGitPollState())
		head.Store("sha2")
		pollOnce(rt, newGitPollState())
		status, msg := rt.processGitPushWebhookPayload(context.Background(), "web", gs, pushWith("sha1", "sha2", ""), http.Header{})
		if status != http.StatusOK || !strings.Contains(msg, "already in flight") {
			t.Fatalf("late webhook = %d %q, want a dedupe", status, msg)
		}
		if q := queuedFor(t, db); len(q) != 1 || q[0].Source != store.DeployAttemptSourcePoll {
			t.Fatalf("queued = %+v", q)
		}
	})
	t.Run("simultaneous", func(t *testing.T) {
		var head atomic.Value
		head.Store("sha1")
		rt, db, gs := newPollRouter(t, &head)
		pollOnce(rt, newGitPollState())
		head.Store("sha2")
		done := make(chan struct{})
		go func() {
			pollOnce(rt, newGitPollState())
			close(done)
		}()
		_, _ = rt.processGitPushWebhookPayload(context.Background(), "web", gs, pushWith("sha1", "sha2", ""), http.Header{})
		<-done
		if q := queuedFor(t, db); len(q) != 1 {
			t.Fatalf("queued %d deploys for one commit: %+v", len(q), q)
		}
	})
}

func TestGitPollerSkipsReleaseModeAndHonorsBackoffAndRateLimit(t *testing.T) {
	var head atomic.Value
	head.Store("sha1")
	rt, db, gs := newPollRouter(t, &head)
	var resolves atomic.Int32
	rt.resolveBranchSHA = func(context.Context, string, string, string) (string, error) {
		resolves.Add(1)
		h, _ := head.Load().(string)
		return h, nil
	}

	gs.TriggerMode = spec.TriggerModeRelease
	if err := db.SaveGitSource(context.Background(), gs); err != nil {
		t.Fatal(err)
	}
	pollOnce(rt, newGitPollState())
	if resolves.Load() != 0 {
		t.Fatal("a source with auto deploy off (release mode) was polled")
	}

	gs.TriggerMode = spec.TriggerModePush
	if err := db.SaveGitSource(context.Background(), gs); err != nil {
		t.Fatal(err)
	}
	state := newGitPollState()
	pollOnce(rt, state)
	pollOnce(rt, state)
	if resolves.Load() != 1 {
		t.Fatalf("resolves = %d, want the second poll deferred until the interval passes", resolves.Load())
	}

	if err := db.SaveGitHubAppConnection(context.Background(), store.GitHubAppConnection{AppID: 1, ClientID: "c", InstanceURL: "https://github.com", CreatedAt: "2026-10-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	rt.recordGitConn(context.Background(), &gitConnError{Provider: gitProviderGitHub, Key: gitProviderGitHub, Status: store.GitConnRateLimited, NextCheck: time.Now().Add(time.Hour), Err: context.Canceled})
	if err := db.SaveGitSource(context.Background(), store.GitSource{ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	before := resolves.Load()
	pollOnce(rt, newGitPollState())
	if resolves.Load() != before {
		t.Fatal("polled a host whose provider connection is rate limited")
	}
}

func TestGitPollerBacksOffAFailingHostAndRecordsTheError(t *testing.T) {
	rt, db, _, _ := newWebhookQueueRouter(t)
	rt.resolveBranchSHA = func(context.Context, string, string, string) (string, error) {
		return "", context.DeadlineExceeded
	}
	state := newGitPollState()
	cfg := gitPollConfig{Enabled: true, Interval: time.Minute, Concurrency: 1, MaxBackoff: time.Hour}
	now := time.Now()
	rt.pollGitSourcesOnce(context.Background(), cfg, state, now)
	rel, _ := db.GetGitSourceReliability(context.Background(), "web")
	if rel.PollError == "" {
		t.Fatal("poll error not recorded")
	}
	if state.due("web", now.Add(90*time.Second)) {
		t.Fatal("a failing source must back off beyond one interval")
	}
}

func TestGitPollerStopsOnContextCancel(t *testing.T) {
	var head atomic.Value
	head.Store("sha1")
	rt, _, _ := newPollRouter(t, &head)
	t.Setenv(envGitPollInterval, "50ms")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		rt.RunGitPoller(ctx)
		close(done)
	}()
	time.Sleep(120 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("poller did not stop after the context was cancelled")
	}
}

func TestGitPollerDisabledByEnv(t *testing.T) {
	var head atomic.Value
	head.Store("sha1")
	rt, _, _ := newPollRouter(t, &head)
	t.Setenv(envGitPollEnabled, "false")
	done := make(chan struct{})
	go func() {
		rt.RunGitPoller(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a disabled poller must return at once")
	}
}
