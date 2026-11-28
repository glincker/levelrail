package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/glincker/levelrail/internal/apiclient"
)

func runDNSWhoAnswers(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseDBAccess(prog, "dns who-answers", "dns who-answers <domain>", args, 1, "a domain", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	a, err := c.client.DNSAuthority(context.Background(), c.rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("who answers for %s: %w", c.rest[0], err))
	}
	return c.result(a, func() { printDNSAuthority(stdout, a) })
}

func printDNSAuthority(out io.Writer, a apiclient.DNSAuthority) {
	_, _ = fmt.Fprintf(out, "%s\n", a.Domain)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME SERVER\tPROVIDER")
	for _, ns := range a.NameServers {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", ns.Name, providerLabel(ns.Provider))
	}
	_ = tw.Flush()
	answers := "none"
	if len(a.Answers) > 0 {
		answers = strings.Join(a.Answers, ", ")
	}
	_, _ = fmt.Fprintf(out, "answers: %s\n", answers)
	if a.CNAME != "" {
		_, _ = fmt.Fprintf(out, "alias of: %s\n", a.CNAME)
	}
	_, _ = fmt.Fprintf(out, "proxied: %s, points at this server: %s\n", answerYesNo(a.Proxied), answerYesNo(a.PointsHere))
	if a.TTL != nil {
		_, _ = fmt.Fprintf(out, "TTL: %s\n", a.TTL.String())
	}
	if len(a.Risks) == 0 {
		_, _ = fmt.Fprintln(out, "risks: none found")
		return
	}
	_, _ = fmt.Fprintln(out, "risks:")
	for _, r := range a.Risks {
		_, _ = fmt.Fprintf(out, "  - %s\n", r)
	}
}

func providerLabel(p string) string {
	if p == "this_host" {
		return "this server"
	}
	return p
}

func answerYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
