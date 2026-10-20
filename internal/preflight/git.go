package preflight

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Git check errors a check maps to a status.
var (
	ErrGitAuth          = errors.New("repository requires credentials")
	ErrGitBranchMissing = errors.New("branch not found")
	ErrGitUnverifiable  = errors.New("url scheme cannot be checked")
)

// GitChecker verifies a repository is reachable and has a branch.
type GitChecker interface {
	Check(ctx context.Context, repoURL, branch string) error
}

const maxRefsBytes = 4 << 20

// SmartHTTPChecker lists refs over git's smart HTTP protocol, so no git
// binary is spawned. Only http(s) URLs can be checked.
type SmartHTTPChecker struct {
	Client *http.Client
}

// Check implements GitChecker.
func (c *SmartHTTPChecker) Check(ctx context.Context, repoURL, branch string) error {
	if !strings.HasPrefix(repoURL, "https://") && !strings.HasPrefix(repoURL, "http://") {
		return ErrGitUnverifiable
	}
	u := strings.TrimSuffix(repoURL, "/") + "/info/refs?service=git-upload-pack"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("preflight: build git request: %w", err)
	}
	req.Header.Set("User-Agent", "git/2.40")
	resp, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("preflight: reach repository: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return ErrGitAuth
	default:
		return fmt.Errorf("preflight: repository answered %d", resp.StatusCode)
	}
	if branch == "" {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRefsBytes))
	if err != nil {
		return fmt.Errorf("preflight: read refs: %w", err)
	}
	if !hasBranch(string(body), branch) {
		return ErrGitBranchMissing
	}
	return nil
}

// hasBranch scans the advertised refs for refs/heads/<branch> as a whole
// ref name (followed by a newline, NUL or end).
func hasBranch(refs, branch string) bool {
	needle := "refs/heads/" + branch
	for i := 0; ; {
		j := strings.Index(refs[i:], needle)
		if j < 0 {
			return false
		}
		end := i + j + len(needle)
		if end >= len(refs) || refs[end] == '\n' || refs[end] == 0 {
			return true
		}
		i = end
	}
}
