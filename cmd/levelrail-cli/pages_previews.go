package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runPagesPreviews lists the latest preview deploy of each branch of a site.
func runPagesPreviews(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages previews", "print the result as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: pages previews requires a site name\n", prog)
		return exitUsage
	}
	name := fs.Arg(0)
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	previews, err := client.ListStaticSitePreviews(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list previews of %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]any{"site": name, "previews": previews}, func() {
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "BRANCH\tSTATUS\tCOMMIT\tUPDATED\tURL")
		for _, p := range previews {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%.7s\t%s\t%s\n", p.Branch, p.Status, p.CommitSHA, p.UpdatedAt, p.URL)
		}
		_ = tw.Flush()
	})
}
