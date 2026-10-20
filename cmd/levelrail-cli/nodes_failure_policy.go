package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runNodesFailurePolicy implements "nodes failure-policy get|set <id>".
func runNodesFailurePolicy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "get" && args[0] != "set") {
		_, _ = fmt.Fprint(stderr, nodesFailurePolicyUsage(prog))
		return exitUsage
	}
	action := args[0]
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes failure-policy "+action, "print the policy as JSON", stderr)
	var mode string
	var grace int
	if action == "set" {
		fs.StringVar(&mode, "mode", "", "off, notify or auto (empty restores the control plane default)")
		fs.IntVar(&grace, "grace", 0, "seconds a node may stay silent before it counts as failed (0 restores the default)")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesFailurePolicyUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes failure-policy "+action, "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var st apiclient.NodeFailureStatus
	var err error
	if action == "set" {
		st, err = client.SetNodeFailurePolicy(context.Background(), id, mode, grace)
	} else {
		st, err = client.GetNodeFailure(context.Background(), id)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("failure policy of node %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, st, func() { printFailureStatus(stdout, st) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runNodesLost implements "nodes lost <id> [--dry-run] [--yes]".
func runNodesLost(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes lost", "print the result as JSON", stderr)
	var dryRun, yes bool
	fs.BoolVar(&dryRun, "dry-run", false, "show the plan without changing anything")
	fs.BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "lost <id> [--dry-run] [--yes]", "Declares an unreachable node lost: stateless apps move to healthy nodes, apps with node-local data wait for a decision. Shows the plan first."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes lost", "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	plan, err := client.MarkNodeLost(ctx, id, true)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("plan for lost node %q: %w", id, err))
	}
	if dryRun {
		if err := renderResult(stdout, of.Format, of.Query, plan, func() {
			_, _ = fmt.Fprintln(stdout, "dry run: nothing changed")
			printFailureStatus(stdout, plan)
		}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	}
	if !yes {
		printFailureStatus(stderr, plan)
		if err := confirmNodeLost(stderr, id); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
	}
	st, err := client.MarkNodeLost(ctx, id, false)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("mark node %q lost: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, st, func() { printFailureStatus(stdout, st) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runNodesFailureResolve implements "nodes failure-resolve <id> <app|database> <name> --wait|--restore [--target ID]".
func runNodesFailureResolve(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes failure-resolve", "print the result as JSON", stderr)
	var wait, restore bool
	var target string
	fs.BoolVar(&wait, "wait", false, "keep waiting for the node to return")
	fs.BoolVar(&restore, "restore", false, "place it on another node so its data can be restored from backup there")
	fs.StringVar(&target, "target", "", "node id for --restore (default: the scheduler picks)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "failure-resolve <id> <app|database> <name> --wait|--restore [--target ID]", "Decides what happens to a resource whose data lives on a failed node."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "nodes failure-resolve", "node id, kind and name", 3)
	if !ok || wait == restore {
		if ok {
			_, _ = fmt.Fprintln(stderr, "pass exactly one of --wait or --restore")
		}
		return exitUsage
	}
	action := "wait"
	if restore {
		action = "restore"
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	st, err := client.ResolveNodeFailure(context.Background(), rest[0], rest[1], rest[2], action, target)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("resolve %s %s on node %q: %w", rest[1], rest[2], rest[0], err))
	}
	if err := renderResult(stdout, of.Format, of.Query, st, func() { printFailureStatus(stdout, st) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printFailureStatus(out io.Writer, st apiclient.NodeFailureStatus) {
	phase := st.Phase
	if phase == "" {
		phase = "healthy"
	}
	_, _ = fmt.Fprintf(out, "node %s: %s (policy %s, grace %ds)\n", st.NodeID, phase, st.Mode, st.GraceSeconds)
	if st.Phase == "grace" {
		_, _ = fmt.Fprintf(out, "  silent for %ds, %ds of grace left\n", st.SilentSeconds, st.GraceRemainingSeconds)
	}
	if st.Reason != "" {
		_, _ = fmt.Fprintf(out, "  %s\n", st.Reason)
	}
	for _, a := range st.Consequences {
		line := fmt.Sprintf("  %-8s %-24s %s", a.Kind, a.Name, a.Class)
		if a.Target != "" {
			line += " -> " + a.Target
		}
		_, _ = fmt.Fprintf(out, "%s [%s] %s\n", line, a.Status, a.Reason)
	}
}

func confirmNodeLost(stderr io.Writer, id string) error {
	if f, ok := stdinSource.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // a terminal file descriptor always fits an int
		return errors.New("not a terminal: pass --yes to mark the node lost without confirmation")
	}
	_, _ = fmt.Fprintf(stderr, "Mark node %s lost and apply this plan? [y/N] ", id)
	line, _ := bufio.NewReader(stdinSource).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return errors.New("cancelled")
	}
	return nil
}

func nodesFailurePolicyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes failure-policy get <id> [flags]
  %[1]s nodes failure-policy set <id> [--mode off|notify|auto] [--grace SECONDS] [flags]

Shows or sets what happens when a node stays unreachable. Default is off.
notify only reports it, auto also moves stateless apps to healthy nodes.
Apps with node-local volumes and databases are never moved automatically.
`, prog)
}
