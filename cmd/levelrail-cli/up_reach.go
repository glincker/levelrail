package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// lsRemoteNoMatchExit is what git ls-remote --exit-code returns when no ref matches.
const lsRemoteNoMatchExit = 2

// repoReachabilityNote is the dry-run check that the repository up would build from answers
// without credentials. It runs with prompts and credential helpers off, so a failure usually
// means a private repo. Empty means reachable or not applicable.
func repoReachabilityNote(ctx context.Context, dir, prog string, inf upInference) string {
	remote := inf.Flags.repo
	if remote == "" || strings.HasPrefix(remote, "-") {
		return ""
	}
	if _, err := exec.LookPath("git"); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, pushCheckTimeout())
	defer cancel()
	_, err := pushGit(ctx, dir, "ls-remote", "--exit-code", "--heads", "--", remote)
	if err == nil {
		return ""
	}
	// The note reaches stderr and --json, so userinfo (a token in the URL) must not.
	shown := safeText(stripURLCredentials(remote))
	// Exit 2 means the remote answered but has no heads: reachable, just empty.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == lsRemoteNoMatchExit {
		return fmt.Sprintf("%s is reachable but has no branches yet, so there is nothing to build; push a commit first", shown)
	}
	return fmt.Sprintf("%s did not answer without credentials, so it may be private. The server needs access to build it: connect the provider (\"%s git-providers\") or add a deploy key to the repo, or skip git and upload the files with \"%s deploy %s\"",
		shown, prog, prog, safeText(dir))
}
