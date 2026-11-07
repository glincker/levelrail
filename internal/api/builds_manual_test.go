package api

import (
	"context"
	"testing"
)

func TestDefaultImageRepo(t *testing.T) {
	const sha = "fe49a4b97c2100c26188d2845a386c58e9610350"
	tests := []struct {
		name    string
		current string
		want    string
	}{
		{"pending placeholder", "dogfood/web:pending", "dogfood/web"},
		{"commit sha tag", "dogfood/web:" + sha, "dogfood/web"},
		{"commit sha tag pinned by digest", "dogfood/web:" + sha + "@sha256:0b6d", "dogfood/web"},
		{"registry port kept", "host:5000/org/web:" + sha, "host:5000/org/web"},
		{"registry image falls back to app name", "nginx:1.27", "web"},
		{"untagged falls back to app name", "nginx", "web"},
		{"empty falls back to app name", "", "web"},
		{"short sha is not a build tag", "org/web:abc123", "web"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultImageRepo("web", tt.current); got != tt.want {
				t.Errorf("defaultImageRepo(%q) = %q, want %q", tt.current, got, tt.want)
			}
		})
	}
}

func TestManualBuild_KeepsDomainAttachedDuringBuild(t *testing.T) {
	t.Parallel()
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fb.release = make(chan struct{})
	fetch := newFakeFetch(t.TempDir(), nil)
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithBuilder(fb))
	rt.fetch = fetch.fetch
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	_, resp := postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://example.com/web.git","ref":"main"}`)
	req := <-fb.notify

	stored, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	cur := *stored
	cur.Domains = []string{"web.example.com"}
	if err := db.SaveDesiredService(context.Background(), cur); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	// The pipeline saves from a snapshot taken at trigger time, with no domains.
	snapshot := cur
	snapshot.Domains = nil
	if req.RefreshDesired == nil {
		t.Fatal("RefreshDesired = nil, want a hook that keeps stored domains")
	}
	if err := req.RefreshDesired(context.Background(), &snapshot); err != nil {
		t.Fatalf("RefreshDesired() error = %v", err)
	}
	if len(snapshot.Domains) != 1 || snapshot.Domains[0] != "web.example.com" {
		t.Errorf("Domains = %v, want [web.example.com]", snapshot.Domains)
	}
	close(fb.release)
	awaitDeployAttemptFinished(t, db, resp.ID)
}
