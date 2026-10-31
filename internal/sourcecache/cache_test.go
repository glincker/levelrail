package sourcecache

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

type fakeHost struct {
	t    *testing.T
	dir  string
	repo *git.Repository
}

func newFakeHost(t *testing.T) *fakeHost {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available for the file transport")
	}
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeHost{t: t, dir: dir, repo: repo}
}

func (h *fakeHost) commit(body string) string {
	const file = "a.txt"
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.dir, file), []byte(body), 0o600); err != nil {
		h.t.Fatal(err)
	}
	wt, err := h.repo.Worktree()
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err := wt.Add(file); err != nil {
		h.t.Fatal(err)
	}
	sig := &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()}
	hash, err := wt.Commit("add "+file, &git.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		h.t.Fatal(err)
	}
	return hash.String()
}

func (h *fakeHost) url() string { return "file://" + h.dir }

func newTestCache(t *testing.T) (*Cache, *int, *[]time.Duration) {
	t.Helper()
	c := New(Config{Dir: t.TempDir(), Retries: 2, Backoff: time.Second, FetchTimeout: time.Minute}, nil)
	fetches := 0
	var sleeps []time.Duration
	realFetch := c.fetchFn
	c.fetchFn = func(ctx context.Context, repo *git.Repository, auth transport.AuthMethod) error {
		fetches++
		return realFetch(ctx, repo, auth)
	}
	c.sleep = func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}
	return c, &fetches, &sleeps
}

func readFile(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "a.txt")) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestKnownCommitNeedsNoHost(t *testing.T) {
	h := newFakeHost(t)
	sha := h.commit("one")
	c, fetches, _ := newTestCache(t)

	co, err := c.Checkout(context.Background(), h.url(), sha, "")
	if err != nil {
		t.Fatal(err)
	}
	if co.Commit != sha || co.FromCache || readFile(t, co.Dir) != "one" {
		t.Fatalf("first checkout: %+v", co)
	}
	co.Cleanup()
	if *fetches != 1 {
		t.Fatalf("fetches = %d, want 1", *fetches)
	}

	c.fetchFn = func(context.Context, *git.Repository, transport.AuthMethod) error {
		t.Fatal("known commit must not fetch")
		return nil
	}
	co, err = c.Checkout(context.Background(), h.url(), sha, "")
	if err != nil {
		t.Fatal(err)
	}
	defer co.Cleanup()
	if !co.FromCache || readFile(t, co.Dir) != "one" {
		t.Fatalf("second checkout: %+v", co)
	}
}

func TestBranchRefreshesAndFailsFastWhenHostDown(t *testing.T) {
	h := newFakeHost(t)
	first := h.commit("one")
	c, _, sleeps := newTestCache(t)
	branch := "master"
	if ref, err := h.repo.Head(); err == nil {
		branch = ref.Name().Short()
	}

	co, err := c.Checkout(context.Background(), h.url(), branch, "")
	if err != nil || co.Commit != first {
		t.Fatalf("checkout = %+v, %v", co, err)
	}
	co.Cleanup()
	second := h.commit("two")
	co, err = c.Checkout(context.Background(), h.url(), branch, "")
	if err != nil || co.Commit != second {
		t.Fatalf("branch must follow the host: %+v, %v", co, err)
	}
	co.Cleanup()

	calls := 0
	c.fetchFn = func(context.Context, *git.Repository, transport.AuthMethod) error {
		calls++
		return &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	}
	_, err = c.Checkout(context.Background(), h.url(), branch, "")
	if !errors.Is(err, ErrHostUnreachable) {
		t.Fatalf("err = %v, want ErrHostUnreachable", err)
	}
	if calls != 3 || len(*sleeps) != 2 || (*sleeps)[0] != time.Second || (*sleeps)[1] != 2*time.Second {
		t.Fatalf("calls=%d sleeps=%v, want 3 calls and 1s,2s backoff", calls, *sleeps)
	}
	if got := c.Unreachable(); len(got) != 1 || got[0].Failures != 1 {
		t.Fatalf("unreachable = %+v", got)
	}

	co, err = c.Checkout(WithOffline(context.Background()), h.url(), branch, "")
	if err != nil || co.Commit != second || !co.FromCache {
		t.Fatalf("offline last known good: %+v, %v", co, err)
	}
	co.Cleanup()
}

func TestOfflineMissIsNotCached(t *testing.T) {
	h := newFakeHost(t)
	h.commit("one")
	c, fetches, _ := newTestCache(t)
	_, err := c.Checkout(WithOffline(context.Background()), h.url(), "master", "")
	if !errors.Is(err, ErrNotCached) || *fetches != 0 {
		t.Fatalf("err = %v, fetches = %d", err, *fetches)
	}
}

func TestPermanentErrorIsNotRetried(t *testing.T) {
	h := newFakeHost(t)
	h.commit("one")
	c, _, sleeps := newTestCache(t)
	calls := 0
	c.fetchFn = func(context.Context, *git.Repository, transport.AuthMethod) error {
		calls++
		return errors.New("authentication required")
	}
	_, err := c.Checkout(context.Background(), h.url(), "master", "")
	if err == nil || errors.Is(err, ErrHostUnreachable) || calls != 1 || len(*sleeps) != 0 {
		t.Fatalf("err=%v calls=%d sleeps=%v", err, calls, *sleeps)
	}
}

