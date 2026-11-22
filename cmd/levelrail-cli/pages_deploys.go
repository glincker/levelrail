package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runPagesDeploys lists a site's deploys with the URL each one is reachable at.
func runPagesDeploys(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages deploys", "print the result as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: pages deploys requires a site name\n", prog)
		return exitUsage
	}
	name := fs.Arg(0)
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	deploys, err := client.ListStaticSiteDeploys(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list deploys of %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]any{"site": name, "deploys": deploys}, func() {
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "DEPLOY\tSTATUS\tLIVE\tSTARTED\tURL")
		for _, d := range deploys {
			live := ""
			if d.Live {
				live = "live"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", d.ID, d.Status, live, d.StartedAt, d.URL)
		}
		_ = tw.Flush()
	})
}

// runPagesPromote makes a retained deploy the one the site's domains serve.
func runPagesPromote(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages promote", "print the result as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: pages promote requires a site name and a deploy id\n", prog)
		return exitUsage
	}
	name, id := fs.Arg(0), fs.Arg(1)
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.PromoteStaticSiteDeploy(context.Background(), name, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("promote deploy %q of site %q: %w", id, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]any{"site": name, "promoted": id, "serving": res.Serving}, func() {
		_, _ = fmt.Fprintf(stdout, "site %q now serves deploy %s\n", name, id)
		printPagesServingNote(stdout, res.Serving)
	})
}

// printPagesServingNote tells the operator when the route is still switching over.
func printPagesServingNote(stdout io.Writer, serving string) {
	if serving == "pending" {
		_, _ = fmt.Fprintln(stdout, "the route is still switching over; it should serve the new files within moments")
	}
}
