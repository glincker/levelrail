package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runSystemSlowStatements implements "system slow-statements".
func runSystemSlowStatements(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "system slow-statements", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s system slow-statements\n\nSlowest statements the control plane ran against its own database (text only, never values).\nTurn recording on with the perf.statement_timing setting.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetSlowStatements(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("slow statements: %w", err))
	}
	return finishRender(stdout, stderr, of, res, func() {
		state := "off"
		if res.Enabled {
			state = "on"
		}
		_, _ = fmt.Fprintf(stdout, "statement timing %s, threshold %d ms, keeping %d\n", state, res.SlowThresholdMS, res.RingSize)
		if len(res.Statements) == 0 {
			_, _ = fmt.Fprintln(stdout, "no slow statements recorded")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "MAX_MS\tLAST_MS\tCOUNT\tSTATEMENT")
		for _, s := range res.Statements {
			_, _ = fmt.Fprintf(tw, "%.1f\t%.1f\t%d\t%s\n", s.MaxMS, s.LastMS, s.Count, s.SQL)
		}
		_ = tw.Flush()
	})
}
