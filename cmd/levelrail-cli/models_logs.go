package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// runModelsLogs implements "models logs <name>": GET
// /api/v1/models/{name}/logs, or the live stream with --follow.
func runModelsLogs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models logs", "print entries as a JSON array to stdout and nothing else", stderr)
	var since, from, to, query string
	var tail int
	var follow bool
	fs.StringVar(&since, "since", "", "how far back to search, e.g. \"1h\", \"30m\" (default: 1h)")
	fs.StringVar(&from, "from", "", "RFC3339 start of the search window (overrides --since)")
	fs.StringVar(&to, "to", "", "RFC3339 end of the search window (default: now)")
	fs.StringVar(&query, "q", "", "full-text search phrase")
	fs.IntVar(&tail, "tail", 0, "only show the last N entries (client-side)")
	fs.BoolVar(&follow, "follow", false, "stream new log lines live until Ctrl+C")
	fs.BoolVar(&follow, "f", false, "shorthand for --follow")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s models logs <name> [flags]\n  %[1]s models logs <name> --follow\n\nSearches a model engine's stored logs, or streams them live with --follow\n(model download and load progress shows up here).\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "models logs", "model name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if follow {
		if since != "" || from != "" || to != "" || query != "" || tail != 0 || of.Query != "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--follow is a live tail and cannot be combined with --since/--from/--to/--q/--tail/--query"))
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		err := client.StreamModelLogs(ctx, name, func(e logStreamEntry) error {
			if of.Format == outputJSON {
				return writeJSONLine(stdout, e)
			}
			_, err := fmt.Fprintf(stdout, "%s %s\n", e.Stream, e.Line)
			return err
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			return reportError(stdout, stderr, of.Format == outputJSON, fmt.Errorf("stream logs for model %q: %w", name, err))
		}
		return exitOK
	}

	if tail < 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--tail must not be negative"))
	}
	fromTime, toTime, err := resolveTimeRange(timeRangeFlags{since: since, from: from, to: to}, time.Now())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	entries, err := client.QueryModelLogs(context.Background(), name, fromTime, toTime, query)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("query logs for model %q: %w", name, err))
	}
	entries = tailEntries(entries, tail)
	if err := renderResult(stdout, of.Format, of.Query, entries, func() { printLogEntriesHuman(stdout, entries) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
