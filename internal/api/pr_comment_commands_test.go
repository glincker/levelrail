package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeGitHub is an httptest GitHub: it answers the three calls comment
// commands make and records every request.
type fakeGitHub struct {
	mu          sync.Mutex
	perms       map[string]string // login -> permission
	permStatus  int               // non-zero forces this status on the permission route
	permBody    string            // non-empty replaces the permission body
	pr          map[string]any
	prStatus    int
	permCalls   atomic.Int32
	prCalls     atomic.Int32
	replies     []string
	authHeaders []string
}

func newFakeGitHub(t *testing.T) (*fakeGitHub, *httptest.Server) {
	t.Helper()
	f := &fakeGitHub{perms: map[string]string{"octo": "write"}, pr: prCmdOpenPR("org/web", "main")}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.authHeaders = append(f.authHeaders, r.Header.Get("Authorization"))
		switch {
		case strings.Contains(r.URL.Path, "/collaborators/"):
			f.permCalls.Add(1)
			if f.permStatus != 0 {
				w.WriteHeader(f.permStatus)
				return
			}
			if f.permBody != "" {
				_, _ = w.Write([]byte(f.permBody))
				return
			}
			login := strings.Split(strings.Split(r.URL.Path, "/collaborators/")[1], "/")[0]
			perm, ok := f.perms[login]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"permission": perm})
		case strings.Contains(r.URL.Path, "/pulls/"):
			f.prCalls.Add(1)
			if f.prStatus != 0 {
				w.WriteHeader(f.prStatus)
				return
			}
			_ = json.NewEncoder(w).Encode(f.pr)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/comments"):
			var in struct {
				Body string `json:"body"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			f.replies = append(f.replies, in.Body)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Errorf("unexpected GitHub request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func prCmdOpenPR(headRepo, baseRef string) map[string]any {
	return map[string]any{
		"number": 7, "state": "open", "merged": false,
		"head": map[string]any{"ref": "feature-x", "sha": "sha1", "repo": map[string]any{"full_name": headRepo}},
		"base": map[string]any{"ref": baseRef, "repo": map[string]any{"full_name": "org/web"}},
	}
}

func (f *fakeGitHub) lastReply() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.replies) == 0 {
		return ""
	}
	return f.replies[len(f.replies)-1]
}

func (f *fakeGitHub) replyCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.replies)
}

func (f *fakeGitHub) apiCalls() int32 { return f.permCalls.Load() + f.prCalls.Load() }

// prCmdGitHubClient sends the comment-command calls to the httptest server and
// keeps the fake's installation token minting.
type prCmdGitHubClient struct {
	*fakeGitHubAppClient
	real *githubapp.Client
}

func (c prCmdGitHubClient) GetPullRequest(ctx context.Context, inst, tok, owner, repo string, n int) (githubapp.PullRequest, error) {
	return c.real.GetPullRequest(ctx, inst, tok, owner, repo, n)
}

func (c prCmdGitHubClient) GetCollaboratorPermission(ctx context.Context, inst, tok, owner, repo, login string) (string, error) {
	return c.real.GetCollaboratorPermission(ctx, inst, tok, owner, repo, login)
}

func (c prCmdGitHubClient) CreateIssueComment(ctx context.Context, inst, tok, owner, repo string, n int, body string) (int64, error) {
	return c.real.CreateIssueComment(ctx, inst, tok, owner, repo, n, body)
}

type prCmdFixture struct {
	t       *testing.T
	rt      *Router
	db      *store.DB
	gh      *fakeGitHub
	builder *sequencedBuilder
	secret  string
	nextID  int64
}

func testPRCommentConfig() PRCommentCommandConfig {
	return PRCommentCommandConfig{
		PerPRPerMinute: 50, PerUserPerMinute: 50, PermissionCacheTTL: time.Minute,
		LogRetention: time.Hour, RunTimeout: time.Minute,
	}
}

// newPRCmdFixture wires app "web" on org/web with commands switched on.
func newPRCmdFixture(t *testing.T, cfg PRCommentCommandConfig, commandsOn bool) *prCmdFixture {
	t.Helper()
	gh, srv := newFakeGitHub(t)
	githubSecrets := newFakeGitHubAppSecrets()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithGitHubAppSecrets(githubSecrets), WithGitSourceSecrets(newFakeGitSourceSecrets()),
		WithDeploySafety(db, nil), WithPRCommentCommands(cfg))
	rt.githubAppClient = prCmdGitHubClient{
		fakeGitHubAppClient: &fakeGitHubAppClient{mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"}},
		real:                &githubapp.Client{HTTP: srv.Client(), BaseURL: srv.URL},
	}
	if rt.prCommands != nil {
		rt.prCommands.run = func(f func()) { f() }
	}
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, githubSecrets)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)
	setPreviewEnabled(t, rt, cookie)
	if commandsOn {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/preview-settings", `{"pr_comment_commands":true}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("enable pr comment commands: %d %s", rec.Code, rec.Body.String())
		}
	}
	builder := &sequencedBuilder{db: db, tag: "levelrail/web:sha1"}
	rt.builder = builder
	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)
	rt.resolveBranchSHA = func(context.Context, string, string, string) (string, error) { return "sha1", nil }
	return &prCmdFixture{t: t, rt: rt, db: db, gh: gh, builder: builder, secret: created.WebhookSecret, nextID: 1000}
}

