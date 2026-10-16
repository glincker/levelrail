package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runAppsSleep(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsSleepUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsSleepUsage(prog))
		return exitOK
	case "list":
		return runAppsSleepList(prog, args[1:], stdout, stderr, lookupEnv)
	case "history":
		return runAppsSleepHistory(prog, args[1:], stdout, stderr, lookupEnv)
	case "enable", "disable", "status", "wake", "now":
		return runAppsSleepVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps sleep subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsSleepUsage(prog))
		return exitUsage
	}
}

func appsSleepUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps sleep enable <app-name> [--after 30m] [--hold]   stop the app after it has had no requests for this long
  %[1]s apps sleep disable <app-name>                never sleep (wakes the app first if it is asleep)
  %[1]s apps sleep list                              every app with sleep on, which are asleep, and the memory freed
  %[1]s apps sleep status <app-name>                 show the setting, whether it is asleep, last slept and woke, and 7 day asleep time
  %[1]s apps sleep history <app-name> [--since 7d] [--limit N]   when the app slept and woke, and why
  %[1]s apps sleep now <app-name>                    put the app to sleep now instead of waiting for the idle time
  %[1]s apps sleep wake <app-name>                   start a sleeping app now

A sleeping app uses no memory or CPU. The next request to its domain starts it
and shows a short "waking up" page that retries on its own, so the first
visitor waits for the cold start. Off by default; --after takes 5m to 168h.
`, prog)
}

func runAppsSleepVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps sleep "+verb, "print the result as JSON to stdout and nothing else", stderr)
	after := fs.Duration("after", 30*time.Minute, "idle time before the app sleeps (enable)")
	hold := fs.Bool("hold", false, "make a request that wakes the app wait for it and be replayed, instead of showing a waking-up page (enable)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps sleep %s <app-name> [flags]\n\nFlags:\n", prog, verb)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	appName, ok := requireOneArg(fs, stderr, prog, "apps sleep "+verb, "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var (
		result appSleepResource
		err    error
	)
	switch verb {
	case "enable":
		if *hold {
			result, err = client.SetAppSleepHold(ctx, appName, int(after.Minutes()), true)
		} else {
			result, err = client.SetAppSleep(ctx, appName, int(after.Minutes()))
		}
	case "disable":
		result, err = client.SetAppSleep(ctx, appName, 0)
	case "wake":
		result, err = client.WakeApp(ctx, appName)
	case "now":
		result, err = client.SleepAppNow(ctx, appName)
	default:
		result, err = client.GetAppSleep(ctx, appName)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("sleep %s for app %q: %w", verb, appName, err))
	}
	var hist *apiclient.SleepHistory
	if verb == "status" {
		if h, herr := client.GetSleepHistory(ctx, appName, "", 1); herr == nil {
			hist = &h
		}
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		if !result.Enabled {
			_, _ = fmt.Fprintln(stdout, "sleep: off")
			return
		}
		_, _ = fmt.Fprintf(stdout, "sleep:   after %d minutes idle\nasleep:  %t\nhold:    %t\n", result.IdleMinutes, result.Sleeping, result.HoldRequests)
		if hist != nil {
			printSleepStatusStats(stdout, hist.Stats)
		}
	})
}

func runAppsSleepList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps sleep list", "print the result as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	fleet, err := client.ListSleepFleet(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list sleeping apps: %w", err))
	}
	lastWoke := map[string]time.Time{}
	for _, a := range fleet.Apps {
		if h, herr := client.GetSleepHistory(context.Background(), a.Name, "30d", 1); herr == nil && h.Stats.LastWokeAt != nil {
			lastWoke[a.Name] = *h.Stats.LastWokeAt
		}
	}
	return writeScheduledTaskResult(stdout, stderr, of, fleet, func() {
		if fleet.Enabled == 0 {
			_, _ = fmt.Fprintln(stdout, "no app sleeps when idle")
			return
		}
		_, _ = fmt.Fprintf(stdout, "%d of %d apps asleep, %d MiB of memory limits not in use\n", fleet.Sleeping, fleet.Enabled, fleet.ReclaimedBytes>>20)
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "APP\tSTATE\tAFTER\tSINCE\tLAST WOKE")
		for _, a := range fleet.Apps {
			state, since := "awake", "-"
			if a.Sleeping {
				state = "asleep"
				if a.Since != nil {
					since = humanAgo(*a.Since, time.Now())
				}
			}
			woke := "-"
			if t, ok := lastWoke[a.Name]; ok {
				woke = humanAgo(t, time.Now())
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d min\t%s\t%s\n", a.Name, state, a.IdleMinutes, since, woke)
		}
		_ = tw.Flush()
	})
}
