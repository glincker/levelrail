package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"time"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/tui"
)

const (
	envTUIRefresh   = "APP_TUI_REFRESH"
	envTUILogBuffer = "APP_TUI_LOG_BUFFER"
	minTUIRefresh   = 2 * time.Second
)

// tuiInteractive reports whether stdin and stdout are both terminals; a
// variable so tests can force either answer.
var tuiInteractive = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) //nolint:gosec // a terminal file descriptor always fits an int
}

// tuiRun is the program launcher, replaced in tests.
var tuiRun = func(ctx context.Context, api tui.API, opts tui.Options) error {
	return tui.Run(ctx, api, opts, os.Stdin, os.Stdout)
}

// runTUI implements "tui": the interactive terminal UI over the same API
// and credentials every other command uses.
func runTUI(prog string, args []string, _, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" tui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	token := fs.String("token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	apiURL := fs.String("api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file)")
	profile := fs.String("profile", "", "named credentials profile to use (overrides "+envProfile+")")
	readOnly := fs.Bool("read-only", false, "disable every action that changes state")
	refresh := fs.Duration("refresh", 0, "refresh interval, minimum 2s (default 2s, or "+envTUIRefresh+")")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tuiUsage(prog)) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "%s tui: unexpected argument %q\n\n%s", prog, fs.Arg(0), tuiUsage(prog))
		return exitUsage
	}
	if !tuiInteractive() {
		_, _ = fmt.Fprintf(stderr, "%s tui needs an interactive terminal (stdin and stdout must be a TTY). Use '%s apps list' or '%s attention' in scripts.\n", prog, prog, prog)
		return exitUsage
	}

	client := apiClientFromFlags(prog, *apiURL, *token, *profile, lookupEnv)
	prof := resolveProfile(*profile, lookupEnv)
	opts := tui.Options{
		Prog:      prog,
		Profile:   prof,
		Host:      tuiHost(resolveAPIURL(*apiURL, lookupEnv, prog, prof)),
		ReadOnly:  *readOnly,
		Color:     tuiColor(lookupEnv),
		Refresh:   tuiRefresh(*refresh, lookupEnv),
		LogBuffer: tuiIntEnv(lookupEnv, envTUILogBuffer),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	api := tui.LiveAPI{Client: client, Timeout: 10 * time.Second}
	if err := tuiRun(ctx, api, opts); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s tui: %v\n", prog, err)
		return exitNetwork
	}
	return exitOK
}

func tuiColor(lookupEnv func(string) (string, bool)) bool {
	if v, ok := lookupEnv("NO_COLOR"); ok && v != "" {
		return false
	}
	if v, ok := lookupEnv("TERM"); ok && v == "dumb" {
		return false
	}
	return true
}

func tuiRefresh(flagVal time.Duration, lookupEnv func(string) (string, bool)) time.Duration {
	d := flagVal
	if d == 0 {
		if v, ok := lookupEnv(envTUIRefresh); ok {
			if parsed, err := time.ParseDuration(v); err == nil {
				d = parsed
			}
		}
	}
	if d < minTUIRefresh {
		return minTUIRefresh
	}
	return d
}

func tuiIntEnv(lookupEnv func(string) (string, bool), key string) int {
	if v, ok := lookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// tuiHost keeps only scheme-less host:port, never userinfo or path.
func tuiHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

func tuiUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tui [flags]

Interactive terminal UI: live apps, databases, deployments, nodes,
domains, alerts and backups, with logs tail and confirm-gated actions.
Secrets and env values are never shown; use the dashboard to reveal them.

Keys: j/k move, enter open, / filter, : command bar (deploy, restart,
rollback, stop, start, logs), l logs, 1-7 or tab switch view, ? help, q quit.

Flags:
  --read-only             disable restart, deploy, rollback, stop and start
  --refresh duration      refresh interval, minimum 2s (default 2s, or %[2]s)
  --token string          API token (default: %[3]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[4]s env var, then %[5]s)
  --profile string        named credentials profile to use (overrides APP_PROFILE)
  -h, --help              show this help

Set NO_COLOR=1 to disable colours. Log buffer size: %[6]s (default 2000 lines).
`, prog, envTUIRefresh, envAPIToken, envAPIURL, defaultAPIURL, envTUILogBuffer)
}
