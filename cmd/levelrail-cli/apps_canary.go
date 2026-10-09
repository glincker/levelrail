package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"
)

func runAppsCanary(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsCanaryUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsCanaryUsage(prog))
		return exitOK
	case "start", "status", "weight", "promote", "abort":
		return runAppsCanaryVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps canary subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsCanaryUsage(prog))
		return exitUsage
	}
}

func appsCanaryUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps canary start <app-name> --image <ref> [--weight 10]   run a new image beside the app and send it a share of traffic
  %[1]s apps canary status <app-name>                              show the running canary
  %[1]s apps canary weight <app-name> --weight <0-99>              change the share (0 pauses it)
  %[1]s apps canary promote <app-name>                             deploy the canary image to the app, then remove the canary
  %[1]s apps canary abort <app-name>                               remove the canary, all traffic returns to the app

One canary per app. Needs a single-replica image app without a pinned host
port. Traffic is split per request by weight, not per user. A canary that is
not ready receives no traffic.
`, prog)
}

func runAppsCanaryVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps canary "+verb, "print the result as JSON to stdout and nothing else", stderr)
	image := fs.String("image", "", "image reference for the canary (start)")
	weight := fs.Int("weight", 0, "percent of traffic for the canary, 1-99 (start, weight)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps canary %s <app-name> [flags]\n\nFlags:\n", prog, verb)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	appName, ok := requireOneArg(fs, stderr, prog, "apps canary "+verb, "app name")
	if !ok {
		return exitUsage
	}
	if verb == "start" && *image == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps canary start requires --image\n", prog)
		return exitUsage
	}
	weightSet := false
	fs.Visit(func(f *flag.Flag) { weightSet = weightSet || f.Name == "weight" })
	if verb == "weight" && !weightSet {
		_, _ = fmt.Fprintf(stderr, "%s: apps canary weight requires --weight\n", prog)
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var (
		result canaryResource
		err    error
	)
	switch verb {
	case "start":
		result, err = client.StartCanary(ctx, appName, *image, *weight)
	case "weight":
		result, err = client.SetCanaryWeight(ctx, appName, *weight)
	case "promote":
		result, err = client.PromoteCanary(ctx, appName)
	case "abort":
		result, err = client.AbortCanary(ctx, appName)
	default:
		result, err = client.GetCanary(ctx, appName)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("canary %s for app %q: %w", verb, appName, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printCanary(stdout, verb, result) })
}

func printCanary(out io.Writer, verb string, r canaryResource) {
	if !r.Active {
		msg := "no canary is running"
		switch verb {
		case "promote":
			msg = "promoted: the app now runs the canary image, canary removed"
		case "abort":
			msg = "canary removed, all traffic is on the app's current release"
		}
		_, _ = fmt.Fprintln(out, msg)
		return
	}
	_, _ = fmt.Fprintf(out, "canary:  %s\nweight:  %d%%\n", r.Image, r.Weight)
	if r.CreatedAt != nil {
		_, _ = fmt.Fprintf(out, "started: %s\n", r.CreatedAt.Format(time.RFC3339))
	}
}
