package sourcecache

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrSparseUnavailable tells the caller to use the full mirror path instead.
var ErrSparseUnavailable = errors.New("sourcecache: sparse checkout unavailable")

// sparseProtocols is what the git binary may speak; tests widen it to file.
var sparseProtocols = "https:http"

// SparseCheckout fetches one branch at depth 1 without blobs and checks out
// only subPath, so a large repo costs a few files instead of its whole history.
func (c *Cache) SparseCheckout(ctx context.Context, repoURL, branch, subPath, token string, maxBytes int64) (Checkout, error) {
	gitBin, err := exec.LookPath("git")
	if err != nil || strings.HasPrefix(repoURL, "-") || strings.HasPrefix(branch, "-") || strings.HasPrefix(subPath, "-") {
		return Checkout{}, ErrSparseUnavailable
	}
	dest, err := os.MkdirTemp("", "levelrail-sparse-*")
	if err != nil {
		return Checkout{}, fmt.Errorf("sourcecache: create checkout dir: %w", err)
	}
	fail := func(err error) (Checkout, error) {
		_ = os.RemoveAll(dest)
		return Checkout{}, err
	}
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL="+sparseProtocols, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if token != "" {
		basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		env = append(env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.extraHeader", "GIT_CONFIG_VALUE_0=Authorization: Basic "+basic)
	}
	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, gitBin, args...) //nolint:gosec // fixed binary, user values come after --
		cmd.Env, cmd.Dir = env, dest
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	args := []string{"clone", "--depth", "1", "--filter=blob:none", "--sparse", "--no-tags", "--single-branch"}
	if branch != "" {
		args = append(args, "--branch="+branch)
	}
	if _, err := run(append(args, "--", repoURL, ".")...); err != nil {
		return fail(fmt.Errorf("%w: clone: %w", ErrSparseUnavailable, err))
	}
	if subPath != "" {
		if _, err := run("sparse-checkout", "set", "--", subPath); err != nil {
			return fail(fmt.Errorf("%w: sparse-checkout: %w", ErrSparseUnavailable, err))
		}
	}
	commit, err := run("rev-parse", "HEAD")
	if err != nil {
		return fail(fmt.Errorf("%w: rev-parse: %w", ErrSparseUnavailable, err))
	}
	if over, _ := dirOver(dest, maxBytes); over {
		return fail(ErrOverBudget)
	}
	return Checkout{Dir: dest, Commit: commit, cleanup: func() { _ = os.RemoveAll(dest) }}, nil
}

func dirOver(root string, limit int64) (bool, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if total += fi.Size(); limit > 0 && total > limit {
			return ErrOverBudget
		}
		return nil
	})
	return errors.Is(err, ErrOverBudget), err
}
