package giteaapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/dockertest"
	"github.com/glincker/levelrail/internal/gitprovider"
)

// Live tests drive the real clients against throwaway Gitea and Forgejo
// containers started on random loopback ports. They run only when
// LEVELRAIL_FORGE_LIVE=1 (scripts/qa/forge-gitea.sh sets it).

type liveForge struct {
	t    *testing.T
	c    *Client
	ctx  context.Context
	f    *dockertest.Forge
	hits *deliveries
}

// deliveries records webhook posts the forge makes to the host receiver.
type deliveries struct {
	mu   sync.Mutex
	list []delivery
}

type delivery struct {
	event   string
	body    []byte
	sig     string
	sigName string
}

func (d *deliveries) add(x delivery) {
	d.mu.Lock()
	d.list = append(d.list, x)
	d.mu.Unlock()
}

func (d *deliveries) wait(event string, within time.Duration) (delivery, bool) {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		for _, x := range d.list {
			if x.event == event {
				d.mu.Unlock()
				return x, true
			}
		}
		d.mu.Unlock()
		time.Sleep(500 * time.Millisecond)
	}
	return delivery{}, false
}

func (e *liveForge) redact(err error) string {
	if err == nil {
		return ""
	}
	return strings.ReplaceAll(err.Error(), e.f.Token, "[redacted]")
}

// call is setup plumbing for endpoints the client has no function for.
func (e *liveForge) call(method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	return e.c.do(e.ctx, method, apiBaseURL(e.f.URL)+path, "Bearer "+e.f.Token, rd, out)
}

func (e *liveForge) step(name string, fn func() error) bool {
	ok := true
	e.t.Run(name, func(t *testing.T) {
		if err := fn(); err != nil {
			ok = false
			t.Errorf("FAIL %s: %s", name, e.redact(err))
			return
		}
		t.Logf("PASS %s", name)
	})
	return ok
}

func startReceiver(t *testing.T, hits *deliveries) string {
	t.Helper()
	// Bound on all interfaces so the container reaches it as host.docker.internal.
	ln, err := net.Listen("tcp", "0.0.0.0:0") //nolint:gosec // the container must reach the receiver through the host gateway
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ev := r.Header.Get("X-Gitea-Event")
		if ev == "" {
			ev = r.Header.Get("X-Forgejo-Event")
		}
		sig, name := r.Header.Get("X-Gitea-Signature"), "X-Gitea-Signature"
		if sig == "" {
			sig, name = r.Header.Get("X-Forgejo-Signature"), "X-Forgejo-Signature"
		}
		hits.add(delivery{event: ev, body: body, sig: sig, sigName: name})
		w.WriteHeader(http.StatusOK)
	}))
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return "http://host.docker.internal:" + port + "/hook"
}

func TestLiveForge(t *testing.T) {
	if os.Getenv("LEVELRAIL_FORGE_LIVE") != "1" {
		t.Skip("LEVELRAIL_FORGE_LIVE not set")
	}
	t.Setenv(gitprovider.AllowPrivateEnv, "true")
	for _, kind := range []dockertest.ForgeKind{dockertest.Gitea, dockertest.Forgejo} {
		t.Run(kind.Name, func(t *testing.T) { liveForgeRun(t, kind) })
	}
}

