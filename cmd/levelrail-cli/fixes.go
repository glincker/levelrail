package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// paramFlags collects repeated --param key=value flags.
type paramFlags map[string]string

func (p paramFlags) String() string { return "" }
func (p paramFlags) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok || k == "" {
		return fmt.Errorf("expected key=value, got %q", v)
	}
	p[k] = val
	return nil
}

// runFixes implements "fixes list|preview|apply" over /api/v1/fixes.
func runFixes(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	sub := ""
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	if sub != "list" && sub != "preview" && sub != "apply" {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s fixes list [--subject S] [--all]\n  %[1]s fixes preview <id>\n  %[1]s fixes apply <id> [--param k=v] [--yes]\n", prog)
		return exitUsage
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "fixes "+sub, "print the result as JSON to stdout and nothing else", stderr)
	subject := fs.String("subject", "", "list: only fixes for this subject (mesh, disk, docker)")
	all := fs.Bool("all", false, "list: include fixes whose check already passes")
	yes := fs.Bool("yes", false, "apply: skip the confirmation prompt")
	params := paramFlags{}
	fs.Var(params, "param", "apply: fix parameter as key=value (repeatable)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	rest := fs.Args()
	if sub != "list" && len(rest) != 1 {
		return reportError(stdout, stderr, jsonOut, newValidationError("a fix id is required"))
	}
	switch sub {
	case "list":
		fixes, err := client.ListFixes(ctx, *subject, *all)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list fixes: %w", err))
		}
		return finishRender(stdout, stderr, of, fixes, func() { printFixes(stdout, fixes) })
	case "preview":
		f, err := client.PreviewFix(ctx, rest[0])
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview fix: %w", err))
		}
		return finishRender(stdout, stderr, of, f, func() { printFixDetail(stdout, f) })
	}
	if !*yes {
		f, err := client.PreviewFix(ctx, rest[0])
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview fix: %w", err))
		}
		printFixDetail(stderr, f)
		if err := confirmFixApply(stderr); err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
		}
	}
	res, err := client.ApplyFix(ctx, rest[0], params)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("apply fix: %w", err))
	}
	return finishRender(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "%s: %s\n", res.Outcome, res.Detail)
	})
}

func printFixes(out io.Writer, fixes []apiclient.Fix) {
	if len(fixes) == 0 {
		_, _ = fmt.Fprintln(out, "no fixes are needed right now")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tKIND\tSTATE\tRISK\tTITLE")
	for _, f := range fixes {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", f.ID, f.Kind, f.State, f.Risk, f.Title)
	}
	_ = tw.Flush()
}

func printFixDetail(out io.Writer, f apiclient.Fix) {
	_, _ = fmt.Fprintf(out, "%s (%s, %s)\n%s\ncheck: %s\n", f.Title, f.Kind, f.Risk, f.Why, f.Detail)
	for _, c := range f.Changes {
		_, _ = fmt.Fprintf(out, "  - %s %s\n", c.Label, c.Detail)
	}
	if f.Command != "" {
		_, _ = fmt.Fprintf(out, "run on the server:\n%s\n", f.Command)
	}
	if f.Blocked != "" {
		_, _ = fmt.Fprintln(out, f.Blocked)
	}
}

func confirmFixApply(stderr io.Writer) error {
	if f, ok := stdinSource.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // a terminal file descriptor always fits an int
		return errors.New("not a terminal: pass --yes to apply without confirmation")
	}
	_, _ = fmt.Fprint(stderr, "Apply this fix? [y/N] ")
	line, _ := bufio.NewReader(stdinSource).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return errors.New("cancelled")
	}
	return nil
}