func TestInterruptedCheckoutKeepsMirrorUsable(t *testing.T) {
	h := newFakeHost(t)
	sha := h.commit("one")
	c, _, _ := newTestCache(t)

	var dest string
	c.export = func(_ string, _ plumbing.Hash, d string) error {
		dest = d
		return errors.New("checkout interrupted")
	}
	if _, err := c.Checkout(context.Background(), h.url(), sha, ""); err == nil {
		t.Fatal("expected the interrupted checkout to fail")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("half-written checkout dir left behind: %v", err)
	}

	c.export = exportWorktree
	c.fetchFn = func(context.Context, *git.Repository, transport.AuthMethod) error {
		t.Fatal("fetch completed earlier, retry must not need the host")
		return nil
	}
	co, err := c.Checkout(context.Background(), h.url(), sha, "")
	if err != nil {
		t.Fatal(err)
	}
	defer co.Cleanup()
	if !co.FromCache || readFile(t, co.Dir) != "one" {
		t.Fatalf("recovered checkout: %+v", co)
	}
}

func TestDirectModeKeepsNoMirror(t *testing.T) {
	h := newFakeHost(t)
	sha := h.commit("one")
	c := New(Config{}, nil)
	co, err := c.Checkout(context.Background(), h.url(), sha, "")
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, co.Dir) != "one" {
		t.Fatal("wrong content")
	}
	co.Cleanup()
	if _, err := os.Stat(co.Dir); !os.IsNotExist(err) {
		t.Fatal("cleanup must remove the checkout")
	}
}

func TestRecoverOnceClearsHostAndReportsBlocked(t *testing.T) {
	c := New(Config{Dir: t.TempDir()}, nil)
	url := "https://git.example.com/org/app.git"
	c.health.markFailed(url, time.Now(), "dial tcp: connection refused")
	c.Block(url, "web")
	c.Block(url, "api")

	c.probeFn = func(context.Context, string) error { return errors.New("connection refused") }
	c.RecoverOnce(context.Background(), func(context.Context, string, []string) { t.Fatal("host still down") })
	if len(c.Unreachable()) != 1 {
		t.Fatal("host must stay unreachable")
	}

	c.probeFn = func(context.Context, string) error { return nil }
	var gotHost string
	var gotBlocked []string
	c.RecoverOnce(context.Background(), func(_ context.Context, host string, blocked []string) { gotHost, gotBlocked = host, blocked })
	if gotHost != "git.example.com" || len(gotBlocked) != 2 || gotBlocked[0] != "api" || len(c.Unreachable()) != 0 {
		t.Fatalf("host=%q blocked=%v unreachable=%v", gotHost, gotBlocked, c.Unreachable())
	}
}

func TestIsUnreachable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"refused", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, true},
		{"timeout", context.DeadlineExceeded, true},
		{"503", errors.New("unexpected client error: unexpected requested status code: 503"), true},
		{"dns", errors.New("dial tcp: lookup github.com: no such host"), true},
		{"auth", errors.New("authentication required"), false},
		{"other", errors.New("object not found"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUnreachable(tt.err); got != tt.want {
				t.Fatalf("IsUnreachable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestMirrorIsSharedOnlyWithinOneScope(t *testing.T) {
	h := newFakeHost(t)
	sha := h.commit("one")
	c, fetches, _ := newTestCache(t)
	scoped := WithScope(context.Background(), "app:web")

	co, err := c.Checkout(scoped, h.url(), sha, "")
	if err != nil {
		t.Fatal(err)
	}
	co.Cleanup()
	before := *fetches

	co, err = c.Checkout(scoped, h.url(), sha, "")
	if err != nil {
		t.Fatal(err)
	}
	co.Cleanup()
	if !co.FromCache || *fetches != before {
		t.Fatalf("same scope: fromCache=%v fetches=%d want %d", co.FromCache, *fetches, before)
	}

	for _, ctx := range []context.Context{context.Background(), WithScope(context.Background(), "app:other"), WithScope(context.Background(), "ghapp:acme")} {
		co, err = c.Checkout(ctx, h.url(), sha, "")
		if err != nil {
			t.Fatal(err)
		}
		co.Cleanup()
		if co.FromCache {
			t.Fatalf("scope %q was served another scope's mirror", scopeOf(ctx))
		}
	}
	if *fetches != before+3 {
		t.Fatalf("fetches = %d, want %d", *fetches, before+3)
	}
}

func TestAnonymousCloneUsesOwnMirror(t *testing.T) {
	h := newFakeHost(t)
	sha := h.commit("one")
	c, _, _ := newTestCache(t)
	co, err := c.Checkout(context.Background(), h.url(), sha, "")
	if err != nil {
		t.Fatal(err)
	}
	defer co.Cleanup()
	if readFile(t, co.Dir) != "one" {
		t.Fatal("wrong content")
	}
}

func TestPushMirrorKeyIgnoresScope(t *testing.T) {
	if mirrorKey("push://local/web", "") != mirrorKey("push://local/web", "app:web") {
		t.Fatal("push mirror key must not depend on scope")
	}
	if mirrorKey("https://example.com/a/b", "") == mirrorKey("https://example.com/a/b", "app:x") {
		t.Fatal("remote mirror key must depend on scope")
	}
}