func liveForgeRun(t *testing.T, kind dockertest.ForgeKind) {
	f := dockertest.StartForge(t, kind)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	e := &liveForge{t: t, c: NewClient(), ctx: ctx, f: f, hits: &deliveries{}}
	inst, tok := f.URL, f.Token

	e.step("current_user", func() error {
		a, err := e.c.CurrentUser(ctx, inst, tok)
		if err == nil && a.Login != f.Admin {
			return fmt.Errorf("login = %q, want %q", a.Login, f.Admin)
		}
		return err
	})

	repo := f.Admin + "/demo"
	orgRepo := "acme/svc"
	if !e.step("setup_repos_and_org", func() error {
		if err := e.call("POST", "/user/repos", map[string]any{"name": "demo", "auto_init": true, "default_branch": "main"}, nil); err != nil {
			return err
		}
		if err := e.call("POST", "/orgs", map[string]any{"username": "acme"}, nil); err != nil {
			return err
		}
		return e.call("POST", "/orgs/acme/repos", map[string]any{"name": "svc", "auto_init": true, "default_branch": "main"}, nil)
	}) {
		return
	}

	e.step("list_and_get_repo", func() error {
		rs, _, err := e.c.ListRepos(ctx, inst, tok, gitprovider.ListOptions{})
		if err != nil {
			return err
		}
		if len(rs) < 2 {
			return fmt.Errorf("ListRepos returned %d repos, want at least 2", len(rs))
		}
		r, err := e.c.GetRepo(ctx, inst, tok, repo)
		if err != nil {
			return err
		}
		if r.DefaultBranch != "main" || !r.Admin || r.CloneURL == "" {
			return fmt.Errorf("GetRepo = %+v", r)
		}
		bs, err := e.c.ListBranches(ctx, inst, tok, repo)
		if err != nil || len(bs) == 0 {
			return badf(fmt.Sprintf("ListBranches = %v", bs), err)
		}
		return nil
	})

	e.step("list_by_owner_and_org", func() error {
		orgs, err := e.c.ListOrgs(ctx, inst, tok)
		if err != nil || len(orgs) != 1 || orgs[0] != "acme" {
			return badf(fmt.Sprintf("ListOrgs = %v", orgs), err)
		}
		org, _, err := e.c.ListOwnerRepos(ctx, inst, tok, "acme", gitprovider.ListOptions{})
		if err != nil || len(org) != 1 || org[0].FullName != orgRepo {
			return badf(fmt.Sprintf("org repos = %+v", org), err)
		}
		usr, _, err := e.c.ListOwnerRepos(ctx, inst, tok, f.Admin, gitprovider.ListOptions{})
		if err != nil || len(usr) != 1 || usr[0].FullName != repo {
			return badf(fmt.Sprintf("user repos = %+v", usr), err)
		}
		return nil
	})

	e.step("probe_access", func() error {
		a, err := e.c.ProbeAccess(ctx, inst, tok, gitprovider.ListOptions{MaxItems: 50})
		if err != nil {
			return err
		}
		if !a.CanAdmin || !a.CanPush || !a.CanPull || a.Sampled != 2 || len(a.Orgs) != 1 {
			return fmt.Errorf("access = %+v", a)
		}
		t.Logf("owners=%v", a.Owners)
		return nil
	})

	e.step("oauth_app_create_and_delete", func() error {
		app, err := e.c.CreateOAuthApp(ctx, inst, tok, "levelrail-live-"+kind.Name, "http://127.0.0.1:1/cb")
		if err != nil {
			return err
		}
		if app.ClientID == "" || app.Secret == "" {
			return fmt.Errorf("created app has no client credentials: %+v", app.ID)
		}
		var listed []map[string]any
		if err := e.call("GET", "/user/applications/oauth2", nil, &listed); err != nil {
			return err
		}
		if len(listed) != 1 {
			return fmt.Errorf("server lists %d applications, want 1", len(listed))
		}
		return e.c.DeleteOAuthApp(ctx, inst, tok, app.ID)
	})

	e.step("oauth_code_flow_and_expiry", func() error { return e.oauthFlow(inst) })

	hookURL := startReceiver(t, e.hits)
	const secret = "live-hook-secret"
	var hookID int64
	e.step("webhook_register_idempotent", func() error {
		h1, err := e.c.EnsureRepoWebhook(ctx, inst, tok, repo, hookURL, secret)
		if err != nil {
			return err
		}
		h2, err := e.c.EnsureRepoWebhook(ctx, inst, tok, repo, hookURL, secret)
		if err != nil {
			return err
		}
		hooks, err := e.c.ListRepoWebhooks(ctx, inst, tok, repo)
		if err != nil {
			return err
		}
		if h1.ID != h2.ID || len(hooks) != 1 || !hooks[0].Active || len(hooks[0].Events) < 2 {
			return fmt.Errorf("hooks = %+v (first %d, second %d)", hooks, h1.ID, h2.ID)
		}
		hookID = h1.ID
		return nil
	})

	e.step("webhook_test_delivery_signed", func() error {
		if err := e.c.TestRepoWebhook(ctx, inst, tok, repo, hookID); err != nil {
			return err
		}
		return e.expectSigned("push", secret)
	})

	// A real push through the contents API fires the push hook as git would.
	e.hits.reset()
	var prHead string
	e.step("push_event_signed", func() error {
		if err := e.call("POST", "/repos/"+repo+"/branches", map[string]any{"new_branch_name": "feat", "old_branch_name": "main"}, nil); err != nil {
			return err
		}
		var res struct {
			Commit struct {
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		err := e.call("POST", "/repos/"+repo+"/contents/web/app.txt", map[string]any{
			"branch": "feat", "message": "add app", "content": base64.StdEncoding.EncodeToString([]byte("hello\n")),
		}, &res)
		if err != nil {
			return err
		}
		prHead = res.Commit.SHA
		return e.expectSigned("push", secret)
	})

	var prNumber int
	e.step("pull_request_event_signed", func() error {
		var pr struct {
			Number int `json:"number"`
		}
		if err := e.call("POST", "/repos/"+repo+"/pulls", map[string]any{"head": "feat", "base": "main", "title": "preview me"}, &pr); err != nil {
			return err
		}
		prNumber = pr.Number
		return e.expectSigned("pull_request", secret)
	})

	e.step("commit_status_roundtrip", func() error {
		if err := e.c.CreateCommitStatus(ctx, inst, tok, repo, prHead, CommitStatusPending, "https://app.example.invalid/p", "deploying", "levelrail/preview"); err != nil {
			return err
		}
		if err := e.c.CreateCommitStatus(ctx, inst, tok, repo, prHead, CommitStatusSuccess, "https://app.example.invalid/p", "ready", "levelrail/preview"); err != nil {
			return err
		}
		var st struct {
			State    string `json:"state"`
			Statuses []struct {
				Context string `json:"context"`
				Status  string `json:"status"`
			} `json:"statuses"`
		}
		if err := e.call("GET", "/repos/"+repo+"/commits/"+prHead+"/status", nil, &st); err != nil {
			return err
		}
		if st.State != "success" || len(st.Statuses) != 1 || st.Statuses[0].Context != "levelrail/preview" {
			return fmt.Errorf("combined status = %+v", st)
		}
		return nil
	})

	e.step("preview_comment_upsert", func() error {
		id, err := e.c.CreateIssueComment(ctx, inst, tok, repo, prNumber, "preview: deploying")
		if err != nil {
			return err
		}
		if err := e.c.UpdateIssueComment(ctx, inst, tok, repo, id, "preview: ready"); err != nil {
			return err
		}
		cs, err := e.c.ListIssueComments(ctx, inst, tok, repo, prNumber)
		if err != nil {
			return err
		}
		if len(cs) != 1 || cs[0].ID != id || cs[0].Body != "preview: ready" {
			return fmt.Errorf("comments = %+v", cs)
		}
		return nil
	})

	e.step("changed_files", func() error {
		pr, err := e.c.PullRequestChangedFiles(ctx, inst, tok, repo, prNumber)
		if err != nil || len(pr) != 1 || pr[0] != "web/app.txt" {
			return badf(fmt.Sprintf("PullRequestChangedFiles = %v", pr), err)
		}
		cmp, err := e.c.CompareChangedFiles(ctx, inst, tok, repo, "main", "feat")
		if err != nil || len(cmp) != 1 || cmp[0] != "web/app.txt" {
			return badf(fmt.Sprintf("CompareChangedFiles = %v", cmp), err)
		}
		return nil
	})

	e.step("exchange_bad_code_is_api_error", func() error {
		_, err := e.c.ExchangeCode(ctx, inst, "nope", "nope", "http://127.0.0.1:1/cb", "bad", "v")
		if err == nil {
			return fmt.Errorf("a bad code exchanged successfully")
		}
		return nil
	})

	e.step("webhook_delete", func() error {
		if err := e.c.DeleteRepoWebhook(ctx, inst, tok, repo, hookID); err != nil {
			return err
		}
		hooks, err := e.c.ListRepoWebhooks(ctx, inst, tok, repo)
		if err == nil && len(hooks) != 0 {
			return fmt.Errorf("%d hooks remain", len(hooks))
		}
		return err
	})
}

func (d *deliveries) reset() {
	d.mu.Lock()
	d.list = nil
	d.mu.Unlock()
}

func (e *liveForge) expectSigned(event, secret string) error {
	d, ok := e.hits.wait(event, 30*time.Second)
	if !ok {
		return fmt.Errorf("no %q delivery within 30s", event)
	}
	if !VerifySignature(secret, d.body, d.sig) {
		return fmt.Errorf("%s delivery has a bad %s", event, d.sigName)
	}
	e.t.Logf("%s delivery verified through %s", event, d.sigName)
	return nil
}

var (
	reInput = regexp.MustCompile(`<input[^>]*>`)
	reAttr  = regexp.MustCompile(`(name|value)="([^"]*)"`)
)

// formFields returns the named inputs of a page, enough to replay a login or grant form.
func formFields(page string) map[string]string {
	out := map[string]string{}
	for _, in := range reInput.FindAllString(page, -1) {
		var name, value string
		for _, m := range reAttr.FindAllStringSubmatch(in, -1) {
			if m[1] == "name" {
				name = m[2]
			} else {
				value = m[2]
			}
		}
		if name != "" {
			out[name] = value
		}
	}
	return out
}

// oauthFlow plays a browser through login and consent so the real token
// endpoint, refresh and the reported expiry are exercised.
func (e *liveForge) oauthFlow(inst string) error {
	app, err := e.c.CreateOAuthApp(e.ctx, inst, e.f.Token, "levelrail-flow-"+e.f.Kind.Name, "http://127.0.0.1:1/cb")
	if err != nil {
		return err
	}
	defer func() { _ = e.c.DeleteOAuthApp(e.ctx, inst, e.f.Token, app.ID) }()

	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(u string) (string, *http.Response, error) {
		resp, err := browser.Get(u)
		if err != nil {
			return "", nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return string(b), resp, nil
	}
	post := func(u string, form url.Values) (*http.Response, error) {
		resp, err := browser.PostForm(u, form)
		if err != nil {
			return nil, err
		}
		_ = resp.Body.Close()
		return resp, nil
	}

	page, _, err := get(inst + "/user/login")
	if err != nil {
		return err
	}
	csrf := formFields(page)["_csrf"]
	if _, err := post(inst+"/user/login", url.Values{"_csrf": {csrf}, "user_name": {e.f.Admin}, "password": {e.f.Password}}); err != nil {
		return err
	}

	verifier := "live-verifier-0123456789-0123456789-0123456789-abcdef"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	authURL := AuthorizeURL(inst, app.ClientID, "http://127.0.0.1:1/cb", "st8", challenge)
	page, resp, err := get(authURL)
	if err != nil {
		return err
	}
	var loc string
	if resp.StatusCode == http.StatusSeeOther || resp.StatusCode == http.StatusFound {
		loc = resp.Header.Get("Location")
	} else {
		fields := formFields(page)
		form := url.Values{}
		for _, k := range []string{"_csrf", "client_id", "state", "scope", "nonce", "redirect_uri"} {
			form.Set(k, fields[k])
		}
		form.Set("granted", "true")
		gresp, gerr := post(inst+"/login/oauth/grant", form)
		if gerr != nil {
			return gerr
		}
		loc = gresp.Header.Get("Location")
	}
	u, err := url.Parse(loc)
	if err != nil || u.Query().Get("code") == "" {
		return fmt.Errorf("no authorization code in redirect %q", strings.SplitN(loc, "?", 2)[0])
	}

	tk, err := e.c.ExchangeCode(e.ctx, inst, app.ClientID, app.Secret, "http://127.0.0.1:1/cb", u.Query().Get("code"), verifier)
	if err != nil {
		return err
	}
	if tk.AccessToken == "" || tk.RefreshToken == "" {
		return fmt.Errorf("exchange returned access=%t refresh=%t", tk.AccessToken != "", tk.RefreshToken != "")
	}
	e.t.Logf("oauth token expires in %s", time.Until(tk.ExpiresAt).Round(time.Minute))
	if tk.ExpiresAt.IsZero() {
		return fmt.Errorf("server reported no token expiry")
	}
	acct, err := e.c.CurrentUser(e.ctx, inst, tk.AccessToken)
	if err != nil || acct.Login != e.f.Admin {
		return badf(fmt.Sprintf("oauth token identity = %+v", acct), err)
	}
	if _, err := e.c.ProbeAccess(e.ctx, inst, tk.AccessToken, gitprovider.ListOptions{MaxItems: 10}); err != nil {
		return fmt.Errorf("probe with oauth token: %w", err)
	}
	// Tokens are JWTs stamped to the second, so an instant refresh can repeat the string.
	time.Sleep(2 * time.Second)
	rt, err := e.c.RefreshToken(e.ctx, inst, app.ClientID, app.Secret, tk.RefreshToken)
	if err != nil {
		return err
	}
	if rt.AccessToken == "" || rt.AccessToken == tk.AccessToken {
		return fmt.Errorf("refresh did not mint a new access token")
	}
	if acct, err := e.c.CurrentUser(e.ctx, inst, rt.AccessToken); err != nil || acct.Login != e.f.Admin {
		return badf(fmt.Sprintf("refreshed token identity = %+v", acct), err)
	}
	return nil
}

func badf(msg string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", msg, err)
	}
	return errors.New(msg)
}
