package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type verifyArgs struct {
	prog, apiURL, token, profile string
	jsonOut                      bool
	of                           outputFlags
	expectIPs                    []string
}

// runDomainsVerify implements "domains check --cutover <domain>...".
func runDomainsVerify(fs *flag.FlagSet, stdout, stderr io.Writer, a verifyArgs, lookupEnv func(string) (string, bool)) int {
	domains := fs.Args()
	if len(domains) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s domains check --cutover requires at least one domain\n", a.prog)
		return exitUsage
	}
	client := apiClientFromFlags(a.prog, a.apiURL, a.token, a.profile, lookupEnv)
	reports := make([]apiclient.DomainVerifyReport, 0, len(domains))
	failed := false
	for _, d := range domains {
		rep, err := client.DomainVerify(context.Background(), strings.ToLower(d), a.expectIPs)
		if err != nil {
			return reportError(stdout, stderr, a.jsonOut, fmt.Errorf("verify %q: %w", d, err))
		}
		failed = failed || rep.Verdict == "FAIL"
		reports = append(reports, rep)
	}
	if code := writeScheduledTaskResult(stdout, stderr, a.of, reports, func() { printVerifyHuman(stdout, reports) }); code != exitOK {
		return code
	}
	if failed {
		return exitCheckFailed
	}
	return exitOK
}

func printVerifyHuman(out io.Writer, reports []apiclient.DomainVerifyReport) {
	for i, r := range reports {
		if i > 0 {
			_, _ = fmt.Fprintln(out)
		}
		_, _ = fmt.Fprintf(out, "%s  %s\n", r.Verdict, r.Summary)
		if len(r.ServerIPs) > 0 {
			_, _ = fmt.Fprintf(out, "this host: %s\n", strings.Join(r.ServerIPs, ", "))
		}
		for _, l := range r.Lines {
			_, _ = fmt.Fprintf(out, "  %-4s %s: %s\n", l.Verdict, l.Title, l.Detail)
			if l.Verdict != "PASS" {
				_, _ = fmt.Fprintf(out, "       next: %s\n", l.Next)
			}
		}
	}
}
