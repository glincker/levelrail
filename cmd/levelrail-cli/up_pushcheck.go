package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// pushCheck is the verdict of comparing the local HEAD with the remote branch the server will build.
type pushCheck struct {
	// Problem is set when the remote provably lacks the commit; Warning when the check could not run.
	Problem string
	Warning string
}

// checkPushed asks the remote itself (git ls-remote), because a local remote-tracking ref can be stale.
func checkPushed(ctx context.Context, dir string, inf upInference, explicitRef bool) pushCheck {
	if _, err := exec.LookPath("git"); err != nil {
		return pushCheck{Warning: "git is not installed, so up cannot check that the remote has your commit"}
	}
	branch := inf.Flags.ref
	if inf.Git.Ref == "" && !explicitRef {
		return pushCheck{Warning: "HEAD is detached, so up cannot check that the remote has your commit"}
	}
	remote := inf.Flags.repo
	if remote == "" {
		return pushCheck{}
	}
	head, err := pushGit(ctx, dir, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return pushCheck{Warning: "the repository has no commits yet, so up cannot check that the remote has your commit"}
	}
	// Only the directory's own branch on its own remote says anything about HEAD.
	compareHead := branch == inf.Git.Ref && inf.Sources["repo"] == srcGit

	out, err := pushGit(ctx, dir, "ls-remote", "--heads", remote, "refs/heads/"+branch)
	if err != nil {
		return pushCheck{Warning: fmt.Sprintf("could not reach %s to check the branch (%v)", remote, err)}
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return pushCheck{Problem: unpushedMessage(branch, head, "does not exist on the remote")}
	}
	remoteSHA := fields[0]
	if !compareHead || remoteSHA == head {
		return pushCheck{}
	}
	_, err = pushGit(ctx, dir, "merge-base", "--is-ancestor", head, remoteSHA)
	if err == nil {
		return pushCheck{}
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return pushCheck{Problem: unpushedMessage(branch, head, "on the remote does not include your latest commit")}
	}
	return pushCheck{Warning: "the remote branch has commits this clone has not fetched, so up cannot tell whether yours is included (git fetch, then retry)"}
}

func unpushedMessage(branch, head, why string) string {
	short := head
	if len(short) > 7 {
		short = short[:7]
	}
	return fmt.Sprintf("branch %q %s (local HEAD %s), so the server would build nothing or stale code.\nPush it first: git push -u origin %s\nOr build what the remote has anyway with --allow-unpushed", branch, why, short, branch)
}

func pushGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // fixed git subcommands, no shell
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}