type commentOpts struct {
	id            int64
	body          string
	login         string
	userType      string
	senderType    string
	viaApp        bool
	action        string
	notPR         bool
	repo          string
	unsigned      bool
	badSignature  bool
	omitEventHdr  bool
	wrongForgeHdr string
}

func (f *prCmdFixture) send(o commentOpts) *httptest.ResponseRecorder {
	f.t.Helper()
	if o.id == 0 {
		f.nextID++
		o.id = f.nextID
	}
	if o.login == "" {
		o.login = "octo"
	}
	if o.userType == "" {
		o.userType = "User"
	}
	if o.senderType == "" {
		o.senderType = "User"
	}
	if o.action == "" {
		o.action = "created"
	}
	if o.repo == "" {
		o.repo = "org/web"
	}
	issue := map[string]any{"number": 7}
	if !o.notPR {
		issue["pull_request"] = map[string]any{"url": "x"}
	}
	var via any
	if o.viaApp {
		via = map[string]any{"id": 3}
	}
	body, _ := json.Marshal(map[string]any{
		"action": o.action, "issue": issue,
		"comment":    map[string]any{"id": o.id, "body": o.body, "user": map[string]any{"login": o.login, "type": o.userType}, "performed_via_github_app": via},
		"sender":     map[string]any{"type": o.senderType},
		"repository": map[string]any{"full_name": o.repo},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", bytes.NewReader(body))
	if !o.omitEventHdr {
		req.Header.Set("X-GitHub-Event", "issue_comment")
	}
	if o.wrongForgeHdr != "" {
		req.Header.Set(o.wrongForgeHdr, "issue_comment")
	}
	req.Header.Set("X-GitHub-Delivery", fmt.Sprintf("d-%d", o.id))
	switch {
	case o.unsigned:
	case o.badSignature:
		req.Header.Set("X-Hub-Signature-256", sign([]byte("wrong"), body))
	default:
		req.Header.Set("X-Hub-Signature-256", sign([]byte(f.secret), body))
	}
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, req)
	return rec
}

func (f *prCmdFixture) attempts() []store.DeployAttempt {
	f.t.Helper()
	a, err := f.db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		f.t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	return a
}

func TestPRComment_Redeploy_RecordsActorAndTrigger(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	rec := f.send(commentOpts{body: "/levelrail redeploy"})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(f.builder.calls) != 1 {
		t.Fatalf("builder calls = %d, want 1", len(f.builder.calls))
	}
	var found bool
	for _, a := range f.attempts() {
		if a.Source == store.DeployAttemptSourcePRComment {
			found = true
			if a.Author != "octo" {
				t.Errorf("attempt author = %q, want the GitHub login octo", a.Author)
			}
		}
	}
	if !found {
		t.Fatalf("no attempt with source %q in %+v", store.DeployAttemptSourcePRComment, f.attempts())
	}
	if got := f.gh.lastReply(); !strings.Contains(got, "Redeploy") || !strings.Contains(got, prCommentMarker) {
		t.Errorf("reply = %q", got)
	}
	for _, h := range f.gh.authHeaders {
		if h != "Bearer ghs_installtoken" {
			t.Errorf("GitHub call used auth %q, want the installation token", h)
		}
	}
}

func TestPRComment_Unsigned_Rejected(t *testing.T) {
	tests := []struct {
		name string
		o    commentOpts
	}{
		{"no signature", commentOpts{body: "/levelrail redeploy", unsigned: true}},
		{"wrong signature", commentOpts{body: "/levelrail redeploy", badSignature: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPRCmdFixture(t, testPRCommentConfig(), true)
			rec := f.send(tt.o)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if len(f.builder.calls) != 0 || f.gh.apiCalls() != 0 || f.gh.replyCount() != 0 {
				t.Errorf("unsigned delivery had effects: builds=%d api=%d replies=%d", len(f.builder.calls), f.gh.apiCalls(), f.gh.replyCount())
			}
		})
	}
}

func TestPRComment_FeatureGates_FailClosed(t *testing.T) {
	disabled := testPRCommentConfig()
	disabled.Disabled = true
	badLimit := testPRCommentConfig()
	badLimit.PerUserPerMinute = 0
	tests := []struct {
		name string
		cfg  PRCommentCommandConfig
		on   bool
	}{
		{"app setting off by default", testPRCommentConfig(), false},
		{"env kill switch beats app setting", disabled, true},
		{"zero rate limit turns the feature off", badLimit, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPRCmdFixture(t, tt.cfg, tt.on)
			rec := f.send(commentOpts{body: "/levelrail redeploy"})
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "off") {
				t.Fatalf("status = %d body = %q, want an ignored delivery", rec.Code, rec.Body.String())
			}
			if len(f.builder.calls) != 0 || f.gh.apiCalls() != 0 {
				t.Errorf("gated comment still acted: builds=%d api=%d", len(f.builder.calls), f.gh.apiCalls())
			}
		})
	}
}

