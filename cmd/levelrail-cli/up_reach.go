package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

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
	if _, err := pushGit(ctx, dir, "ls-remote", "--exit-code", "--heads", "--", remote); err == nil {
		return ""
	}
	return fmt.Sprintf("%s did not answer without credentials, so it may be private. The server needs access to build it: connect the provider (\"%s git-providers\") or add a deploy key to the repo, or skip git and upload the files with \"%s deploy %s\"",
		safeText(remote), prog, prog, safeText(dir))
}
