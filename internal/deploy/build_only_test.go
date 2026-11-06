package deploy

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
)

func TestBuildOnlyRejectsDockerfileOutsideSource(t *testing.T) {
	p := New(nil, nil)
	_, err := p.BuildOnly(context.Background(), BuildOnlyRequest{
		SourceDir:  t.TempDir(),
		Type:       "dockerfile",
		Dockerfile: "../../outside/Dockerfile",
		Tag:        "x:1",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "dockerfile") {
		t.Fatalf("err = %v, want a dockerfile containment error", err)
	}
}

func TestRailpackCacheIDFromAppName(t *testing.T) {
	cases := []struct {
		name, service, repo, want string
	}{
		{"plain name", "web", "levelrail/web", "web"},
		{"sanitized", "My_App.v2", "levelrail/other", "my-app-v2"},
		{"empty name falls back to repo", "", "levelrail/shop", "shop"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &fakeBuilder{railpackResult: &build.Result{Tag: tc.repo + ":abc"}}
			p := New(b, &fakeServiceStore{})
			_, _ = p.Deploy(context.Background(), Request{
				ServiceName: tc.service, Service: railpackService(), SourceDir: t.TempDir(), CommitSHA: "abc", ImageRepo: tc.repo,
			}, nil)
			if got := b.lastRailpackReq.CacheID; got != tc.want {
				t.Fatalf("CacheID = %q, want %q", got, tc.want)
			}
		})
	}
}
