package sourcecache

import (
	"bytes"
	"context"
	"encoding/base64"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/cgi" //nolint:gosec // test-only fixture server
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureToken = "tok-b113-0123456789abcdef"

// credentialedGit serves a tiny bare repo over smart HTTP and demands
// basic auth with fixtureToken, using the real git binary.
func credentialedGit(t *testing.T) (repoURL, sha string) {
	t.Helper()
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	out, err := exec.Command(gitBin, "--exec-path").Output() //nolint:gosec // fixed args
	if err != nil {
		t.Skip("git exec-path unavailable")
	}
	backend := filepath.Join(strings.TrimSpace(string(out)), "git-http-backend")
	if _, err := os.Stat(backend); err != nil {
		t.Skip("git-http-backend not installed")
	}

	root := t.TempDir()
	work := filepath.Join(root, "work")
	run := func(dir string, args ...string) string {
		cmd := exec.Command(gitBin, args...) //nolint:gosec // literal args in test
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, b)
		}
		return strings.TrimSpace(string(b))
	}
	if err := os.MkdirAll(work, 0o750); err != nil {
		t.Fatal(err)
	}
	run(work, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(work, "secret.txt"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(work, "add", "secret.txt")
	run(work, "commit", "-q", "-m", "init")
	sha = run(work, "rev-parse", "HEAD")
	run(root, "clone", "-q", "--bare", work, filepath.Join(root, "priv.git"))

	h := &cgi.Handler{Path: backend, Env: []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pw, ok := r.BasicAuth(); !ok || pw != fixtureToken {
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/priv.git", sha
}

func TestCredentialedHostIsNotServedAcrossCredentials(t *testing.T) {
	repoURL, sha := credentialedGit(t)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cacheDir := t.TempDir()
	c := New(Config{Dir: cacheDir, Retries: 0, FetchTimeout: 0}, logger)

	scopeA := WithScope(context.Background(), "app:a")
	co, err := c.Checkout(scopeA, repoURL, "main", fixtureToken)
	if err != nil {
		t.Fatalf("authorized clone failed: %v", err)
	}
	co.Cleanup()

	var errText strings.Builder
	cases := []struct {
		name  string
		ctx   context.Context
		ref   string
		token string
	}{
		{"no scope no token, branch", context.Background(), "main", ""},
		{"no scope no token, full sha", context.Background(), sha, ""},
		{"other scope no token, full sha", WithScope(context.Background(), "app:b"), sha, ""},
		{"other scope wrong token, full sha", WithScope(context.Background(), "app:b"), sha, "wrong-token-zzz"},
		{"other scope wrong token, branch", WithScope(context.Background(), "app:b"), "main", "wrong-token-zzz"},
	}
	for _, tc := range cases {
		got, err := c.Checkout(tc.ctx, repoURL, tc.ref, tc.token)
		if err == nil {
			got.Cleanup()
			t.Errorf("%s: served private contents without valid credential (fromCache=%v)", tc.name, got.FromCache)
			continue
		}
		errText.WriteString(err.Error() + "\n")
	}

	// Offline lookups must not read another scope's mirror either.
	if got, err := c.Checkout(WithOffline(WithScope(context.Background(), "app:b")), repoURL, sha, ""); err == nil {
		got.Cleanup()
		t.Error("offline lookup in another scope was served from cache")
	}

	assertNoSecret(t, cacheDir, logs.String()+errText.String())
}

// assertNoSecret scans names and contents under dir, plus any captured text.
func assertNoSecret(t *testing.T, dir, captured string) {
	t.Helper()
	needles := []string{
		fixtureToken,
		base64.StdEncoding.EncodeToString([]byte(fixtureToken)),
		base64.StdEncoding.EncodeToString([]byte("x-access-token:" + fixtureToken)),
	}
	hit := func(s string) string {
		for _, n := range needles {
			if strings.Contains(s, n) {
				return n[:6] + "..."
			}
		}
		return ""
	}
	if n := hit(captured); n != "" {
		t.Errorf("credential material (%s) found in logs or error text", n)
	}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if n := hit(p); n != "" {
			t.Errorf("credential material (%s) in path %s", n, p)
		}
		if d.Type().IsRegular() {
			b, rerr := os.ReadFile(p) //nolint:gosec // test walks its own temp dir
			if rerr != nil {
				return rerr
			}
			if n := hit(string(b)); n != "" {
				t.Errorf("credential material (%s) in file %s", n, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
