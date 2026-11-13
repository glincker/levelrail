package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type fakeGitSourceGetter struct {
	err   error
	calls int
}

func (f *fakeGitSourceGetter) GetGitSource(context.Context, string) (apiclient.GitSourceResource, error) {
	f.calls++
	return apiclient.GitSourceResource{}, f.err
}

func noGitSource() error {
	return &apiclient.APIError{StatusCode: http.StatusNotFound, Message: "no git source connected for this app"}
}

func TestGitSourceHintLine(t *testing.T) {
	cases := []struct {
		remote, ref, want string
	}{
		{"git@github.com:acme/shop.git", "dev", "lr apps git-source set shop --repo https://github.com/acme/shop --branch dev"},
		{"https://x-access-token:ghs_secret@github.com/acme/shop.git", "", "lr apps git-source set shop --repo https://github.com/acme/shop --branch main"},
		{"ssh://git@gitlab.example.com:2222/group/sub/shop.git", "main", "lr apps git-source set shop --repo https://gitlab.example.com/group/sub/shop --branch main"},
		{"", "main", ""},
		{"/srv/git/shop.git", "main", ""},
		{"myalias:acme/shop", "main", ""},
	}
	for _, c := range cases {
		got := gitSourceHintLine("lr", "shop", detectedGit{RepoURL: c.remote, Ref: c.ref})
		if c.want == "" {
			if got != "" {
				t.Errorf("%q: got %q, want no hint", c.remote, got)
			}
			continue
		}
		if !strings.HasPrefix(got, "This app has no git source, so pushes will not deploy. Connect it: ") || !strings.HasSuffix(got, c.want) {
			t.Errorf("%q: got %q, want suffix %q", c.remote, got, c.want)
		}
		if strings.Contains(got, "ghs_secret") {
			t.Errorf("%q: hint leaked a credential: %q", c.remote, got)
		}
	}
}

func TestPrintGitSourceHint(t *testing.T) {
	git := detectedGit{RepoURL: "git@github.com:acme/shop.git", Ref: "main"}
	off := func(k string) (string, bool) {
		if k == envGitSourceHint {
			return "off", true
		}
		return "", false
	}
	cases := []struct {
		name      string
		err       error
		env       func(string) (string, bool)
		git       detectedGit
		wantHint  bool
		wantCalls int
	}{
		{"no git source", noGitSource(), envMap(), git, true, 1},
		{"connected", nil, envMap(), git, false, 1},
		{"server error is not proof", &apiclient.APIError{StatusCode: http.StatusInternalServerError}, envMap(), git, false, 1},
		{"network error is not proof", errors.New("dial tcp: refused"), envMap(), git, false, 1},
		{"suppressed by env", noGitSource(), off, git, false, 0},
		{"no remote", noGitSource(), envMap(), detectedGit{}, false, 0},
	}
	for _, c := range cases {
		f := &fakeGitSourceGetter{err: c.err}
		var buf bytes.Buffer
		printGitSourceHint(context.Background(), f, "lr", "shop", c.git, c.env, &buf)
		if got := strings.Contains(buf.String(), "has no git source"); got != c.wantHint {
			t.Errorf("%s: hint printed = %v, output %q", c.name, got, buf.String())
		}
		if f.calls != c.wantCalls {
			t.Errorf("%s: GetGitSource calls = %d, want %d", c.name, f.calls, c.wantCalls)
		}
	}
}

func TestUpPrintsGitSourceHintWhenUnconnected(t *testing.T) {
	_, srv := newFakeUp(t)
	dir := newUpProject(t)
	code, stdout, stderr := runUpCLI(t, srv, dir)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr:\n%s", code, stderr)
	}
	want := "Connect it: levelrail-cli-test apps git-source set " + dirName(dir) + " --repo https://github.com/acme/shop --branch main"
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr missing %q:\n%s", want, stderr)
	}
	if strings.Contains(stdout, "git source") {
		t.Errorf("hint went to stdout: %q", stdout)
	}

	var out, errb bytes.Buffer
	env := func(k string) (string, bool) {
		if k == envGitSourceHint {
			return "0", true
		}
		return "", false
	}
	code = run("levelrail-cli-test", []string{"up", dir, "--api-url", srv.URL, "--token", "t", "--yes"}, &out, &errb, env)
	if code != exitOK || strings.Contains(errb.String(), "has no git source") {
		t.Errorf("suppressed run: exit %d, stderr:\n%s", code, errb.String())
	}
}

func TestDeployUploadPrintsGitSourceHintFromCheckout(t *testing.T) {
	dir := writeDeployDir(t, map[string]string{"Dockerfile": "FROM nginx\nEXPOSE 8080\n", "index.html": "hi"})
	initRepo(t, dir, "git@github.com:acme/site.git")
	app := filepath.Base(dir)
	var mu sync.Mutex
	created := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/"+app:
			if !created {
				http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"name":"` + app + `","fallback_url":"https://app.example.test"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps":
			created = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"` + app + `"}`))
		case strings.HasSuffix(r.URL.Path, "/uploads"):
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"hash":"` + strings.Repeat("ab", 32) + `","size":10,"file_count":2}`))
		case strings.HasSuffix(r.URL.Path, "/builds"):
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"dep_1"}`))
		case strings.HasSuffix(r.URL.Path, "/deploys/dep_1"):
			_, _ = w.Write([]byte(`{"id":"dep_1","outcome":"healthy"}`))
		case strings.HasSuffix(r.URL.Path, "/git-source"):
			http.Error(w, `{"error":"no git source connected for this app"}`, http.StatusNotFound)
		default:
			http.Error(w, `{"error":"unexpected `+r.Method+" "+r.URL.Path+`"}`, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	args := []string{"deploy", dir, "--yes", "--api-url", srv.URL, "--token", "tok"}
	code, out, errOut := runDeployCLI(args, nil)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	want := "levelrail-cli-test apps git-source set " + app + " --repo https://github.com/acme/site --branch main"
	if !strings.Contains(errOut, want) || strings.Contains(out, "git source") {
		t.Errorf("want %q on stderr only; stdout %q stderr:\n%s", want, out, errOut)
	}
	if _, _, errOut = runDeployCLI(args, map[string]string{envGitSourceHint: "off"}); strings.Contains(errOut, "has no git source") {
		t.Errorf("hint not suppressed:\n%s", errOut)
	}
}
