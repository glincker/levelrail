package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
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
