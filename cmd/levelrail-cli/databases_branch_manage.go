package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// renderBranchNode prints one branch as JSON, or a one line summary.
func renderBranchNode(stdout, stderr io.Writer, of outputFlags, node apiclient.DatabaseBranchNode, human string) int {
	if err := renderResult(stdout, of.Format, of.Query, node, func() { _, _ = fmt.Fprint(stdout, human) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func branchLine(n apiclient.DatabaseBranchNode) string {
	flags := ""
	if n.Protected {
		flags = " protected"
	}
	exp := n.ExpiresAt
	if exp == "" {
		exp = "never"
	}
	return fmt.Sprintf("%s\tparent=%s\tmode=%s\tstate=%s\texpires=%s%s\n", safeTerminalText(n.Name), safeTerminalText(n.Parent), safeTerminalText(n.Mode), safeTerminalText(n.State), safeTerminalText(exp), flags)
}

// runDatabasesBranchManage handles ls, show, reset, promote, protect and rm.
func runDatabasesBranchManage(prog, sub string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cmd := "databases branch " + sub
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, cmd, "print the result as JSON to stdout and nothing else", stderr)
	var confirm string
	var off bool
	fs.StringVar(&confirm, "confirm", "", "type the branch name to confirm a delete")
	fs.BoolVar(&off, "off", false, "protect: remove protection instead")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesBranchUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	want := 2
	if sub == "ls" {
		want = 1
	}
	if len(rest) != want {
		_, _ = fmt.Fprintf(stderr, "%s: %s needs %d argument(s)\n\n", prog, cmd, want)
		fs.Usage()
		return exitUsage
	}
	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	fail := func(err error) int { return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", cmd, err)) }
	switch sub {
	case "ls":
		tree, err := client.ListDatabaseBranches(ctx, rest[0])
		if err != nil {
			return fail(err)
		}
		if err := renderResult(stdout, of.Format, of.Query, tree, func() {
			for _, n := range tree.Branches {
				_, _ = fmt.Fprint(stdout, branchLine(n))
			}
		}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	case "show":
		n, err := client.GetDatabaseBranch(ctx, rest[0], rest[1])
		if err != nil {
			return fail(err)
		}
		return renderBranchNode(stdout, stderr, of, n, branchLine(n)+reasonLine(n))
	case "reset":
		n, err := client.ResetDatabaseBranch(ctx, rest[0], rest[1])
		if err != nil {
			return fail(err)
		}
		return renderBranchNode(stdout, stderr, of, n, fmt.Sprintf("branch %q is being reset from %q\n", safeTerminalText(n.Name), safeTerminalText(n.Parent)))
	case "promote":
		n, err := client.PromoteDatabaseBranch(ctx, rest[0], rest[1])
		if err != nil {
			return fail(err)
		}
		return renderBranchNode(stdout, stderr, of, n, fmt.Sprintf("branch %q is now a standalone database\n", safeTerminalText(n.Name)))
	case "protect":
		on := !off
		n, err := client.PatchDatabaseBranch(ctx, rest[0], rest[1], apiclient.PatchDatabaseBranchRequest{Protected: &on})
		if err != nil {
			return fail(err)
		}
		return renderBranchNode(stdout, stderr, of, n, branchLine(n))
	default:
		if confirm != rest[1] {
			return reportError(stdout, stderr, jsonOut, newValidationError("rm needs --confirm %s", rest[1]))
		}
		if err := client.DeleteDatabaseBranch(ctx, rest[0], rest[1], confirm); err != nil {
			return fail(err)
		}
		if jsonOut {
			_, _ = fmt.Fprintln(stdout, `{"deleted":true}`)
			return exitOK
		}
		_, _ = fmt.Fprintf(stdout, "branch %q deleted\n", rest[1])
		return exitOK
	}
}

func reasonLine(n apiclient.DatabaseBranchNode) string {
	if strings.TrimSpace(n.StateReason) == "" {
		return ""
	}
	return "reason: " + safeTerminalText(n.StateReason) + "\n"
}
