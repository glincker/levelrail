package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type gitRemoteResource = apiclient.GitRemote

func runGit(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, gitUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, gitUsage(prog))
		return exitOK
	case "remote":
		return runGitRemote(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown git subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, gitUsage(prog))
		return exitUsage
	}
}

func gitUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s git remote add <app> [flags]            enable git push deploy and print the exact remote line (token shown once); --apply sets it in the current repo
  %[1]s git remote show <app> [flags]           show the remote URL, deploy branch and last push
  %[1]s git remote rotate <app> [flags]         issue a new token, the old one stops working at once; --apply updates the repo's remote
  %[1]s git remote branch <app> <branch> [flags]   choose the branch that deploys (default main)
  %[1]s git remote revoke <app> [flags]         turn the remote off and invalidate its token

Run "%[1]s git remote <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runGitRemote(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, gitUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, gitUsage(prog))
		return exitOK
	case "add":
		return runGitRemoteIssue(prog, "add", args[1:], stdout, stderr, lookupEnv)
	case "rotate":
		return runGitRemoteIssue(prog, "rotate", args[1:], stdout, stderr, lookupEnv)
	case "show":
		return runGitRemoteShow(prog, args[1:], stdout, stderr, lookupEnv)
	case "branch":
		return runGitRemoteBranch(prog, args[1:], stdout, stderr, lookupEnv)
	case "revoke":
		return runGitRemoteRevoke(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown git remote subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, gitUsage(prog))
		return exitUsage
	}
}

func printGitRemoteHuman(stdout io.Writer, g gitRemoteResource) {
	_, _ = fmt.Fprintf(stdout, "url:           %s\nbranch:        %s\nmax push:      %d MiB\n", g.URL, g.Branch, g.MaxPushBytes>>20)
	if g.LastPushAt != nil {
		_, _ = fmt.Fprintf(stdout, "last push:     %s (%s, %s)\n", g.LastPushAt.Format("2006-01-02 15:04:05"), shortCommit(g.LastPushCommit), g.LastPushResult)
	}
}

func shortCommit(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

func runGitRemoteIssue(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "git remote " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the remote, including the one-time token, as JSON to stdout and nothing else", stderr)
	applyFlag := fs.Bool("apply", false, "set the remote in a local git repository (add, or set-url if it exists); the token URL is then kept only in .git/config and not printed")
	dirFlag := fs.String("dir", "", "repository directory for --apply (default: the current directory)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s <app> [flags]\n\nIssues the push token and prints the exact git remote add line. The token is\nshown once. Pushing the deploy branch builds and deploys that commit.\nWith --apply the remote is configured in the repository for you.\n\nFlags:\n", prog, label)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, label, "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	ctx := context.Background()
	if verb == "add" {
		cur, err := client.GetGitRemote(ctx, name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("check git remote for app %q: %w", name, err))
		}
		if cur.Configured {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("app %q already has a git remote: use %q to issue a new token", name, prog+" git remote rotate "+name))
		}
	}
	g, err := client.IssueGitRemote(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("issue git remote token for app %q: %w", name, err))
	}
	var applyErr error
	var appliedDir string
	if *applyFlag {
		appliedDir, applyErr = applyGitRemote(ctx, *dirFlag, g.RemoteName, g.URL, g.Username, g.Token)
	}
	if err := renderResult(stdout, of.Format, of.Query, g, func() {
		printGitRemoteHuman(stdout, g)
		if *applyFlag && applyErr == nil {
			_, _ = fmt.Fprintf(stdout, "\nremote %s configured in %s\n\nDeploy with:\n  git push %s %s\n", g.RemoteName, appliedDir, g.RemoteName, g.Branch)
			return
		}
		_, _ = fmt.Fprintf(stdout, "\nRun this in your repository (the token is shown once):\n  %s\n\nThen deploy with:\n  git push %s %s\n", g.AddCommand, g.RemoteName, g.Branch)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if applyErr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: --apply failed: %v\n", prog, applyErr)
		return exitValidation
	}
	return exitOK
}

var gitRemoteNameRe = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9_.-]{0,63}$`)

// applyGitRemote sets the push remote with git in argv form. Errors never carry
// the URL or token: git output is dropped because it can echo the URL.
func applyGitRemote(ctx context.Context, dir, name, rawURL, username, token string) (string, error) {
	if !gitRemoteNameRe.MatchString(name) || name == "." || name == ".." {
		return "", errors.New("invalid remote name from the server")
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("invalid remote URL from the server")
	}
	if token != "" {
		if username == "" {
			username = "git"
		}
		u.User = url.UserPassword(username, token)
	}
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", errors.New("cannot resolve the repository directory")
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return "", errors.New("git was not found in PATH")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	inside := exec.CommandContext(ctx, gitBin, "-C", abs, "rev-parse", "--is-inside-work-tree") //nolint:gosec // argv form, literal git, validated name and URL
	out, err := inside.Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return "", fmt.Errorf("%s is not inside a git work tree", abs)
	}
	verb := "add"
	cur, curErr := exec.CommandContext(ctx, gitBin, "-C", abs, "remote", "get-url", "--", name).Output() //nolint:gosec // argv form, literal git, validated name and URL
	if curErr == nil {
		// Only rotate a remote that already points at this server and path; never repoint an unrelated one.
		cu, perr := url.Parse(strings.TrimSpace(string(cur)))
		if perr != nil || cu.Host != u.Host || cu.Path != u.Path {
			return "", fmt.Errorf("remote %q already exists and points elsewhere; remove it or pick another name first", name)
		}
		verb = "set-url"
	}
	if err := exec.CommandContext(ctx, gitBin, "-C", abs, "remote", verb, "--", name, u.String()).Run(); err != nil { //nolint:gosec // argv form, literal git, validated name and URL
		return "", fmt.Errorf("git remote %s failed", verb)
	}
	return abs, nil
}

func runGitRemoteShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "git remote show", "print the remote as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s git remote show <app> [flags]\n\nShows the remote URL, deploy branch and the last push. The token is never\nshown again: rotate it to get a new one.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "git remote show", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	g, err := client.GetGitRemote(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get git remote for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, g, func() {
		if !g.Configured {
			_, _ = fmt.Fprintf(stdout, "git push deploy is not enabled for app %q.\nEnable it with: %s git remote add %s\n", name, prog, name)
			return
		}
		printGitRemoteHuman(stdout, g)
		_, _ = fmt.Fprintf(stdout, "\ngit remote add %s %s\n(use the token from add or rotate as the password; user name %q)\n", g.RemoteName, g.URL, g.Username)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runGitRemoteBranch(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "git remote branch", "print the remote as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s git remote branch <app> <branch> [flags]\n\nChooses which pushed branch deploys. Other branches are rejected with a message.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, branch, jsonOut, of, exitCode, ok := parseTwoArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, twoArgCmd{prog, "git remote branch", "<app> <branch>"}, lookupEnv)
	if !ok {
		return exitCode
	}
	g, err := client.SetGitRemoteBranch(context.Background(), name, branch)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set git remote branch for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, g, func() {
		_, _ = fmt.Fprintf(stdout, "app %q now deploys the %q branch\n", name, g.Branch)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runGitRemoteRevoke(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "git remote revoke", "print {\"revoked\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s git remote revoke <app> [flags]\n\nTurns the push remote off. Its token stops working at once.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "git remote revoke", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if err := client.DeleteGitRemote(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke git remote for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"revoked": true}, func() {
		_, _ = fmt.Fprintf(stdout, "git push deploy turned off for app %q\n", name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
