package main

import (
	"context"
	"fmt"
	"io"
)

// runPagesAnalytics implements "pages analytics <site> [--range 24h|7d|30d]".
func runPagesAnalytics(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages analytics", "print the analytics as JSON to stdout and nothing else", stderr)
	var rng string
	fs.StringVar(&rng, "range", "24h", "time range: 24h, 7d or 30d")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "pages analytics", "site name")
	if !ok {
		return exitUsage
	}
	if rng != "24h" && rng != "7d" && rng != "30d" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--range must be 24h, 7d or 30d, got %q", rng))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.StaticSiteAnalytics(context.Background(), name, rng)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("analytics for site %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printPagesAnalytics(stdout, res) })
}

func printPagesAnalytics(out io.Writer, res map[string]any) {
	_, _ = fmt.Fprintf(out, "requests: %v\n", res["requests"])
	for _, sec := range []struct{ key, title string }{{"top_pages", "top pages"}, {"top_referrers", "top referrers"}} {
		_, _ = fmt.Fprintf(out, "%s:\n", sec.title)
		rows, _ := res[sec.key].([]any)
		for _, r := range rows {
			m, _ := r.(map[string]any)
			_, _ = fmt.Fprintf(out, "  %6v  %v\n", m["count"], m["label"])
		}
	}
	_, _ = fmt.Fprintln(out, "status codes:")
	rows, _ := res["status"].([]any)
	for _, r := range rows {
		m, _ := r.(map[string]any)
		_, _ = fmt.Fprintf(out, "  %6v  %v\n", m["count"], m["code"])
	}
}
