package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsDeploysGate implements "apps deploys gate <name> [deploy-id]" and
// its "deploy status" alias.
func runAppsDeploysGate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys gate", "print the gate timeline as JSON to stdout and nothing else", stderr)
	var watch bool
	var intervalStr string
	fs.BoolVar(&watch, "watch", false, "keep printing new steps until the release clears its watch, fails or is superseded")
	fs.StringVar(&intervalStr, "interval", "2s", "how often --watch re-checks")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysGateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps deploys gate requires an app name and an optional deploy id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, deployID := rest[0], "latest"
	if len(rest) == 2 {
		deployID = rest[1]
	}
	interval, err := parseDurationOrZero(intervalStr)
	if err != nil || interval <= 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--interval must be a valid positive duration, got %q", intervalStr))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	gate, err := client.GetDeployGate(ctx, name, deployID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get deploy gate %q for app %q: %w", deployID, name, err))
	}
	if !watch {
		printWithScan := func() {
			printDeployGate(stdout, gate, 0)
			if scan, serr := client.GetDeploymentSecretScan(ctx, name, gate.DeployID); serr == nil && scan.Action != "allow" {
				_, _ = fmt.Fprintln(stdout, "secret scan:")
				printSecretScanRun(stdout, scan)
			}
		}
		if code := writeScheduledTaskResult(stdout, stderr, of, gate, printWithScan); code != exitOK {
			return code
		}
		return gateExitCode(gate)
	}
	return followDeployGate(ctx, client, name, gate, interval, jsonOut, stdout, stderr)
}

// followDeployGate prints gate, then every new step, until it is terminal.
func followDeployGate(ctx context.Context, client *Client, name string, gate apiclient.DeployGateResource, interval time.Duration, jsonOut bool, stdout, stderr io.Writer) int {
	printed := 0
	for {
		if jsonOut {
			for _, e := range gate.Events[printed:] {
				if err := writeJSONLine(stdout, e); err != nil {
					return reportError(stdout, stderr, jsonOut, err)
				}
			}
		} else {
			printDeployGate(stdout, gate, printed)
		}
		printed = len(gate.Events)
		if gate.Terminal {
			return gateExitCode(gate)
		}
		select {
		case <-ctx.Done():
			return exitOK
		case <-time.After(interval):
		}
		next, err := client.GetDeployGate(ctx, name, gate.DeployID)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get deploy gate %q for app %q: %w", gate.DeployID, name, err))
		}
		gate = next
	}
}

func gateExitCode(g apiclient.DeployGateResource) int {
	if g.Failed {
		return exitDeployNotHealthy
	}
	return exitOK
}

// printDeployGate renders the header once (from == 0) and the steps after from.
func printDeployGate(w io.Writer, g apiclient.DeployGateResource, from int) {
	if from == 0 {
		_, _ = fmt.Fprintf(w, "deploy:   %s\nimage:    %s\nstage:    %s\n", g.DeployID, unpinnedImage(g.Image), g.Stage)
		if g.Reason != "" {
			_, _ = fmt.Fprintf(w, "reason:   %s\n", g.Reason)
		}
		s := g.Settings
		_, _ = fmt.Fprintf(w, "gate:     %d passes, %s stable, %s drain, watch %s (mode %s)\n",
			s.SuccessThreshold, s.StableWindow, s.DrainTimeout, s.WatchWindow, s.WatchMode)
		_, _ = fmt.Fprintf(w, "watch:    %s\n", describeWatch(g.Watch))
		if g.RollbackPin != nil {
			_, _ = fmt.Fprintf(w, "pinned:   %s\n", g.RollbackPin.PinRef)
		}
		_, _ = fmt.Fprintln(w)
	}
	for _, e := range g.Events[min(from, len(g.Events)):] {
		line := fmt.Sprintf("%s  %-9s %-7s %s", e.At.Local().Format("15:04:05"), e.Phase, e.Status, e.Message)
		if e.Reason != "" {
			line += " [" + e.Reason + "]"
		}
		_, _ = fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
	if g.Terminal && g.Superseded && !g.Failed {
		_, _ = fmt.Fprintln(w, "a newer deploy replaced this one")
	}
}

func appsDeploysGateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys gate <name> [deploy-id] [flags]
  %[1]s deploy status <name> [deploy-id] [flags]

Shows how a release earned its traffic: readiness passes, the stable window,
cutover, the previous release's drain, the rollback pin, and the post-deploy
watch that rolls a bad release back. Without deploy-id the newest deploy is
shown. With --watch it keeps printing new steps until the release clears its
watch, fails, is rolled back or is replaced by a newer deploy.

Exit codes: 0 healthy or still in progress, %[5]d failed its gate, tripped its
watch or was rolled back.

Flags:
  --watch                  follow the timeline until it ends
  --interval string        how often --watch re-checks (Go duration, default 2s)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the timeline as JSON (one object per step with --watch)
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, exitDeployNotHealthy)
}
