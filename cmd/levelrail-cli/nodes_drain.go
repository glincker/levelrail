package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runNodesDrain implements "nodes drain <id> [--target ID] [--wait]": the
// server cordons the node and moves everything off one verified move at a
// time. --wait polls until the drain finishes and exits non-zero on failure.
func runNodesDrain(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes drain", "print the drain as JSON to stdout and nothing else", stderr)
	var target string
	var wait bool
	var timeout, interval time.Duration
	fs.StringVar(&target, "target", "", "node id to move everything to (default: the scheduler spreads)")
	fs.BoolVar(&wait, "wait", false, "wait until the drain finishes")
	fs.DurationVar(&timeout, "timeout", 30*time.Minute, "how long --wait waits")
	fs.DurationVar(&interval, "poll-interval", 3*time.Second, "how often --wait polls")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesDrainUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes drain", "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	d, err := client.StartNodeDrain(ctx, id, target)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("drain node %q: %w", id, err))
	}
	if wait {
		d, err = pollUntil(ctx, timeout, interval, d, func(ctx context.Context) (apiclient.NodeDrainResource, error) {
			return client.GetNodeDrain(ctx, d.ID)
		}, apiclient.NodeDrainResource.Done)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("wait for drain %s: %w", d.ID, err))
		}
	}
	if err := renderResult(stdout, of.Format, of.Query, d, func() { printDrainHuman(stdout, d, wait) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if d.Status == "failed" {
		return exitAPIError
	}
	return exitOK
}

func printDrainHuman(out io.Writer, d apiclient.NodeDrainResource, waited bool) {
	_, _ = fmt.Fprintf(out, "drain %s of node %s: %s\n", d.ID, d.NodeID, d.Status)
	if d.Status == "running" && !waited {
		_, _ = fmt.Fprintln(out, "the node is cordoned; follow progress with: nodes drains "+d.NodeID)
	}
	if len(d.Moves) > 0 {
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "KIND\tNAME\tTO\tSTATUS\tPHASE")
		for _, m := range d.Moves {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", m.Kind, m.Resource, nodeLabel(m.ToNodeID), m.Status, m.Phase)
		}
		_ = tw.Flush()
	}
	for _, b := range d.Blocked {
		_, _ = fmt.Fprintf(out, "blocked: %s %s: %s\n", b.Kind, b.Name, b.Reason)
	}
	if d.Error != "" {
		_, _ = fmt.Fprintf(out, "error: %s\n", d.Error)
	}
}

func nodeLabel(id string) string {
	if id == "" {
		return "(local)"
	}
	return id
}

// pollUntil re-reads a resource until done reports true or timeout passes.
func pollUntil[T any](ctx context.Context, timeout, interval time.Duration, cur T, get func(context.Context) (T, error), done func(T) bool) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for !done(cur) {
		select {
		case <-ctx.Done():
			return cur, fmt.Errorf("still running after %s", timeout)
		case <-time.After(interval):
		}
		next, err := get(ctx)
		if err != nil {
			return cur, err
		}
		cur = next
	}
	return cur, nil
}

func nodesDrainUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes drain <id> [--target NODE-ID] [--wait] [flags]

Cordons <id>, then moves every database and then every app off it, one
at a time. Each move is staged on the target and verified ready before
the source stops; stateful resources are stopped, their volumes copied,
and started on the target. Apps follow their database's node when it is
eligible. The drain stops at the first move that fails (that move rolls
back) and leaves the node cordoned. GPU apps need a GPU node; models
cannot be moved and are reported as blocked.

Flags:
  --target string          node id to move everything to (default: the scheduler spreads)
  --wait                   wait until the drain finishes, exit non-zero if it fails
  --timeout duration       how long --wait waits (default 30m)
  --poll-interval duration how often --wait polls (default 3s)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the drain as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runNodesDrains implements "nodes drains <id>": the node's recent drains with their moves.
func runNodesDrains(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes drains", "print drains as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "drains <id>", "Shows a node's recent drains, newest first, with each move."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes drains", "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	drains, err := client.ListNodeDrains(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list drains of node %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, drains, func() {
		if len(drains) == 0 {
			_, _ = fmt.Fprintln(stdout, "this node has never been drained")
		}
		for _, d := range drains {
			printDrainHuman(stdout, d, true)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
