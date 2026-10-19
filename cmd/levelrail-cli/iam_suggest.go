package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runIAMSuggest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "iam suggest", "print the suggestions as JSON to stdout and nothing else", stderr)
	var days int
	fs.IntVar(&days, "days", 30, "audit window in days (1 to 365)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, iamSuggestUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.IAMSuggestions(context.Background(), days)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("iam suggestions: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printSuggestions(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printSuggestions(out io.Writer, s apiclient.IAMSuggestions) {
	_, _ = fmt.Fprintf(out, "Token usage over the last %d day(s)\n", s.WindowDays)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TOKEN\tSTATUS\tRISK\tGRANTED\tSUGGESTED")
	for _, t := range s.Tokens {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", t.Name, t.Status, t.Risk, strings.Join(t.Granted, ","), strings.Join(t.SuggestedAbilities, ","))
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintln(out, s.Note)
}

func iamSuggestUsage(prog string) string {
	return fmt.Sprintf("Usage:\n  %[1]s iam suggest [--days N] [flags]\n\nSuggest narrower abilities for each token from what the audit log shows it used.\n\nFlags:\n  --days int          audit window in days (default 30)\n", prog) +
		fmt.Sprintf(iamAPIFlagsHelp, envAPIToken, envAPIURL, defaultAPIURL)
}
