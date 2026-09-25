package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runLBList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "lb list", "print the list as JSON to stdout and nothing else", stderr)
	state := fs.String("state", "", "only balancers in this state: balancing, degraded or none")
	search := fs.String("search", "", "only apps whose name contains this text")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s lb list [--state S] [--search Q] [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "%s: lb list takes no arguments\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.ListLoadBalancers(context.Background(), apiclient.LoadBalancerListParams{State: *state, Search: *search})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list load balancers: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printLBList(stdout, res) })
}

func printLBList(out io.Writer, res apiclient.LoadBalancerList) {
	if len(res.Items) == 0 {
		_, _ = fmt.Fprintln(out, "no load balancers configured")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "APP\tALGORITHM\tSTATE\tUPSTREAMS")
	for _, it := range res.Items {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d/%d healthy\n", it.App, it.Algorithm, it.State, it.UpstreamsHealthy, it.UpstreamsTotal)
	}
	_ = tw.Flush()
	if res.Total > len(res.Items) {
		_, _ = fmt.Fprintf(out, "showing %d of %d\n", len(res.Items), res.Total)
	}
}
