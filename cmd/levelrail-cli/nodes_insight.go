package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/glincker/levelrail/internal/apiclient"
)

func runNodesDoctor(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes doctor", "print the checks as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s nodes doctor <id> [flags]\n\nRuns the agent doctor: docker, disk, clock, ports, certificate, heartbeat and version checks, each failure with a fix.\n", prog)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes doctor", "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	doc, err := client.GetNodeDoctor(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("doctor for node %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, doc, func() { printNodeDoctor(stdout, doc) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if doc.Summary.Fail > 0 {
		return exitCheckFailed
	}
	return exitOK
}

func printNodeDoctor(out io.Writer, doc apiclient.NodeDoctor) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "CHECK\tSTATUS\tDETAIL")
	for _, c := range doc.Checks {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", c.ID, c.Status, c.Detail)
	}
	_ = tw.Flush()
	for _, c := range doc.Checks {
		if c.Fix != "" {
			_, _ = fmt.Fprintf(out, "\n%s: %s\n", c.ID, c.Fix)
			if c.FixCommand != "" {
				_, _ = fmt.Fprintf(out, "  %s\n", c.FixCommand)
			}
		}
	}
	if !doc.Reported {
		_, _ = fmt.Fprintln(out, "\nThe agent has not sent a preflight report yet, so docker, disk, clock and port checks are missing.")
	}
}

func runNodesTimeline(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes timeline", "print the timeline as JSON to stdout and nothing else", stderr)
	window := fs.String("window", "", "how far back to look, for example 24h or 168h (default: server default, 24h)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s nodes timeline <id> [--window 24h] [flags]\n\nShows how long a node spent online and offline, with an uptime figure.\n", prog)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes timeline", "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	tl, err := client.GetNodeTimeline(context.Background(), id, *window)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("timeline for node %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, tl, func() { printNodeTimeline(stdout, tl) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printNodeTimeline(out io.Writer, tl apiclient.NodeTimeline) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FROM\tTO\tSTATE\tDURATION")
	for _, s := range tl.Segments {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.From.Format("01-02 15:04"), s.To.Format("01-02 15:04"), s.State, (time.Duration(s.Seconds) * time.Second).String())
	}
	_ = tw.Flush()
	if tl.UptimePercent != nil {
		_, _ = fmt.Fprintf(out, "Uptime %.2f%% with %d transitions\n", *tl.UptimePercent, tl.Transitions)
	}
}

func runNodesVersionSkew(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes version-skew", "print the report as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s nodes version-skew [flags]\n\nCompares every agent with the control plane version and prints the suggested upgrade order.\n", prog)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	rep, err := client.GetNodeVersionSkew(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("version skew: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, rep, func() { printNodeVersionSkew(stdout, rep) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printNodeVersionSkew(out io.Writer, rep apiclient.NodeVersionSkew) {
	_, _ = fmt.Fprintf(out, "Control plane %s: %d current, %d behind, %d ahead, %d unknown\n\n", rep.ControlPlaneVersion, rep.Current, rep.Behind, rep.Ahead, rep.Unknown)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NODE\tVERSION\tSTATE\tCLASS")
	for _, n := range rep.Nodes {
		state := "offline"
		if n.Online {
			state = "online"
		}
		v := n.Version
		if v == "" {
			v = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", n.Name, v, state, n.Class)
	}
	_ = tw.Flush()
	if len(rep.Plan) > 0 {
		_, _ = fmt.Fprintln(out, "\nUpgrade plan (cordoned first, offline last):")
		for _, s := range rep.Plan {
			note := ""
			if s.Note != "" {
				note = " (" + s.Note + ")"
			}
			_, _ = fmt.Fprintf(out, "  %d. %s %s to %s%s\n", s.Order, s.Name, s.From, s.To, note)
		}
	}
}

func runNodesAgentUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 && (args[0] == "hold" || args[0] == "unhold" || args[0] == "apply") {
		return runNodesAgentUpdateControl(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes agent-update", "print the decision as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s nodes agent-update <id> [flags]\n\nShows which agent version the node should run, following the control plane update channel,\nand why an update is waiting.\n\n  %s nodes agent-update hold|unhold <id>      stop or resume automatic updates for one node\n  %s nodes agent-update apply <id> --force    update anyway on the next agent check\n", prog, prog, prog)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes agent-update", "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	d, err := client.GetNodeAgentUpdate(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("agent update for node %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, d, func() {
		_, _ = fmt.Fprintf(stdout, "%s on channel %s", d.Action, d.Channel)
		if d.Target != "" {
			_, _ = fmt.Fprintf(stdout, " to %s", d.Target)
		}
		_, _ = fmt.Fprintf(stdout, ": %s\n", d.Reason)
		if d.Held {
			_, _ = fmt.Fprintf(stdout, "this node is on hold; release it with: %s nodes agent-update unhold %s\n", prog, id)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

const agentUpdateDisclaimer = "Skips the maintenance window and hold. The update is still signature verified and rolls back on failure."

// runNodesAgentUpdateControl handles hold, unhold and apply. apply is the
// explicit override: it needs --force, shows a disclaimer and asks unless --yes.
func runNodesAgentUpdateControl(prog, action string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes agent-update "+action, "print the result as JSON to stdout and nothing else", stderr)
	force := fs.Bool("force", false, "update anyway (apply only)")
	yes := fs.Bool("yes", false, "skip the confirmation prompt (apply only)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s nodes agent-update %s <id> [flags]\n\n  --force   update anyway on the next agent check (apply only)\n  --yes     skip the confirmation prompt (apply only)\n", prog, action)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes agent-update "+action, "node id")
	if !ok {
		return exitUsage
	}
	if action == "apply" {
		if !*force {
			_, _ = fmt.Fprintf(stderr, "nothing was changed. To update anyway run: %s nodes agent-update apply %s --force\n%s\n", prog, id, agentUpdateDisclaimer)
			return exitUsage
		}
		_, _ = fmt.Fprintln(stderr, agentUpdateDisclaimer)
		if !*yes {
			if err := confirmDestructive(stderr, "Update this agent anyway?"); err != nil {
				return reportError(stdout, stderr, jsonOut, err)
			}
		}
		action = "override"
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.NodeAgentUpdateControl(context.Background(), id, action); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("agent update %s for node %q: %w", action, id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, map[string]string{"node_id": id, "action": action}, func() {
		_, _ = fmt.Fprintf(stdout, "node %q: %s recorded\n", id, action)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
