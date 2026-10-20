package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsWatch dispatches "apps watch get|set|stop <name>".
func runAppsWatch(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsWatchUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsWatchUsage(prog))
		return exitOK
	case "get", "set", "stop":
		return runAppsWatchVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps watch subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsWatchUsage(prog))
		return exitUsage
	}
}

func runAppsWatchVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps watch " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	var sf safetyFlags
	if verb == "set" {
		sf.registerWatch(fs)
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsWatchUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	var (
		res apiclient.AppWatchResource
		err error
	)
	switch verb {
	case "stop":
		res, err = client.StopAppWatch(ctx, name)
	case "set":
		res, err = setAppWatch(ctx, client, name, fs, &sf)
	default:
		res, err = client.GetAppWatch(ctx, name)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for app %q: %w", label, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printAppWatch(stdout, res) })
}

func setAppWatch(ctx context.Context, client *Client, name string, fs *flag.FlagSet, sf *safetyFlags) (apiclient.AppWatchResource, error) {
	current, err := client.GetDeploySafety(ctx, name)
	if err != nil {
		return apiclient.AppWatchResource{}, err
	}
	overrides := current.Overrides
	if err := sf.apply(fs, &overrides); err != nil {
		return apiclient.AppWatchResource{}, err
	}
	if _, err := client.SetDeploySafety(ctx, name, overrides); err != nil {
		return apiclient.AppWatchResource{}, err
	}
	return client.GetAppWatch(ctx, name)
}

// describeWatch is the one-line state shown by "deploys gate" and "watch get".
func describeWatch(w apiclient.WatchStatusResource) string {
	switch w.State {
	case "watching":
		return fmt.Sprintf("watching, %s left of %s (mode %s)", (time.Duration(w.RemainingSeconds) * time.Second).String(), (time.Duration(w.WindowSeconds) * time.Second).String(), w.Mode)
	case "tripped", "rolled_back":
		return fmt.Sprintf("%s: %s [%s]", w.State, w.Message, w.Reason)
	case "stopped":
		return "stopped early by an operator"
	case "cleared":
		return "cleared, no regression in the window"
	case "off":
		return "off for this app"
	default:
		return "waiting for the release to take traffic"
	}
}

func printAppWatch(w io.Writer, r apiclient.AppWatchResource) {
	s := r.Settings
	if r.DeployID != "" {
		_, _ = fmt.Fprintf(w, "deploy:         %s (%s)\n", r.DeployID, unpinnedImage(r.Image))
	}
	_, _ = fmt.Fprintf(w, "state:          %s\n", describeWatch(r.Watch))
	if r.Watch.Notice != "" {
		_, _ = fmt.Fprintf(w, "notice:         %s\n", r.Watch.Notice)
	}
	_, _ = fmt.Fprintf(w, "mode:           %s\n", s.WatchMode)
	_, _ = fmt.Fprintf(w, "window:         %s\n", s.WatchWindow)
	_, _ = fmt.Fprintf(w, "max restarts:   %d\n", s.WatchMaxRestarts)
	_, _ = fmt.Fprintf(w, "5xx rate:       %.0f%% (needs %d requests)\n", s.WatchErrorRate*100, s.WatchMinRequests)
	_, _ = fmt.Fprintf(w, "latency ratio:  %.1fx p95 over the previous release (min rise %s, %d ticks in a row)\n", s.WatchLatencyRatio, s.WatchLatencyMinDelta, s.WatchSoftTicks)
	_, _ = fmt.Fprintf(w, "soft rollback:  %t\n", s.WatchSoftRollback)
}

func appsWatchUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps watch get <name> [flags]     post-deploy watch state and settings
  %[1]s apps watch set <name> [flags]     change the watch for this app ("default" clears one)
  %[1]s apps watch stop <name> [flags]    end the open watch of the newest release early

After a release takes traffic it is watched for --watch-window. A crashloop,
OOM kill, readiness failure or 5xx share over --watch-error-rate is a hard
signal: in mode auto the release is rolled back to the retained previous one,
in mode notify the alert channels are told. A p95 latency regression against
the previous release is a soft signal: it only notifies unless --soft-rollback
is true and the mode is auto. To roll back by hand use "%[1]s apps rollback".

Set flags:
  --mode MODE                 off, notify or auto
  --watch-window D            e.g. 10m
  --watch-max-restarts N      restarts that trip the watch
  --watch-error-rate F        5xx share between 0 and 1
  --watch-min-requests N      requests needed before a rate or latency counts
  --latency-ratio F           p95 multiple over the previous release, 0 is off
  --soft-rollback BOOL        let a latency regression roll back in auto mode

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
