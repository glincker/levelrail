package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runRolloutWaves handles "rollout waves <app>": the rollout policy and the
// last rolling deploy's wave and replica counts.
func runRolloutWaves(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "rollout waves"
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the rollout as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, rolloutUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetRollout(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for app %q: %w", label, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printRollout(stdout, res) })
}

// printRolloutSummary shows the waves before "rollout status" waits; a
// failed lookup stays quiet so the wait itself still runs.
func printRolloutSummary(prog string, args []string, stdout io.Writer, lookupEnv func(string) (string, bool)) {
	var name, apiURL, token, profile string
	for i := 0; i < len(args); i++ {
		a := args[i]
		key, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") {
			if name == "" {
				name = a
			}
			continue
		}
		if !hasVal && i+1 < len(args) && (key == "api-url" || key == "token" || key == "profile") {
			i++
			val = args[i]
		}
		switch key {
		case "json", "o", "output":
			return
		case "api-url":
			apiURL = val
		case "token":
			token = val
		case "profile":
			profile = val
		}
	}
	if name == "" {
		return
	}
	res, err := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv).GetRollout(context.Background(), name)
	if err != nil || res.Strategy != "rolling" {
		return
	}
	printRollout(stdout, res)
}

func printRollout(w io.Writer, r apiclient.RolloutResource) {
	_, _ = fmt.Fprintf(w, "app %s: strategy %s, %d replica(s)\n", r.App, r.Strategy, r.Replicas)
	_, _ = fmt.Fprintf(w, "  max surge %d (%s), max unavailable %d (%s), min available %d (%s)\n",
		r.Resolved.MaxSurge, rolloutValueOrEnv(r.Policy.MaxSurge), r.Resolved.MaxUnavailable, rolloutValueOrEnv(r.Policy.MaxUnavailable),
		r.Resolved.MinAvailable, rolloutValueOrEnv(r.Policy.MinAvailable))
	p := r.Progress
	if p == nil {
		_, _ = fmt.Fprintln(w, "  no rolling deploy recorded since the control plane started")
		return
	}
	_, _ = fmt.Fprintf(w, "  %s: wave %d of %d, updated %d/%d, ready %d, unavailable %d, surge %d\n",
		p.Phase, p.Wave, p.Waves, p.Updated, p.Desired, p.Ready, p.Unavailable, p.Surge)
	if p.Message != "" {
		_, _ = fmt.Fprintf(w, "  %s\n", p.Message)
	}
}

func rolloutValueOrEnv(v string) string {
	if v == "" {
		return "env default"
	}
	return v
}
