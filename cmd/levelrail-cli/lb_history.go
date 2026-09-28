package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/loadbalancer"
)

func runLBCheck(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "lb check", "print the results as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s lb check <app> [flags]\n\nProbes every upstream once, right now, and records the results.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "lb check", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.CheckLoadBalancer(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check load balancer for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		for _, r := range res.Results {
			verdict := "ok"
			if !r.OK {
				verdict = "FAIL"
			}
			line := fmt.Sprintf("  %-14s %-28s %-4s status=%d latency=%dms", r.ID, r.Dial, verdict, r.StatusCode, r.LatencyMs)
			if r.Reason != "" {
				line += "  " + r.Reason
			}
			_, _ = fmt.Fprintln(stdout, line)
		}
		if res.Note != "" {
			_, _ = fmt.Fprintf(stdout, "note: %s\n", res.Note)
		}
	})
}

func runLBHistory(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "lb history", "print the history as JSON to stdout and nothing else", stderr)
	limit := fs.Int("limit", 60, "most recent checks to show per upstream")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s lb history <app> [--limit N] [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "lb history", "app name")
	if !ok {
		return exitUsage
	}
	if *limit < 1 {
		_, _ = fmt.Fprintf(stderr, "%s: --limit must be at least 1\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetLoadBalancerHistory(context.Background(), name, *limit)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get load balancer history for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printLBHistory(stdout, res) })
}

func printLBHistory(out io.Writer, res apiclient.LoadBalancerHistory) {
	if len(res.Upstreams) == 0 {
		_, _ = fmt.Fprintln(out, "no upstream history yet")
		return
	}
	for _, u := range res.Upstreams {
		passed := 0
		for _, c := range u.Checks {
			if c.OK {
				passed++
			}
		}
		_, _ = fmt.Fprintf(out, "%s (%s) admin=%s checks=%d passed=%d\n", u.ID, u.Dial, u.AdminState, len(u.Checks), passed)
		for _, t := range u.Transitions {
			_, _ = fmt.Fprintf(out, "  %s  %s -> %s  %s\n", t.At.Format("2006-01-02 15:04:05"), t.From, t.To, t.Reason)
		}
	}
}

func runLBUpstream(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "lb upstream", "print the updated upstream as JSON to stdout and nothing else", stderr)
	state := fs.String("state", "", "active, draining (no new connections) or disabled (out of the pool) (required)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s lb upstream <app> <id> --state active|draining|disabled [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "lb upstream", "an app name and an upstream id", 2)
	if !ok {
		return exitUsage
	}
	if !loadbalancer.ValidAdminState(*state) {
		_, _ = fmt.Fprintf(stderr, "%s: lb upstream requires --state active, draining or disabled\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, id := rest[0], rest[1]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.SetLoadBalancerUpstreamState(context.Background(), name, id, *state)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set upstream %q of app %q to %s: %w", id, name, *state, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		line := fmt.Sprintf("%s (%s): admin_state=%s state=%s", res.ID, res.Dial, res.AdminState, res.State)
		if res.Reason != "" {
			line += " " + strings.TrimSpace(res.Reason)
		}
		_, _ = fmt.Fprintln(stdout, line)
	})
}