func TestPRComment_PermissionCheck_FailsClosed(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*fakeGitHub)
	}{
		{"read permission", func(g *fakeGitHub) { g.perms["octo"] = "read" }},
		{"none permission", func(g *fakeGitHub) { g.perms["octo"] = "none" }},
		{"triage maps to read", func(g *fakeGitHub) { g.perms["octo"] = "triage" }},
		{"unknown permission string", func(g *fakeGitHub) { g.perms["octo"] = "owner-ish" }},
		{"not a collaborator", func(g *fakeGitHub) { delete(g.perms, "octo") }},
		{"api 500", func(g *fakeGitHub) { g.permStatus = http.StatusInternalServerError }},
		{"api 403", func(g *fakeGitHub) { g.permStatus = http.StatusForbidden }},
		{"api 401", func(g *fakeGitHub) { g.permStatus = http.StatusUnauthorized }},
		{"garbage body", func(g *fakeGitHub) { g.permBody = "not json" }},
		{"empty permission", func(g *fakeGitHub) { g.permBody = `{"permission":""}` }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPRCmdFixture(t, testPRCommentConfig(), true)
			tt.setup(f.gh)
			f.send(commentOpts{body: "/levelrail redeploy"})
			if len(f.builder.calls) != 0 {
				t.Fatal("a deploy ran for a commenter without confirmed write access")
			}
			if f.gh.replyCount() != 0 {
				t.Errorf("unauthorized commenter got a reply: %q", f.gh.lastReply())
			}
			if f.gh.prCalls.Load() != 0 {
				t.Error("pull request was read before the permission check passed")
			}
		})
	}
}

func TestPRComment_PermissionAllowed(t *testing.T) {
	for _, perm := range []string{"write", "admin"} {
		t.Run(perm, func(t *testing.T) {
			f := newPRCmdFixture(t, testPRCommentConfig(), true)
			f.gh.perms["octo"] = perm
			f.send(commentOpts{body: "/levelrail status"})
			if f.gh.replyCount() != 1 {
				t.Fatalf("replies = %d, want 1", f.gh.replyCount())
			}
		})
	}
}

func TestPRComment_PermissionCache_ReusedBriefly(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	f.send(commentOpts{body: "/levelrail status"})
	f.send(commentOpts{body: "/levelrail status"})
	if got := f.gh.permCalls.Load(); got != 1 {
		t.Fatalf("permission lookups = %d, want 1 (cached)", got)
	}
	now := time.Now().Add(2 * time.Minute)
	f.rt.prCommands.now = func() time.Time { return now }
	f.send(commentOpts{body: "/levelrail status"})
	if got := f.gh.permCalls.Load(); got != 2 {
		t.Fatalf("permission lookups after expiry = %d, want 2", got)
	}
}

