package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runSystemDBSpace implements "system db-space" and "system db-space compact [--force]".
func runSystemDBSpace(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	compact := len(args) > 0 && args[0] == "compact"
	label := "system db-space"
	if compact {
		args = args[1:]
		label += " compact"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	force := fs.Bool("force", false, "compact: run even when little space is free")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s system db-space\n  %[1]s system db-space compact [--force]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	if compact {
		res, err := client.CompactDB(ctx, *force)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("compact database: %w", err))
		}
		return finishRender(stdout, stderr, of, res, func() {
			if !res.Compacted {
				_, _ = fmt.Fprintf(stdout, "not compacted: %s\n", res.Reason)
				return
			}
			_, _ = fmt.Fprintf(stdout, "compacted: %d bytes before, %d after\n", res.BytesBefore, res.BytesAfter)
		})
	}
	res, err := client.GetDBSpace(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("database space: %w", err))
	}
	return finishRender(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "total %d bytes, reclaimable %d bytes (%d%% free)\n", res.TotalBytes, res.ReclaimableBytes, res.FreePercent)
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "NAME\tBYTES")
		for _, t := range res.Tables {
			_, _ = fmt.Fprintf(tw, "%s\t%d\n", t.Name, t.Bytes)
		}
		_ = tw.Flush()
	})
}

// runSystemProcesses implements "system processes".
func runSystemProcesses(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "system processes", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s system processes\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetSystemProcesses(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("system processes: %w", err))
	}
	return finishRender(stdout, stderr, of, res, func() {
		if !res.Supported {
			_, _ = fmt.Fprintln(stdout, "process table is only available on a Linux control plane host")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "PID\tNAME\tRSS BYTES\tTHREADS")
		for _, p := range res.Processes {
			_, _ = fmt.Fprintf(tw, "%d\t%s\t%d\t%d\n", p.PID, p.Name, p.RSSBytes, p.Threads)
		}
		_ = tw.Flush()
	})
}
