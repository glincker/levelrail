package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	watchStateLive    = "live"
	watchStateStalled = "stalled"
	// defaultWaitTimeout and defaultWaitPoll are flag defaults, both overridable.
	defaultWaitTimeout = 10 * time.Minute
	defaultWaitPoll    = 3 * time.Second
)

func runDomainsWatch(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] != "list" {
		_, _ = fmt.Fprint(stderr, domainsWatchUsage(prog))
		return exitUsage
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains watch list", "print watched domains as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsWatchUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	rows, err := client.ListDomainWatches(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list watched domains: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, rows, func() { printDomainWatches(stdout, rows) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printDomainWatches(out io.Writer, rows []apiclient.DomainWatch) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "no domains are being watched")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DOMAIN\tAPP\tSTATE\tCHECKS\tSINCE")
	for _, r := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", r.Domain, r.App, r.State, r.Attempts, r.Since.Format(time.RFC3339))
	}
	_ = tw.Flush()
}

// runDomainsWait polls the watcher snapshot and prints each state change; exit 0 live, 1 stalled or timeout.
func runDomainsWait(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains wait", "print each state change as a JSON line", stderr)
	var timeout, poll time.Duration
	fs.DurationVar(&timeout, "timeout", defaultWaitTimeout, "give up after this long")
	fs.DurationVar(&poll, "poll", defaultWaitPoll, "how often to ask the control plane")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsWaitUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains wait", "a domain", 1)
	if !ok {
		return exitUsage
	}
	domain := strings.ToLower(rest[0])
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	last := ""
	for {
		w, err := client.GetDomainWatch(ctx, domain)
		var apiErr *apiError
		switch {
		case ctx.Err() != nil:
			_, _ = fmt.Fprintf(stderr, "timed out after %s waiting for %s (last state: %s)\n", timeout, domain, orDash(last))
			return exitCheckFailed
		case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound:
			_, _ = fmt.Fprintf(stderr, "%s is not being watched: it is already live, or it is not added to an app yet\n", domain)
			return exitCheckFailed
		case err != nil:
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("watch %q: %w", domain, err))
		}
		if w.State != last {
			last = w.State
			printWaitState(stdout, jsonOut, w)
		}
		switch w.State {
		case watchStateLive:
			return exitOK
		case watchStateStalled:
			_, _ = fmt.Fprintf(stderr, "%s is stalled: DNS has not settled within the watch limit; run %s domains diagnose %s\n", domain, prog, domain)
			return exitCheckFailed
		}
		select {
		case <-ctx.Done():
		case <-time.After(poll):
		}
	}
}

func printWaitState(out io.Writer, jsonOut bool, w apiclient.DomainWatch) {
	if jsonOut {
		_, _ = fmt.Fprintf(out, "{\"domain\":%q,\"state\":%q}\n", w.Domain, w.State)
		return
	}
	_, _ = fmt.Fprintf(out, "%s  %s  %s\n", time.Now().Format("15:04:05"), w.Domain, strings.ReplaceAll(w.State, "_", " "))
}

func domainsWatchUsage(prog string) string {
	return fmt.Sprintf("Usage:\n  %[1]s domains watch list [flags]\n\nLists the domains the control plane is following until DNS and the certificate are ready.\n", prog)
}

func domainsWaitUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains wait <domain> [--timeout 10m] [--poll 3s] [flags]

Prints each state change (waiting dns, propagating, waiting certificate, live).
Exits 0 when the domain is live, 1 when it stalls or the timeout passes.
`, prog)
}