func TestPRComment_PermissionRevoked_NotCachedAsAllowed(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	f.gh.perms["octo"] = "read"
	f.send(commentOpts{body: "/levelrail status"})
	f.gh.perms["octo"] = "write"
	f.send(commentOpts{body: "/levelrail status"})
	if f.gh.replyCount() != 1 {
		t.Fatalf("replies = %d, want 1: a refusal must not be cached", f.gh.replyCount())
	}
}

func TestPRComment_BotsAndOwnComments_Ignored(t *testing.T) {
	tests := []struct {
		name string
		o    commentOpts
	}{
		{"user type Bot", commentOpts{userType: "Bot"}},
		{"login ends with [bot]", commentOpts{login: "dependabot[bot]"}},
		{"sender is a Bot", commentOpts{senderType: "Bot"}},
		{"written by a GitHub App", commentOpts{viaApp: true}},
		{"carries our own reply marker", commentOpts{body: prCommentMarker + "\n/levelrail redeploy"}},
		{"login with path characters", commentOpts{login: "../admin"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPRCmdFixture(t, testPRCommentConfig(), true)
			if tt.o.body == "" {
				tt.o.body = "/levelrail redeploy"
			}
			f.gh.perms[tt.o.login] = "admin"
			rec := f.send(tt.o)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			if len(f.builder.calls) != 0 || f.gh.apiCalls() != 0 || f.gh.replyCount() != 0 {
				t.Errorf("ignored comment had effects: builds=%d api=%d replies=%d", len(f.builder.calls), f.gh.apiCalls(), f.gh.replyCount())
			}
		})
	}
}

func TestPRComment_NotACommandOrNotNew_Ignored(t *testing.T) {
	tests := []struct {
		name string
		o    commentOpts
	}{
		{"edited", commentOpts{body: "/levelrail redeploy", action: "edited"}},
		{"deleted", commentOpts{body: "/levelrail redeploy", action: "deleted"}},
		{"plain issue", commentOpts{body: "/levelrail redeploy", notPR: true}},
		{"no command", commentOpts{body: "looks good"}},
		{"quoted reply", commentOpts{body: "> /levelrail redeploy"}},
		{"code block", commentOpts{body: "```\n/levelrail redeploy\n```"}},
		{"another repository", commentOpts{body: "/levelrail redeploy", repo: "evil/web"}},
		{"gitea compat header", commentOpts{body: "/levelrail redeploy", wrongForgeHdr: "X-Gitea-Event-Type"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPRCmdFixture(t, testPRCommentConfig(), true)
			rec := f.send(tt.o)
			// A non-GitHub forge's comment event falls through to the push parser, which rejects it.
			if rec.Code >= http.StatusBadRequest && tt.o.wrongForgeHdr == "" {
				t.Fatalf("status = %d", rec.Code)
			}
			if len(f.builder.calls) != 0 || f.gh.replyCount() != 0 {
				t.Errorf("builds=%d replies=%d, want none", len(f.builder.calls), f.gh.replyCount())
			}
		})
	}
}

func TestPRComment_MultipleCommands_NothingRuns(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	f.send(commentOpts{body: "/levelrail redeploy\n/levelrail rollback"})
	if len(f.builder.calls) != 0 {
		t.Fatal("ambiguous comment deployed")
	}
	if !strings.Contains(f.gh.lastReply(), "one") {
		t.Errorf("reply = %q, want a one-command-per-comment hint", f.gh.lastReply())
	}
}

func TestPRComment_PullRequestState(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(map[string]any)
		prStatus  int
		wantReply string
		wantBuild bool
	}{
		{"closed", func(m map[string]any) { m["state"] = "closed" }, 0, "not open", false},
		{"merged", func(m map[string]any) { m["merged"] = true }, 0, "not open", false},
		{"fork head", func(m map[string]any) { m["head"].(map[string]any)["repo"] = map[string]any{"full_name": "fork/web"} }, 0, "forks", false},
		{"deleted fork head", func(m map[string]any) { m["head"].(map[string]any)["repo"] = nil }, 0, "forks", false},
		{"other base branch", func(m map[string]any) { m["base"].(map[string]any)["ref"] = "develop" }, 0, "not the deployed branch", false},
		{"api error", nil, http.StatusInternalServerError, "", false},
		{"open same repo", nil, 0, "Redeploy", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPRCmdFixture(t, testPRCommentConfig(), true)
			if tt.mutate != nil {
				tt.mutate(f.gh.pr)
			}
			f.gh.prStatus = tt.prStatus
			f.send(commentOpts{body: "/levelrail redeploy"})
			if got := len(f.builder.calls) > 0; got != tt.wantBuild {
				t.Fatalf("deployed = %v, want %v", got, tt.wantBuild)
			}
			if !strings.Contains(f.gh.lastReply(), tt.wantReply) {
				t.Errorf("reply = %q, want it to contain %q", f.gh.lastReply(), tt.wantReply)
			}
		})
	}
}

