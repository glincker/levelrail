package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

var silenceDurationAliases = map[string]string{
	"1h": "1h", "1d": "1d", "1w": "1w", "30d": "30d", "forever": "forever",
}

func silenceUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s status silence <issue> --for 1h|1d|1w|30d|forever [--reason text] [--scope name] [--yes]
  %[1]s status silences
  %[1]s status unsilence <id>

Mutes one status issue so it leaves the header count, the Status page and
the attention list. A silenced issue stays visible under Silenced, with who
muted it and until when. <issue> is an item id from "%[1]s attention --json".
Forever, and any silence of a security or data-loss issue, needs --yes.
`, prog)
}

func runStatusSilence(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "status silence", "print the silence as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, silenceUsage(prog)) }
	dur := fs.String("for", "", "how long: 1h, 1d, 1w, 30d or forever")
	reason := fs.String("reason", "", "why this is a known non-concern")
	scope := fs.String("scope", "", "limit the silence to one node or app")
	yes := fs.Bool("yes", false, "confirm silencing forever or a security or data-loss issue")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 || silenceDurationAliases[*dur] == "" {
		_, _ = fmt.Fprint(stderr, silenceUsage(prog))
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	sil, err := client.CreateIssueSilence(context.Background(), apiclient.CreateIssueSilenceRequest{IssueKey: fs.Arg(0), Scope: *scope, Duration: *dur, Reason: *reason, Confirm: *yes})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("silence %s: %w", fs.Arg(0), err))
	}
	if err := renderResult(stdout, of.Format, of.Query, sil, func() {
		_, _ = fmt.Fprintf(stdout, "Silenced %s until %s (id %s)\n", sil.IssueKey, silenceUntilLabel(sil.Until), sil.ID)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runStatusSilences(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "status silences", "print silences as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, silenceUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListIssueSilences(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list silences: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, list, func() {
		if len(list) == 0 {
			_, _ = fmt.Fprintln(stdout, "No silences.")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tISSUE\tUNTIL\tBY\tREASON")
		for _, s := range list {
			until := silenceUntilLabel(s.Until)
			if s.Expired {
				until += " (expired)"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", s.ID, s.IssueKey, until, s.CreatedBy, s.Reason)
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runStatusUnsilence(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "status unsilence", "print the result as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, silenceUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprint(stderr, silenceUsage(prog))
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteIssueSilence(context.Background(), fs.Arg(0)); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("unsilence %s: %w", fs.Arg(0), err))
	}
	_, _ = fmt.Fprintf(stdout, "Unsilenced %s\n", fs.Arg(0))
	return exitOK
}

func silenceUntilLabel(until *time.Time) string {
	if until == nil {
		return "forever"
	}
	return until.Local().Format("2006-01-02 15:04")
}
