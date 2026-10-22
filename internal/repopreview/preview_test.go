package repopreview

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/appwizard"
)

type fakeClient struct {
	meta  Meta
	err   error
	calls int
	token string
}

func (f *fakeClient) Fetch(_ context.Context, _ Target, token string) (Meta, error) {
	f.calls++
	f.token = token
	return f.meta, f.err
}

func TestBuildKinds(t *testing.T) {
	stars := 42
	good := Meta{Owner: "acme", Name: "web", DefaultBranch: "main", Branches: []string{"main", "dev"}, Stars: &stars,
		Languages:  map[string]int64{"Go": 75, "TypeScript": 25},
		LastCommit: &Commit{SHA: "0123456789abcdef", Message: "fix: thing\n\nlong body", Author: "a", At: time.Unix(0, 0)},
		Readme:     "# Title\n<script>alert(1)</script> hello [link](http://x.y) ![img](a.png)"}
	tests := []struct {
		name     string
		url      string
		token    string
		meta     Meta
		err      error
		wantKind Kind
		wantCall bool
	}{
		{"public", "https://github.com/acme/web", "", good, nil, KindPublic, true},
		{"private without account", "https://github.com/acme/secret", "", Meta{}, ErrNotFound, KindPrivate, true},
		{"missing with account", "https://github.com/acme/none", "tok", Meta{}, ErrNotFound, KindNotFound, true},
		{"rate limited", "https://gitlab.com/acme/web", "", Meta{}, ErrRateLimited, KindRateLimited, true},
		{"unknown host", "https://git.example.org/acme/web", "", good, nil, KindUnsupportedHost, false},
		{"known host without metadata api", "https://bitbucket.org/acme/web", "", Meta{}, ErrNoMetadata, KindUnsupportedHost, true},
		{"metadata link local", "http://169.254.169.254/latest/meta-data", "", good, nil, KindUnsupportedHost, false},
		{"localhost", "http://localhost:8080/acme/web", "", good, nil, KindUnsupportedHost, false},
		{"loopback ip", "https://127.0.0.1/acme/web", "", good, nil, KindUnsupportedHost, false},
		{"userinfo trick", "https://github.com@evil.test/acme/web", "", good, nil, KindUnsupportedHost, false},
		{"ssh scheme", "ssh://github.com/acme/web", "", good, nil, KindInvalidURL, false},
		{"no repo path", "https://github.com/acme", "", good, nil, KindInvalidURL, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakeClient{meta: tc.meta, err: tc.err}
			b := &Builder{Client: fc}
			res, err := b.Build(context.Background(), tc.url, "", tc.token)
			if err != nil {
				t.Fatalf("Build error: %v", err)
			}
			if res.Kind != tc.wantKind {
				t.Fatalf("kind = %q, want %q", res.Kind, tc.wantKind)
			}
			if (fc.calls > 0) != tc.wantCall {
				t.Fatalf("host called = %v, want %v", fc.calls > 0, tc.wantCall)
			}
		})
	}
}

func TestBuildPublicPayload(t *testing.T) {
	fc := &fakeClient{meta: Meta{Owner: "acme", Name: "web", DefaultBranch: "main",
		Languages:  map[string]int64{"Go": 3, "CSS": 1},
		LastCommit: &Commit{SHA: "0123456789abcdef", Message: "fix: thing\n\nbody"},
		Readme:     "# Title\n<script>alert(1)</script> hello [link](http://x.y)"}}
	called := ""
	b := &Builder{Client: fc, Analyze: func(_ context.Context, _, ref, _ string) (*appwizard.Analysis, error) {
		called = ref
		return &appwizard.Analysis{Branch: ref}, nil
	}}
	res, err := b.Build(context.Background(), "https://github.com/acme/web.git?x=1#frag", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if called != "main" || res.Analysis == nil {
		t.Fatalf("analysis should run on the default branch, got %q", called)
	}
	if res.RepoURL != "https://github.com/acme/web" {
		t.Fatalf("repo url not normalised: %q", res.RepoURL)
	}
	if res.LastCommit.SHA != "0123456" || res.LastCommit.Message != "fix: thing" {
		t.Fatalf("commit not shortened: %+v", res.LastCommit)
	}
	if res.Languages[0].Name != "Go" || res.Languages[0].Percent != 75 {
		t.Fatalf("languages: %+v", res.Languages)
	}
	if strings.ContainsAny(res.ReadmeExcerpt, "<>[]") || !strings.Contains(res.ReadmeExcerpt, "hello link") {
		t.Fatalf("readme not sanitised: %q", res.ReadmeExcerpt)
	}
}

func TestBuildExtraHostAndCache(t *testing.T) {
	fc := &fakeClient{meta: Meta{DefaultBranch: "main"}}
	now := time.Unix(1000, 0)
	b := &Builder{Client: fc, Now: func() time.Time { return now },
		Extra: func(context.Context) []string { return []string{"git.corp.example"} }}
	for range 2 {
		res, err := b.Build(context.Background(), "https://git.corp.example/team/app", "", "")
		if err != nil || res.Kind != KindPublic {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	}
	if fc.calls != 1 {
		t.Fatalf("second call should hit the cache, calls=%d", fc.calls)
	}
	now = now.Add(time.Hour)
	if _, err := b.Build(context.Background(), "https://git.corp.example/team/app", "", ""); err != nil || fc.calls != 2 {
		t.Fatalf("expired entry should refetch, calls=%d err=%v", fc.calls, err)
	}
}

func TestBuildHostFailureIsWrapped(t *testing.T) {
	b := &Builder{Client: &fakeClient{err: errors.New("boom")}}
	if _, err := b.Build(context.Background(), "https://github.com/a/b", "", ""); err == nil || !strings.Contains(err.Error(), "repopreview: fetch") {
		t.Fatalf("want wrapped error, got %v", err)
	}
}

func TestCacheIsolatedPerToken(t *testing.T) {
	fc := &fakeClient{meta: Meta{DefaultBranch: "main", Private: true}}
	b := &Builder{Client: fc}
	for _, tok := range []string{"good", "bad", "good"} {
		if _, err := b.Build(context.Background(), "https://github.com/a/b", "", tok); err != nil {
			t.Fatal(err)
		}
	}
	if fc.calls != 2 {
		t.Fatalf("each distinct token must fetch once, calls=%d", fc.calls)
	}
}

func TestBranchCapFromEnv(t *testing.T) {
	t.Setenv(EnvMaxBranches, "2")
	fc := &fakeClient{meta: Meta{Branches: []string{"a", "b", "c", "d"}}}
	res, _ := (&Builder{Client: fc}).Build(context.Background(), "https://github.com/a/b", "", "")
	if len(res.Branches) != 2 {
		t.Fatalf("branches = %v", res.Branches)
	}
}
