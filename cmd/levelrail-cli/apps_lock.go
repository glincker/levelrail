package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsLockUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps lock <app-name> [--reason TEXT] [flags]
  %[1]s apps lock <app-name> --status [flags]
  %[1]s apps unlock <app-name> [flags]
  %[1]s apps publish <app-name> <deploy-id> [flags]

A publish lock pins the live release. New pushes, webhooks and build hooks
still build, but each finished build waits as "held" instead of going live.
"apps publish" makes one held build live, "apps unlock" lets later builds go
live on their own again. "apps deploys rollback-to --lock" rolls back and
locks in one step.

Flags:
  --reason string         why publishing is locked, shown in the banner
  --status                show the lock and its held builds without changing it
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read
  --json                  print the result as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runAppsLock implements "apps lock", "apps unlock" and "apps publish".
func runAppsLock(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	var reason string
	var status bool
	if verb == "lock" {
		fs.StringVar(&reason, "reason", "", "why publishing is locked, shown in the banner")
		fs.BoolVar(&status, "status", false, "show the lock and its held builds without changing it")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsLockUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	if verb == "publish" {
		rest, ok := requireArgs(fs, stderr, prog, label, "an app name and a held deploy id", 2)
		if !ok {
			return exitUsage
		}
		res, err := client.PublishHeldBuild(ctx, rest[0], rest[1])
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("publish deploy %q of app %q: %w", rest[1], rest[0], err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, res, func() {
			_, _ = fmt.Fprintf(stdout, "published deploy %s of %s\n", rest[1], rest[0])
		})
	}
	name, ok := requireOneArg(fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	var res apiclient.PublishLockResource
	var err error
	switch {
	case verb == "unlock":
		res, err = client.UnlockPublishing(ctx, name)
	case status:
		res, err = client.GetPublishLock(ctx, name)
	default:
		res, err = client.LockPublishing(ctx, name, reason)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s app %q: %w", label, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printPublishLockHuman(stdout, res) })
}

func printPublishLockHuman(out io.Writer, res apiclient.PublishLockResource) {
	if !res.Locked {
		_, _ = fmt.Fprintf(out, "unlocked, builds go live on their own (%d held)\n", len(res.HeldBuilds))
		return
	}
	_, _ = fmt.Fprintf(out, "LOCKED by %s at %s", res.LockedBy, res.LockedAt.Format(time.RFC3339))
	if res.Reason != "" {
		_, _ = fmt.Fprintf(out, " (%s)", res.Reason)
	}
	_, _ = fmt.Fprintf(out, "\nlive image: %s\nheld builds: %d\n", res.PinnedImage, len(res.HeldBuilds))
	for _, id := range res.HeldBuilds {
		_, _ = fmt.Fprintf(out, "  %s\n", id)
	}
}
