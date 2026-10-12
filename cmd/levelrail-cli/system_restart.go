package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const defaultRestartWaitTimeout = 120 * time.Second

// restartPollInterval is a var so tests do not sleep for real seconds.
var restartPollInterval = 2 * time.Second

// runSystemRestart implements "system restart" and "system restart status":
// GET and POST /api/v1/system/restart (internal/api/system_restart.go).
func runSystemRestart(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 && args[0] == "status" {
		return runSystemRestartStatus(prog, args[1:], stdout, stderr, lookupEnv)
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "system restart", "print the restart state as JSON to stdout and nothing else", stderr)
	wait := fs.Bool("wait", false, "wait until the control plane answers again from a new process")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	timeout := fs.Duration("timeout", defaultRestartWaitTimeout, "how long --wait polls before giving up")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, systemRestartUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	cur, err := client.GetSystemRestart(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get restart state: %w", err))
	}
	if !cur.Supported {
		msg := "restart is not available: " + cur.Reason
		if cur.Hint != "" {
			msg += "\nrun this on the server instead: " + cur.Hint
		}
		return reportError(stdout, stderr, jsonOut, newValidationError("%s", msg))
	}
	if !*yes {
		if err := confirmRestart(stderr); err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
		}
	}

	posted, err := client.RestartControlPlane(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restart control plane: %w", err))
	}
	if !*wait {
		return renderRestart(stdout, stderr, of, posted, func() {
			_, _ = fmt.Fprintf(stderr, "restart requested; check progress with \"%s system restart status\"\n", prog)
		})
	}

	_, _ = fmt.Fprintln(stderr, "restart requested, waiting for the control plane to come back...")
	final, err := waitForRestart(ctx, client, posted.BootID, *timeout, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	return renderRestart(stdout, stderr, of, final, func() {
		_, _ = fmt.Fprintln(stderr, "control plane is back")
	})
}

func runSystemRestartStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "system restart status", "print the restart state as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, systemRestartUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	cur, err := client.GetSystemRestart(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get restart state: %w", err))
	}
	return renderRestart(stdout, stderr, of, cur, nil)
}

func renderRestart(stdout, stderr io.Writer, of outputFlags, r apiclient.SystemRestart, extra func()) int {
	if err := renderResult(stdout, of.Format, of.Query, r, func() {
		if extra != nil {
			extra()
		}
		printSystemRestartHuman(stdout, r)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printSystemRestartHuman(out io.Writer, r apiclient.SystemRestart) {
	_, _ = fmt.Fprintf(out, "boot id:   %s\nsupported: %t\npending:   %t\n", r.BootID, r.Supported, r.Pending)
	if r.Reason != "" {
		_, _ = fmt.Fprintf(out, "reason:    %s\n", r.Reason)
	}
	if r.Hint != "" {
		_, _ = fmt.Fprintf(out, "hint:      %s\n", r.Hint)
	}
}

func confirmRestart(stderr io.Writer) error {
	if f, ok := stdinSource.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // a terminal file descriptor always fits an int
		return errors.New("not a terminal: pass --yes to restart without confirmation")
	}
	_, _ = fmt.Fprint(stderr, "Restart the control plane? The dashboard and API are briefly unavailable. [y/N] ")
	line, _ := bufio.NewReader(stdinSource).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return errors.New("cancelled")
	}
	return nil
}

// waitForRestart polls until boot_id differs from oldBootID. Connection
// errors are expected while the process is down and are not fatal.
func waitForRestart(ctx context.Context, client *apiclient.Client, oldBootID string, timeout time.Duration, stderr io.Writer) (apiclient.SystemRestart, error) {
	deadline := time.Now().Add(timeout)
	down := false
	for {
		cur, err := client.GetSystemRestart(ctx)
		switch {
		case err != nil:
			if !down {
				_, _ = fmt.Fprintln(stderr, "control plane is down, waiting...")
				down = true
			}
		case cur.BootID != oldBootID:
			return cur, nil
		default:
			_, _ = fmt.Fprintln(stderr, "still shutting down...")
		}
		if time.Now().After(deadline) {
			return apiclient.SystemRestart{}, fmt.Errorf("control plane did not come back within %s", timeout)
		}
		time.Sleep(restartPollInterval)
	}
}

func systemRestartUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s system restart [--wait] [--yes] [--timeout DURATION] [flags]
  %[1]s system restart status [flags]

Restarts the control plane process through its supervisor. Asks for
confirmation unless --yes. With --wait, polls until a new process answers
(its boot id differs), tolerating connection errors while it is down.
If no supervisor was detected, prints the reason and the command to run
on the server, and exits non-zero.

Flags:
  --wait                   wait until the control plane is back
  --yes                    skip the confirmation prompt
  --timeout duration       how long --wait polls before giving up (default %[5]s)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string         control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string         named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                   print the restart state as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, defaultRestartWaitTimeout)
}
