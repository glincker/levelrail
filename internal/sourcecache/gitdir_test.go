package sourcecache

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
)

func cleanGit(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:gosec // test
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GIT_") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	return cmd
}

func TestCheckoutHasWorkingGitDir(t *testing.T) {
	h := newFakeHost(t)
	sha := h.commit("one")
	c, _, _ := newTestCache(t)
	for _, name := range []string{"freshly fetched", "from cache"} {
		co, err := c.Checkout(context.Background(), h.url(), sha, "")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if co.FromCache != (name == "from cache") {
			t.Fatalf("%s: FromCache = %v", name, co.FromCache)
		}
		repo, err := git.PlainOpen(co.Dir)
		if err != nil {
			t.Fatalf("%s: no .git: %v", name, err)
		}
		head, err := repo.Head()
		if err != nil || head.Hash().String() != sha {
			t.Fatalf("%s: HEAD = %v, %v, want %s", name, head, err, sha)
		}
		out, err := cleanGit(co.Dir, "rev-parse", "HEAD").Output()
		if err != nil || strings.TrimSpace(string(out)) != sha {
			t.Fatalf("%s: git rev-parse = %q, %v", name, out, err)
		}
		if err := cleanGit(co.Dir, "log", "-1").Run(); err != nil {
			t.Fatalf("%s: git log: %v", name, err)
		}
		co.Cleanup()
	}
}
