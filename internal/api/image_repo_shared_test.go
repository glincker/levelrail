package api

import (
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func TestImageRepoOwnerRejectsSharedRepo(t *testing.T) {
	all := []store.DesiredService{{Name: "web", Image: "web:" + hex40}, {Name: "api", Image: "nginx:latest"}}
	if got := imageRepoOwner(all, "other", "web"); got != "web" {
		t.Fatalf("owner = %q, want web", got)
	}
	if got := imageRepoOwner(all, "web", "web"); got != "" {
		t.Fatalf("an app may reuse its own repo, got %q", got)
	}
	if got := imageRepoOwner(all, "other", "fresh"); got != "" {
		t.Fatalf("unused repo flagged: %q", got)
	}
}

const hex40 = "0123456789abcdef0123456789abcdef01234567"

func TestImageRepoOwnerDerivedAndNormalized(t *testing.T) {
	all := []store.DesiredService{{Name: "web", Image: "web:" + hex40}}
	for _, repo := range []string{"web", "library/web", "docker.io/web", "docker.io/library/web", "WEB"} {
		if got := imageRepoOwner(all, "other", repo); got != "web" {
			t.Fatalf("repo %q owner = %q, want web", repo, got)
		}
	}
	// An app whose image is a 40 hex tag on another app's repo derives that repo.
	bypass := []store.DesiredService{{Name: "victim", Image: "docker.io/library/victim:" + hex40}}
	if got := imageRepoOwner(bypass, "attacker", normalizeImageRepo(defaultImageRepo("attacker", "victim:"+hex40))); got != "victim" {
		t.Fatalf("derived repo bypass owner = %q", got)
	}
}

func TestImageRepoClaimConcurrent(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var wg sync.WaitGroup
	owners := make([]string, 8)
	for i := range owners {
		wg.Add(1)
		go func() {
			defer wg.Done()
			owners[i], _ = db.ClaimImageRepo(ctx, normalizeImageRepo("web"), "app"+strconv.Itoa(i))
		}()
	}
	wg.Wait()
	for _, o := range owners {
		if o == "" || o != owners[0] {
			t.Fatalf("claims disagree: %v", owners)
		}
	}
	if err := db.ReleaseImageRepos(ctx, owners[0]); err != nil {
		t.Fatal(err)
	}
	if o, _ := db.ImageRepoOwner(ctx, normalizeImageRepo("web")); o != "" {
		t.Fatalf("repo not released: %q", o)
	}
}
