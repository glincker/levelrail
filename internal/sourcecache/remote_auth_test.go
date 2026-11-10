package sourcecache

import (
	"context"
	"errors"
	"testing"
)

func TestRemoteAuth_SSHURLsRequireDeployKey(t *testing.T) {
	urls := []string{
		"ssh://git@example.com/org/repo.git",
		"git@example.com:org/repo.git",
		"ssh://example.com:2222/org/repo",
	}
	for _, u := range urls {
		for _, token := range []string{"", "tok"} {
			auth, err := RemoteAuth(context.Background(), u, token)
			if !errors.Is(err, ErrSSHAuthRequired) {
				t.Errorf("%q token=%q: err = %v, want ErrSSHAuthRequired", u, token, err)
			}
			if auth != nil {
				t.Errorf("%q token=%q: auth must be nil without a deploy key", u, token)
			}
		}
	}
}