func TestPRComment_ForkAllowedBySetting(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	f.gh.pr["head"].(map[string]any)["repo"] = map[string]any{"full_name": "fork/web"}
	if err := f.db.SetGitSourcePRCommentCommandsForks(context.Background(), "web", true); err != nil {
		t.Fatal(err)
	}
	f.send(commentOpts{body: "/levelrail status"})
	if strings.Contains(f.gh.lastReply(), "forks") || f.gh.replyCount() != 1 {
		t.Fatalf("reply = %q, want a status reply once forks are allowed", f.gh.lastReply())
	}
}

func TestPRComment_Idempotent_PerCommentID(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	for i := 0; i < 3; i++ {
		f.send(commentOpts{id: 555, body: "/levelrail redeploy"})
	}
	if len(f.builder.calls) != 1 {
		t.Fatalf("builder calls = %d, want 1 for a redelivered comment", len(f.builder.calls))
	}
	if f.gh.replyCount() != 1 {
		t.Errorf("replies = %d, want 1", f.gh.replyCount())
	}
	f.send(commentOpts{body: "/levelrail redeploy"})
	if len(f.builder.calls) != 2 {
		t.Fatalf("builder calls = %d, want a new comment id to run", len(f.builder.calls))
	}
}

func TestPRComment_RateLimits(t *testing.T) {
	t.Run("per user", func(t *testing.T) {
		cfg := testPRCommentConfig()
		cfg.PerUserPerMinute = 2
		f := newPRCmdFixture(t, cfg, true)
		for i := 0; i < 5; i++ {
			f.send(commentOpts{body: "/levelrail status"})
		}
		if f.gh.replyCount() != 2 {
			t.Fatalf("replies = %d, want 2 before the user limit", f.gh.replyCount())
		}
		f.gh.perms["other"] = "write"
		f.send(commentOpts{body: "/levelrail status", login: "other"})
		if f.gh.replyCount() != 3 {
			t.Errorf("a second user was throttled by the first one's budget")
		}
	})
	t.Run("per pull request", func(t *testing.T) {
		cfg := testPRCommentConfig()
		cfg.PerPRPerMinute = 2
		f := newPRCmdFixture(t, cfg, true)
		f.gh.perms["other"] = "write"
		f.send(commentOpts{body: "/levelrail status"})
		f.send(commentOpts{body: "/levelrail status", login: "other"})
		f.send(commentOpts{body: "/levelrail status", login: "other"})
		f.send(commentOpts{body: "/levelrail status"})
		if f.gh.replyCount() != 2 {
			t.Fatalf("replies = %d, want 2 before the pull request limit", f.gh.replyCount())
		}
	})
	t.Run("limited comments do not reach github", func(t *testing.T) {
		cfg := testPRCommentConfig()
		cfg.PerUserPerMinute = 1
		f := newPRCmdFixture(t, cfg, true)
		f.send(commentOpts{body: "/levelrail status"})
		before := f.gh.apiCalls()
		f.send(commentOpts{body: "/levelrail status"})
		if f.gh.apiCalls() != before {
			t.Error("a rate limited comment still called the GitHub API")
		}
	})
}

func TestPRComment_FreezeRefusal(t *testing.T) {
	for _, cmd := range []string{"/levelrail redeploy", "/levelrail rollback", "/levelrail preview redeploy"} {
		t.Run(cmd, func(t *testing.T) {
			f := newPRCmdFixture(t, testPRCommentConfig(), true)
			if _, err := f.db.ReplaceDeployFreezeWindows(context.Background(), store.DeployFreezeScopeApp("web"),
				[]store.DeployFreezeWindow{{Cron: "* * * * *", Duration: 2 * time.Hour, Reason: "quarter end @everyone [click](http://evil.example) <b>x</b>"}}); err != nil {
				t.Fatal(err)
			}
			f.send(commentOpts{body: cmd})
			if len(f.builder.calls) != 0 {
				t.Fatal("a frozen app deployed from a comment")
			}
			reply := f.gh.lastReply()
			if !strings.Contains(reply, "frozen") {
				t.Fatalf("reply = %q, want the freeze reason", reply)
			}
			for _, bad := range []string{"@", "[", "]", "<", ">", "(http"} {
				if strings.Contains(strings.TrimPrefix(reply, prCommentMarker), bad) {
					t.Errorf("reply echoes %q unescaped: %q", bad, reply)
				}
			}
		})
	}
}

