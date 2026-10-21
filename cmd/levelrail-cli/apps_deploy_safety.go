package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// inheritValue clears one override so the setting follows the platform default.
const inheritValue = "default"

// runAppsDeploySafety dispatches "apps deploy-safety show|set|targets <name>".
func runAppsDeploySafety(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsDeploySafetyUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsDeploySafetyUsage(prog))
		return exitOK
	case "show", "set", "targets":
		return runAppsDeploySafetyVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps deploy-safety subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsDeploySafetyUsage(prog))
		return exitUsage
	}
}

type safetyFlags struct {
	threshold, stable, drain, watch, restarts, errorRate, minRequests, autoRollback string
	mode, latencyRatio, softRollback                                                string
}

func (f *safetyFlags) registerSoft(fs *flag.FlagSet) {
	fs.StringVar(&f.mode, "mode", "", "off, notify (alert only) or auto (alert and roll back), or default")
	fs.StringVar(&f.latencyRatio, "latency-ratio", "", "p95 latency multiple over the previous release that counts as a regression (0 turns it off)")
	fs.StringVar(&f.softRollback, "soft-rollback", "", "let a latency regression roll back too, in auto mode (true or false)")
}

func (f *safetyFlags) registerWatch(fs *flag.FlagSet) {
	f.registerSoft(fs)
	fs.StringVar(&f.watch, "watch-window", "", "how long a new release is watched after cutover, e.g. 10m")
	fs.StringVar(&f.restarts, "watch-max-restarts", "", "restarts inside the watch that trip it")
	fs.StringVar(&f.errorRate, "watch-error-rate", "", "5xx share (0 to 1) inside the watch that trips it")
	fs.StringVar(&f.minRequests, "watch-min-requests", "", "requests needed before the 5xx share or latency counts")
}

func (f *safetyFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.threshold, "success-threshold", "", "consecutive readiness passes before traffic")
	fs.StringVar(&f.stable, "stable-window", "", "how long a new container must run without a restart or OOM kill")
	fs.StringVar(&f.drain, "drain-timeout", "", "how long the previous release keeps serving after cutover")
	fs.StringVar(&f.watch, "watch-window", "", "how long a new release is watched after cutover")
	fs.StringVar(&f.restarts, "watch-max-restarts", "", "restarts inside the watch that trip it")
	fs.StringVar(&f.errorRate, "watch-error-rate", "", "5xx share (0 to 1) inside the watch that trips it")
	fs.StringVar(&f.minRequests, "watch-min-requests", "", "requests needed before the 5xx share counts")
	fs.StringVar(&f.autoRollback, "auto-rollback", "", "roll back automatically when the watch trips (true or false)")
	f.registerSoft(fs)
}

func runAppsDeploySafetyVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps deploy-safety " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	var sf safetyFlags
	if verb == "set" {
		sf.register(fs)
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploySafetyUsage(prog)) }

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

	if verb == "targets" {
		targets, err := client.ListRollbackTargets(ctx, name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for app %q: %w", label, name, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, targets, func() { printRollbackTargets(stdout, targets) })
	}

	res, err := client.GetDeploySafety(ctx, name)
	if err == nil && verb == "set" {
		overrides := res.Overrides
		if err = sf.apply(fs, &overrides); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		res, err = client.SetDeploySafety(ctx, name, overrides)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for app %q: %w", label, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printDeploySafety(stdout, res) })
}

// apply writes every flag the caller set onto o; "default" clears one.
func (f *safetyFlags) apply(fs *flag.FlagSet, o *apiclient.DeploySafetyOverrides) error {
	var err error
	fs.Visit(func(fl *flag.Flag) {
		if err != nil {
			return
		}
		raw := fl.Value.String()
		inherit := raw == inheritValue
		switch fl.Name {
		case "success-threshold":
			o.SuccessThreshold, err = optionalInt(fl.Name, raw, inherit)
		case "watch-max-restarts":
			o.WatchMaxRestarts, err = optionalInt(fl.Name, raw, inherit)
		case "watch-min-requests":
			o.WatchMinRequests, err = optionalInt(fl.Name, raw, inherit)
		case "stable-window":
			o.StableWindow, err = optionalDuration(fl.Name, raw, inherit)
		case "drain-timeout":
			o.DrainTimeout, err = optionalDuration(fl.Name, raw, inherit)
		case "watch-window":
			o.WatchWindow, err = optionalDuration(fl.Name, raw, inherit)
		case "watch-error-rate":
			o.WatchErrorRate, err = optionalFloat(fl.Name, raw, inherit)
		case "mode":
			o.WatchMode, err = optionalMode(raw, inherit)
		case "latency-ratio":
			o.WatchLatencyRatio, err = optionalFloat(fl.Name, raw, inherit)
		case "soft-rollback":
			o.WatchSoftRollback, err = optionalBool(fl.Name, raw, inherit)
		case "auto-rollback":
			o.AutoRollback, err = optionalBool(fl.Name, raw, inherit)
		}
	})
	return err
}

func optionalMode(raw string, inherit bool) (*string, error) {
	if inherit {
		return nil, nil
	}
	switch raw {
	case "off", "notify", "auto":
		return &raw, nil
	}
	return nil, newValidationError("--mode must be off, notify, auto or %q, got %q", inheritValue, raw)
}

func optionalInt(name, raw string, inherit bool) (*int, error) {
	if inherit {
		return nil, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil, newValidationError("--%s must be a whole number or %q, got %q", name, inheritValue, raw)
	}
	return &n, nil
}

func optionalDuration(name, raw string, inherit bool) (*string, error) {
	if inherit {
		return nil, nil
	}
	if _, err := time.ParseDuration(raw); err != nil {
		return nil, newValidationError("--%s must be a duration such as 10s or %q, got %q", name, inheritValue, raw)
	}
	return &raw, nil
}

func optionalFloat(name, raw string, inherit bool) (*float64, error) {
	if inherit {
		return nil, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, newValidationError("--%s must be a number or %q, got %q", name, inheritValue, raw)
	}
	return &v, nil
}

func optionalBool(name, raw string, inherit bool) (*bool, error) {
	if inherit {
		return nil, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, newValidationError("--%s must be true, false or %q, got %q", name, inheritValue, raw)
	}
	return &v, nil
}

func printDeploySafety(w io.Writer, r apiclient.DeploySafetyResource) {
	e, o := r.Effective, r.Overrides
	mark := func(set bool) string {
		if set {
			return "  (this app)"
		}
		return ""
	}
	_, _ = fmt.Fprintf(w, "success threshold:   %d%s\n", e.SuccessThreshold, mark(o.SuccessThreshold != nil))
	_, _ = fmt.Fprintf(w, "stable window:       %s%s\n", e.StableWindow, mark(o.StableWindow != nil))
	_, _ = fmt.Fprintf(w, "drain timeout:       %s%s\n", e.DrainTimeout, mark(o.DrainTimeout != nil))
	_, _ = fmt.Fprintf(w, "watch window:        %s%s\n", e.WatchWindow, mark(o.WatchWindow != nil))
	_, _ = fmt.Fprintf(w, "watch max restarts:  %d%s\n", e.WatchMaxRestarts, mark(o.WatchMaxRestarts != nil))
	_, _ = fmt.Fprintf(w, "watch 5xx rate:      %.0f%%%s\n", e.WatchErrorRate*100, mark(o.WatchErrorRate != nil))
	_, _ = fmt.Fprintf(w, "watch min requests:  %d%s\n", e.WatchMinRequests, mark(o.WatchMinRequests != nil))
	_, _ = fmt.Fprintf(w, "watch mode:          %s%s\n", e.WatchMode, mark(o.WatchMode != nil || o.AutoRollback != nil))
	_, _ = fmt.Fprintf(w, "watch latency ratio: %.1fx%s\n", e.WatchLatencyRatio, mark(o.WatchLatencyRatio != nil))
	_, _ = fmt.Fprintf(w, "soft rollback:       %t%s\n", e.WatchSoftRollback, mark(o.WatchSoftRollback != nil))
	_, _ = fmt.Fprintf(w, "rollback pins kept:  %d\n", e.RollbackPins)
}

func printRollbackTargets(w io.Writer, targets []apiclient.RollbackTargetResource) {
	if len(targets) == 0 {
		_, _ = fmt.Fprintln(w, "no pinned releases yet; a release is pinned when it takes traffic")
		return
	}
	for _, t := range targets {
		live := ""
		if t.Live {
			live = "  live"
		}
		_, _ = fmt.Fprintf(w, "%s  %s  %s  %s%s\n", t.PinnedAt.Local().Format(time.DateTime), t.DeployID, unpinnedImage(t.Image), t.PinRef, live)
	}
}

func appsDeploySafetyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploy-safety show <name> [flags]       effective readiness gate and post-deploy watch settings
  %[1]s apps deploy-safety set <name> [flags]        override settings for this app ("default" clears one)
  %[1]s apps deploy-safety targets <name> [flags]    pinned releases image cleanup will never remove

A new release takes traffic only after --success-threshold consecutive
readiness passes and --stable-window of running without a restart or OOM
kill. The previous release keeps serving for --drain-timeout after cutover.
For --watch-window after cutover a crashloop, OOM kill or 5xx share over
--watch-error-rate rolls the release back when --auto-rollback is on.

Set flags:
  --success-threshold N       consecutive readiness passes before traffic
  --stable-window D           e.g. 10s
  --drain-timeout D           e.g. 15s
  --watch-window D            e.g. 5m
  --watch-max-restarts N      restarts inside the watch that trip it
  --watch-error-rate F        5xx share between 0 and 1, e.g. 0.2
  --watch-min-requests N      requests needed before the 5xx share counts
  --auto-rollback BOOL        true or false
  --mode MODE                 off, notify or auto (see "apps watch")
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
