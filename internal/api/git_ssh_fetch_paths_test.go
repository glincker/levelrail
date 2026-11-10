package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/sourcecache"
)

// Both one-off remote listings must stop before any transport for an ssh URL
// that has no deploy key, so the host's own ssh identity is never used.
func TestRemoteRefResolvers_SSHURLWithoutKeyFailClosed(t *testing.T) {
	urls := []string{"ssh://git@example.com/org/repo.git", "git@example.com:org/repo.git"}
	for _, u := range urls {
		if _, err := resolveRemoteBranchSHA(context.Background(), u, "main", ""); !errors.Is(err, sourcecache.ErrSSHAuthRequired) {
			t.Errorf("branch %q: err = %v, want ErrSSHAuthRequired", u, err)
		}
		if _, err := resolveRemoteTagSHA(context.Background(), u, "v1", ""); !errors.Is(err, sourcecache.ErrSSHAuthRequired) {
			t.Errorf("tag %q: err = %v, want ErrSSHAuthRequired", u, err)
		}
	}
}