func TestPRComment_ProtectedEnvironment_NeedsApproval(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	seedProtectedDeployFixture(t, f.db)
	svc, _ := f.db.GetDesiredService(context.Background(), "web")
	svc.EnvironmentID = "env_prod"
	if err := f.db.SaveDesiredService(context.Background(), *svc); err != nil {
		t.Fatal(err)
	}
	f.send(commentOpts{body: "/levelrail redeploy"})
	if len(f.builder.calls) != 0 {
		t.Fatal("a protected environment deployed without approval")
	}
	if !strings.Contains(f.gh.lastReply(), "approval") {
		t.Errorf("reply = %q, want an approval explanation", f.gh.lastReply())
	}
}

func TestPRComment_Status_IsReadOnly(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	before := len(f.attempts())
	f.send(commentOpts{body: "/levelrail status"})
	if len(f.builder.calls) != 0 || len(f.attempts()) != before {
		t.Fatal("status changed deploy state")
	}
	if !strings.Contains(f.gh.lastReply(), "Running image") {
		t.Errorf("reply = %q", f.gh.lastReply())
	}
}

func TestPRComment_Rollback_NoEarlierDeploy(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	f.send(commentOpts{body: "/levelrail rollback"})
	if len(f.builder.calls) != 0 {
		t.Fatal("rollback built something")
	}
	if !strings.Contains(f.gh.lastReply(), "no earlier successful deploy") {
		t.Errorf("reply = %q", f.gh.lastReply())
	}
}

func TestPRComment_PreviewRedeploy(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	f.builder.tag = "levelrail/web-pr-7:sha1"
	f.send(commentOpts{body: "/levelrail preview redeploy"})
	if len(f.builder.calls) != 1 {
		t.Fatalf("builder calls = %d, want 1", len(f.builder.calls))
	}
	p, err := f.db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 7)
	if err != nil || p.HeadSHA != "sha1" {
		t.Fatalf("preview = %+v, err = %v", p, err)
	}
	if !strings.Contains(f.gh.lastReply(), "Preview redeploy") {
		t.Errorf("reply = %q", f.gh.lastReply())
	}
}

func TestPRComment_PreviewRedeploy_PreviewsOff(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	if err := f.db.SetGitSourcePreviewEnabled(context.Background(), "web", false); err != nil {
		t.Fatal(err)
	}
	f.send(commentOpts{body: "/levelrail preview redeploy"})
	if len(f.builder.calls) != 0 || !strings.Contains(f.gh.lastReply(), "off") {
		t.Fatalf("calls = %d reply = %q", len(f.builder.calls), f.gh.lastReply())
	}
}

func TestSanitizeCommentText(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hello world", "hello world"},
		{"@everyone ping", "everyone ping"},
		{"[a](http://x.example)", "a http x.example"},
		{"<script>alert(1)</script>", "script alert 1 /script"},
		{"`code` **bold** # head", "code bold head"},
		{"line1\nline2\r\nline3", "line1 line2 line3"},
		{"https://evil.example/x", "https evil.example/x"},
		{strings.Repeat("a", 500), strings.Repeat("a", 20) + "..."},
	}
	for _, tt := range tests {
		limit := 500
		if strings.HasPrefix(tt.want, "aaaa") {
			limit = 20
		}
		if got := sanitizeCommentText(tt.in, limit); got != tt.want {
			t.Errorf("sanitizeCommentText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPRCommentSettings_DefaultOffAndToggle(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), false)
	gs, err := f.db.GetGitSource(context.Background(), "web")
	if err != nil || gs.PRCommentCommands || gs.PRCommentCommandsForks {
		t.Fatalf("defaults = %+v, err = %v, want both off", gs, err)
	}
	cookie := loginTestSession(t, f.rt, f.db)
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/preview-settings", `{"pr_comment_commands":true}`))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"pr_comment_commands":true`) || !strings.Contains(rec.Body.String(), `"pr_comment_commands_forks":false`) {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/preview-settings", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty body status = %d, want 400", rec.Code)
	}
}
