package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/sourcecache"
)

// Ref listings of an ssh repo with no deploy key must fail before go-git can
// pick up the control plane's own ssh identity.
func TestRemoteRefResolversNeedDeployKeyForSSH(t *testing.T) {
	ctx := context.Background()
	for _, url := range []string{"ssh://git@10.0.0.5:6379/x", "git@example.com:o/r.git"} {
		if _, err := resolveRemoteBranchSHA(ctx, url, "main", "tok"); !errors.Is(err, sourcecache.ErrSSHAuthRequired) {
			t.Errorf("branch %s: err = %v, want ErrSSHAuthRequired", url, err)
		}
		if _, err := resolveRemoteTagSHA(ctx, url, "v1", ""); !errors.Is(err, sourcecache.ErrSSHAuthRequired) {
			t.Errorf("tag %s: err = %v, want ErrSSHAuthRequired", url, err)
		}
	}
	rt := &Router{}
	if _, err := resolveRemoteBranchSHA(rt.gitAuthCtx(ctx, "web", "git@example.com:o/r.git"), "git@example.com:o/r.git", "main", ""); !errors.Is(err, sourcecache.ErrSSHAuthRequired) {
		t.Errorf("app without a stored key: err = %v, want ErrSSHAuthRequired", err)
	}
}
