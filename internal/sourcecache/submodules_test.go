package sourcecache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

func storeBlob(t *testing.T, s storer.EncodedObjectStorer, data []byte) plumbing.Hash {
	t.Helper()
	obj := s.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	h, err := s.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func storeTree(t *testing.T, s storer.EncodedObjectStorer, entries []object.TreeEntry) plumbing.Hash {
	t.Helper()
	obj := s.NewEncodedObject()
	if err := (&object.Tree{Entries: entries}).Encode(obj); err != nil {
		t.Fatal(err)
	}
	h, err := s.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func storeCommit(t *testing.T, s storer.EncodedObjectStorer, tree plumbing.Hash) plumbing.Hash {
	t.Helper()
	sig := object.Signature{Name: "t", Email: "t@example.com", When: time.Unix(1700000000, 0)}
	obj := s.NewEncodedObject()
	if err := (&object.Commit{Author: sig, Committer: sig, Message: "c", TreeHash: tree}).Encode(obj); err != nil {
		t.Fatal(err)
	}
	h, err := s.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// subRepo is a local repository a submodule URL is mapped to.
func subRepo(t *testing.T) (dir string, commit plumbing.Hash) {
	t.Helper()
	dir = t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	blob := storeBlob(t, repo.Storer, []byte("from the submodule\n"))
	tree := storeTree(t, repo.Storer, []object.TreeEntry{{Name: "lib.txt", Mode: filemode.Regular, Hash: blob}})
	commit = storeCommit(t, repo.Storer, tree)
	if err := repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/main", commit)); err != nil {
		t.Fatal(err)
	}
	return dir, commit
}

// parentCheckout builds a bare parent with .gitmodules, a gitlink at lib
// and an LFS pointer, and exports it the way a real Checkout does.
func parentCheckout(t *testing.T, gitmodules string, subCommit plumbing.Hash, lfsBody []byte) string {
	t.Helper()
	bare := t.TempDir()
	repo, err := git.PlainInit(bare, true)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(lfsBody)
	pointer := fmt.Sprintf("%s\noid sha256:%s\nsize %d\n", lfsSpecVersion, hex.EncodeToString(sum[:]), len(lfsBody))
	entries := []object.TreeEntry{
		{Name: ".gitmodules", Mode: filemode.Regular, Hash: storeBlob(t, repo.Storer, []byte(gitmodules))},
		{Name: "big.bin", Mode: filemode.Regular, Hash: storeBlob(t, repo.Storer, []byte(pointer))},
		{Name: "lib", Mode: filemode.Submodule, Hash: subCommit},
		{Name: "readme.txt", Mode: filemode.Regular, Hash: storeBlob(t, repo.Storer, []byte("parent\n"))},
	}
	commit := storeCommit(t, repo.Storer, storeTree(t, repo.Storer, entries))
	dest := t.TempDir()
	if err := exportWorktree(bare, commit, dest); err != nil {
		t.Fatal(err)
	}
	return dest
}

type lfsServer struct {
	srv  *httptest.Server
	body []byte
	mu   sync.Mutex
	auth []string
}

func newLFSServer(t *testing.T, body []byte) *lfsServer {
	t.Helper()
	ls := &lfsServer{body: body}
	ls.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ls.mu.Lock()
		ls.auth = append(ls.auth, r.Header.Get("Authorization"))
		ls.mu.Unlock()
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ".git/info/lfs/objects/batch"):
			var req struct {
				Objects []struct {
					OID  string `json:"oid"`
					Size int64  `json:"size"`
				} `json:"objects"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			type act struct {
				Href string `json:"href"`
			}
			out := map[string]any{"objects": []any{}}
			objs := []any{}
			for _, o := range req.Objects {
				objs = append(objs, map[string]any{"oid": o.OID, "size": o.Size, "actions": map[string]any{"download": act{Href: ls.srv.URL + "/objects/" + o.OID}}})
			}
			out["objects"] = objs
			w.Header().Set("Content-Type", lfsMediaType)
			_ = json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/objects/"):
			_, _ = w.Write(ls.body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ls.srv.Close)
	return ls
}

func TestGitSubmodulesAndLFS(t *testing.T) {
	subDir, subCommit := subRepo(t)
	lfsBody := []byte(strings.Repeat("large file content ", 200))
	ls := newLFSServer(t, lfsBody)
	parentURL := ls.srv.URL + "/org/app.git"
	parentHost := urlHost(parentURL)
	creds := &githttp.BasicAuth{Username: "x-access-token", Password: "test-token"}

	cases := []struct {
		name       string
		modURL     string
		ex         Extras
		wantErr    error
		wantSub    bool
		wantLFS    bool
		wantCredOn bool
		mapped     string
	}{
		{name: "relative submodule on parent host gets creds", modURL: "../lib.git", mapped: "https://" + parentHost + "/org/lib.git", wantSub: true, wantLFS: true, wantCredOn: true},
		{name: "other host fetched without creds", modURL: "https://other.example/lib.git", mapped: "https://other.example/lib.git", wantSub: true, wantLFS: true},
		{name: "other host listed gets creds", modURL: "https://other.example/lib.git", mapped: "https://other.example/lib.git", ex: Extras{CredentialHosts: []string{"other.example"}}, wantSub: true, wantLFS: true, wantCredOn: true},
		{name: "ssh refused", modURL: "ssh://git@other.example/lib.git", wantErr: ErrSubmoduleURL},
		{name: "scp ssh refused", modURL: "git@other.example:org/lib.git", wantErr: ErrSubmoduleURL},
		{name: "file refused", modURL: "file:///" + subDir, wantErr: ErrSubmoduleURL},
		{name: "git protocol refused", modURL: "git://other.example/lib.git", wantErr: ErrSubmoduleURL},
		{name: "embedded credentials refused", modURL: (&url.URL{Scheme: "https", User: url.UserPassword("user", "fake"), Host: "other.example", Path: "/lib.git"}).String(), wantErr: ErrSubmoduleURL},
		{name: "relative escape refused", modURL: "../../../lib.git", wantErr: ErrSubmoduleURL},
		{name: "count cap", modURL: "../lib.git", ex: Extras{MaxCount: -1}, wantErr: ErrSubmoduleLimit},
		{name: "missing credential is clear", modURL: "https://private.example/lib.git", mapped: "auth", wantErr: ErrSubmoduleAuth},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gm := fmt.Sprintf("[submodule \"lib\"]\n\tpath = lib\n\turl = %s\n", tc.modURL)
			dest := parentCheckout(t, gm, subCommit, lfsBody)
			ex := Extras{Submodules: true, LFS: true, MaxDepth: 3, MaxCount: 8, MaxBytes: 64 << 20, Timeout: time.Minute, CredentialHosts: tc.ex.CredentialHosts}
			if tc.ex.MaxCount < 0 {
				ex.MaxCount = 0
			}
			c := New(Config{}, nil)
			c.lfsClient = ls.srv.Client()
			var gotAuth transport.AuthMethod
			var gotURL string
			c.subFetch = func(ctx context.Context, dir, u string, auth transport.AuthMethod) error {
				gotAuth, gotURL = auth, u
				if tc.mapped == "auth" {
					return transport.ErrAuthenticationRequired
				}
				return fetchInto(ctx, dir, subDir, u, nil)
			}
			err := c.fetchExtras(context.Background(), dest, parentURL, creds, ex)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if strings.Contains(err.Error(), "test-token") {
					t.Fatal("error leaks the token")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if gotURL != tc.mapped {
				t.Fatalf("fetched %q, want %q", gotURL, tc.mapped)
			}
			if (gotAuth != nil) != tc.wantCredOn {
				t.Fatalf("submodule auth sent = %v, want %v", gotAuth != nil, tc.wantCredOn)
			}
			if b, err := fs.ReadFile(os.DirFS(dest), "lib/lib.txt"); err != nil || string(b) != "from the submodule\n" {
				t.Fatalf("submodule content = %q, %v", b, err)
			}
			if b, err := fs.ReadFile(os.DirFS(dest), "big.bin"); err != nil || string(b) != string(lfsBody) {
				t.Fatalf("lfs object not materialized: %v", err)
			}
		})
	}
}

func TestGitSubmodulesLFSForeignHostRefused(t *testing.T) {
	body := []byte("payload")
	ls := newLFSServer(t, body)
	other := newLFSServer(t, body)
	dest := t.TempDir()
	sum := sha256.Sum256(body)
	pointer := fmt.Sprintf("%s\noid sha256:%s\nsize %d\n", lfsSpecVersion, hex.EncodeToString(sum[:]), len(body))
	if err := os.WriteFile(filepath.Join(dest, "a.bin"), []byte(pointer), 0o600); err != nil {
		t.Fatal(err)
	}
	c := New(Config{}, nil)
	c.lfsClient = ls.srv.Client()
	run := &extrasRun{c: c, ex: Extras{LFS: true, MaxBytes: 1 << 20}, root: dest, parentHost: urlHost(ls.srv.URL)}
	act := lfsAction{Href: other.srv.URL + "/objects/x"}
	err := run.lfsDownload(context.Background(), run.httpClient(run.parentHost), run.parentHost, nil, act, lfsPointer{file: filepath.Join(dest, "a.bin"), oid: hex.EncodeToString(sum[:]), size: int64(len(body))})
	if !errors.Is(err, ErrLFSRefused) {
		t.Fatalf("err = %v, want ErrLFSRefused", err)
	}
	u, _ := url.Parse(other.srv.URL)
	run.ex.LFSHosts = []string{u.Host}
	if err := run.lfsDownload(context.Background(), run.httpClient(run.parentHost), run.parentHost, nil, act, lfsPointer{file: filepath.Join(dest, "a.bin"), oid: hex.EncodeToString(sum[:]), size: int64(len(body))}); err != nil {
		t.Fatalf("listed host: %v", err)
	}
}

func TestGitSubmodulesPathAndURLRules(t *testing.T) {
	for _, p := range []string{"../x", "/abs", "a/../../x", ".git/hooks", "a/.GIT", ".", ""} {
		if _, err := safeSubmodulePath(p); err == nil {
			t.Errorf("path %q accepted", p)
		}
	}
	if got, err := safeSubmodulePath("vendor/lib"); err != nil || got != filepath.FromSlash("vendor/lib") {
		t.Errorf("vendor/lib = %q, %v", got, err)
	}
	if u, err := resolveSubmoduleURL("https://h.example/org/app.git", "./sub.git"); err != nil || u.String() != "https://h.example/org/app.git/sub.git" {
		t.Errorf("./sub.git = %v, %v", u, err)
	}
	if _, err := resolveSubmoduleURL("ssh://h.example/org/app.git", "../sub.git"); err == nil {
		t.Error("relative url under an ssh parent accepted")
	}
	// A symlinked directory on the path must not be followed out of the checkout.
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "vendor")); err != nil {
		t.Skip("symlinks unsupported")
	}
	run := &extrasRun{root: root}
	if _, err := run.prepareDir(root, filepath.FromSlash("vendor/lib")); err == nil {
		t.Error("symlinked component accepted")
	}
}

func TestGitSubmodulesBlockedAddresses(t *testing.T) {
	for _, a := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "::1", "::ffff:10.0.0.1", "100.64.0.1", "0.0.0.0", "192.168.1.1", "fe80::1", "224.0.0.1"} {
		if err := dialControl(nil)("tcp", net.JoinHostPort(a, "443"), nil); !errors.Is(err, errAddressBlocked) {
			t.Errorf("%s allowed", a)
		}
	}
	if err := dialControl(nil)("tcp", "140.82.112.3:443", nil); err != nil {
		t.Errorf("public address blocked: %v", err)
	}
	allowed := parseCIDRs([]string{"10.0.0.0/8", "::1"})
	for _, a := range []string{"10.1.2.3", "::ffff:10.0.0.1", "::1"} {
		if err := dialControl(allowed)("tcp", net.JoinHostPort(a, "443"), nil); err != nil {
			t.Errorf("%s refused despite allowlist: %v", a, err)
		}
	}
	for _, h := range []string{"https://2130706433/x.git", "https://0x7f000001/x.git", "https://017700000001/x.git", "https://127.1/x.git"} {
		if _, err := resolveSubmoduleURL("https://h.example/a.git", h); err == nil {
			t.Errorf("%s accepted", h)
		}
	}
	// No policy on the context means no dial at all.
	if _, err := guardedTransport().RoundTrip(httptest.NewRequest(http.MethodGet, "https://h.example/", nil)); err == nil {
		t.Error("request without a policy was dialed")
	}
}

func TestGitSubmodulesGenericUnreachable(t *testing.T) {
	_, subCommit := subRepo(t)
	dest := parentCheckout(t, "[submodule \"lib\"]\n\tpath = lib\n\turl = https://127.0.0.1:1/lib.git\n", subCommit, []byte("x"))
	c := New(Config{}, nil)
	err := c.fetchExtras(context.Background(), dest, "https://h.example/org/app.git", nil, Extras{Submodules: true, MaxDepth: 3, MaxCount: 8, MaxBytes: 64 << 20, Timeout: 10 * time.Second})
	if !errors.Is(err, ErrSubmoduleUnreachable) {
		t.Fatalf("err = %v, want ErrSubmoduleUnreachable", err)
	}
	if strings.Contains(err.Error(), "allowed") || strings.Contains(err.Error(), "refused") || strings.Contains(err.Error(), "timeout") {
		t.Fatalf("error reveals the cause: %v", err)
	}
}

func TestGitSubmodulesCountingBodyCap(t *testing.T) {
	b := &atomic.Int64{}
	b.Store(10)
	body := &countingBody{ReadCloser: io.NopCloser(strings.NewReader(strings.Repeat("a", 64))), budget: b}
	if _, err := io.ReadAll(body); !errors.Is(err, errBudgetExceeded) {
		t.Fatalf("err = %v, want errBudgetExceeded", err)
	}
}

func TestGitSubmodulesLFSHostIsSubmoduleHost(t *testing.T) {
	run := &extrasRun{parentHost: "parent.example", ex: Extras{LFSHosts: []string{"cdn.example"}}}
	if !run.lfsHostAllowed("sub.example", "sub.example") {
		t.Error("submodule's own host refused")
	}
	if run.lfsHostAllowed("sub.example", "parent.example") {
		t.Error("parent host accepted for a submodule's objects")
	}
	if !run.lfsHostAllowed("sub.example", "cdn.example") {
		t.Error("listed host refused")
	}
}
