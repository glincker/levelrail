package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	envPushCheckTimeoutSec     = "LEVELRAIL_PUSH_CHECK_TIMEOUT_SEC"
	defaultPushCheckTimeoutSec = 15
)

// pushCheckTimeout bounds every git call so a hung remote cannot stall up.
func pushCheckTimeout() time.Duration {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(envPushCheckTimeoutSec))); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return defaultPushCheckTimeoutSec * time.Second
}

// safeText drops control characters so a hostile remote or branch cannot forge terminal output.
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

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
	// A leading dash would be read by git as an option (--upload-pack=...).
	if strings.HasPrefix(remote, "-") || strings.HasPrefix(branch, "-") {
		return pushCheck{Warning: "the repository URL or branch starts with a dash, so up cannot check that the remote has your commit"}
	}
	ctx, cancel := context.WithTimeout(ctx, pushCheckTimeout())
	defer cancel()
	head, err := pushGit(ctx, dir, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return pushCheck{Warning: "the repository has no commits yet, so up cannot check that the remote has your commit"}
	}
	// Only the directory's own branch on its own remote says anything about HEAD.
	compareHead := branch == inf.Git.Ref && inf.Sources["repo"] == srcGit

	out, err := pushGit(ctx, dir, "ls-remote", "--heads", "--", remote, "refs/heads/"+branch)
	if err != nil {
		return pushCheck{Warning: fmt.Sprintf("could not reach %s to check the branch (%v)", safeText(remote), err)}
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
	return fmt.Sprintf("branch %q %s (local HEAD %s), so the server would build nothing or stale code.\nPush it first: git push -u origin %s\nOr build what the remote has anyway with --allow-unpushed", safeText(branch), why, short, safeText(branch))
}

func pushGit(ctx context.Context, dir string, args ...string) (string, error) {
	// Config overrides neutralise hostile repo settings that run commands. git:// is off the
	// allowlist because the first core.gitProxy git reads (the repo's) wins over -c.
	full := append([]string{"-C", dir, "-c", "core.fsmonitor=false", "-c", "core.sshCommand=ssh", "-c", "core.gitProxy=",
		"-c", "credential.helper=", "-c", "core.askPass=", "-c", "protocol.ext.allow=never"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...) //nolint:gosec // fixed git subcommands, validated operands, no shell
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PROXY_COMMAND=", "GIT_ALLOW_PROTOCOL=https:ssh:file")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}
