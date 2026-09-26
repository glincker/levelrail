package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// deployExtraFlags are apps deploy/rollback's digest and freeze flags.
type deployExtraFlags struct {
	pull           bool
	overrideFreeze bool
	overrideReason string
}

func (d *deployExtraFlags) register(fs *flag.FlagSet) {
	fs.BoolVar(&d.pull, "pull", false, "re-resolve the tag against its registry and fail instead of using a cached image; without --image, redeploys the current tag")
	fs.BoolVar(&d.overrideFreeze, "override-freeze", false, "deploy even though a deploy freeze window is active (requires --override-reason)")
	fs.StringVar(&d.overrideReason, "override-reason", "", "why this deploy overrides the freeze, recorded on the deploy")
}

func (d deployExtraFlags) request(image string, confirm bool) apiclient.DeployTriggerRequest {
	return apiclient.DeployTriggerRequest{Image: image, Confirm: confirm, Pull: d.pull, OverrideFreeze: d.overrideFreeze, OverrideReason: d.overrideReason}
}

// unpinnedImage drops a trailing "@digest" for display and re-resolution.
func unpinnedImage(ref string) string {
	if i := strings.LastIndex(ref, "@"); i > 0 {
		return ref[:i]
	}
	return ref
}

// shortDigest renders a digest as its first 12 hex characters plus how it
// was obtained when that is not a plain registry resolution.
func shortDigest(digest, reason string) string {
	d := strings.TrimPrefix(digest, "sha256:")
	if len(d) > 12 {
		d = d[:12]
	}
	if d == "" {
		d = "-"
	}
	if reason != "" && reason != "Resolved" && reason != "AlreadyPinned" {
		d += " (" + reason + ")"
	}
	return d
}

// runAppsFreeze dispatches "apps freeze set|show|clear".
func runAppsFreeze(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsFreezeUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsFreezeUsage(prog))
		return exitOK
	case "show", "set", "clear":
		return runAppsFreezeVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps freeze subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsFreezeUsage(prog))
		return exitUsage
	}
}

func appsFreezeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps freeze show <app-name> [flags]
  %[1]s apps freeze set <app-name> --cron EXPR --duration D [--timezone TZ] [--reason TEXT] [flags]
  %[1]s apps freeze clear <app-name> [flags]

A freeze window starts at every match of a 5-field cron expression,
evaluated in --timezone (default UTC), and lasts --duration. While one
is active, webhook, pipeline and released deploys are held and run
automatically when the window ends; manual deploys need
"apps deploy --override-freeze --override-reason TEXT". "set" replaces
the app's windows with the one given. Global windows (settings) apply
on top and are shown as inherited.

Example, a weekend freeze in Berlin:
  %[1]s apps freeze set web --cron "0 17 * * 5" --duration 64h --timezone Europe/Berlin --reason weekend

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read
  --json                    print the result as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsFreezeVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps freeze " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the freeze state as JSON to stdout and nothing else", stderr)
	var win apiclient.FreezeWindowResource
	if verb == "set" {
		fs.StringVar(&win.Cron, "cron", "", "5-field cron expression for when each window starts (required)")
		fs.StringVar(&win.Duration, "duration", "", "how long each window lasts, e.g. 2h or 64h (required)")
		fs.StringVar(&win.Timezone, "timezone", "UTC", "IANA timezone the cron expression is evaluated in")
		fs.StringVar(&win.Reason, "reason", "", "shown on held deploys")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsFreezeUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	if verb == "set" && (win.Cron == "" || win.Duration == "") {
		return reportError(stdout, stderr, jsonOut, newValidationError("--cron and --duration are required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var (
		res apiclient.DeployFreezeResource
		err error
	)
	switch verb {
	case "show":
		res, err = client.GetDeployFreeze(ctx, name)
	case "set":
		res, err = client.SetDeployFreeze(ctx, name, []apiclient.FreezeWindowResource{win})
	default:
		res, err = client.SetDeployFreeze(ctx, name, nil)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for app %q: %w", label, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printDeployFreezeHuman(stdout, res) })
}

func printDeployFreezeHuman(out io.Writer, res apiclient.DeployFreezeResource) {
	if res.Status.Frozen && res.Status.Until != nil {
		_, _ = fmt.Fprintf(out, "status:  FROZEN until %s", res.Status.Until.Format(time.RFC3339))
		if res.Status.Reason != "" {
			_, _ = fmt.Fprintf(out, " (%s)", res.Status.Reason)
		}
		_, _ = fmt.Fprintln(out)
	} else {
		_, _ = fmt.Fprintln(out, "status:  open, automatic deploys run immediately")
	}
	printWindows := func(title string, ws []apiclient.FreezeWindowResource) {
		if len(ws) == 0 {
			return
		}
		_, _ = fmt.Fprintf(out, "%s:\n", title)
		for _, w := range ws {
			_, _ = fmt.Fprintf(out, "  %-16s for %-8s %-16s %s\n", w.Cron, w.Duration, w.Timezone, w.Reason)
		}
	}
	printWindows("windows", res.Windows)
	printWindows("inherited (global)", res.Inherited)
	if len(res.Windows) == 0 && len(res.Inherited) == 0 {
		_, _ = fmt.Fprintln(out, "no freeze windows")
	}
}
