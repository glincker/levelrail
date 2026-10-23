package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsAbuseUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps abuse get <app> [flags]        show the policy, its status and what it counted lately
  %[1]s apps abuse set <app> [flags]        change the policy; a flag changes only its own field
  %[1]s apps abuse dry-run <app> [flags]    replay recent traffic against the saved or a draft policy

set and dry-run flags:
  --mode off|log_only|enforce   log_only counts what would be blocked and blocks nothing (default off)
  --rps N                       sustained requests per second per client IP, 0 turns the limit off
  --burst N                     bucket size (default two seconds of --rps)
  --deny-cidr LIST              comma separated IPs or CIDR ranges to turn away, "none" clears the list
  --deny-agent LIST             comma separated User-Agent substrings to turn away, "none" clears the list
  --require-user-agent=BOOL     turn away requests with no User-Agent
  --suggested                   start from the suggestion for this app's recent peak (set, dry-run)

dry-run flags:
  --minutes N                   how far back to replay (default from the instance)

Exceeding the limit answers 429 with Retry-After. Move from log_only to
enforce once dry-run shows nothing you want to keep would be blocked. The
control plane keeps recent requests in memory only, so a dry run covers what
it still holds.
%[2]s`, prog, commonFlagsHelp)
}

// runAppsAbuse dispatches "apps abuse get|set|dry-run".
func runAppsAbuse(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsAbuseUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsAbuseUsage(prog))
		return exitOK
	case "get", "set", "dry-run":
		return runAppsAbuseVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown apps abuse subcommand %q\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, appsAbuseUsage(prog))
	return exitUsage
}

type abuseFlags struct {
	mode, cidrs, agents *string
	rps                 *float64
	burst, minutes      *int
	requireUA           *bool
	suggested           *bool
}

func bindAbuseFlags(fs *flag.FlagSet) abuseFlags {
	return abuseFlags{
		mode:      fs.String("mode", "", "off, log_only or enforce"),
		rps:       fs.Float64("rps", 0, "requests per second per client IP"),
		burst:     fs.Int("burst", 0, "bucket size"),
		cidrs:     fs.String("deny-cidr", "", "comma separated IPs or CIDR ranges"),
		agents:    fs.String("deny-agent", "", "comma separated User-Agent substrings"),
		requireUA: fs.Bool("require-user-agent", false, "turn away requests with no User-Agent"),
		minutes:   fs.Int("minutes", 0, "minutes of traffic to replay (dry-run)"),
		suggested: fs.Bool("suggested", false, "start from the suggested rate for this app"),
	}
}

func runAppsAbuseVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps abuse " + verb
	var f abuseFlags
	c, code, ok := parseCLICall(prog, label, appsAbuseUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) { f = bindAbuseFlags(fs) })
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	ctx := context.Background()
	cur, err := c.client.GetAppAbuse(ctx, app)
	if err != nil {
		return c.fail(fmt.Errorf("read abuse policy for %q: %w", app, err))
	}
	if verb == "get" {
		return c.render(cur, func() { printAppAbuse(c.stdout, cur) })
	}
	draft := applyAbuseFlags(cur, c.fs, f)
	if verb == "set" {
		res, err := c.client.SetAppAbuse(ctx, app, draft)
		if err != nil {
			return c.fail(fmt.Errorf("set abuse policy for %q: %w", app, err))
		}
		return c.render(res, func() { printAppAbuse(c.stdout, res) })
	}
	req := apiclient.AbuseDryRunRequest{Policy: &draft, Minutes: *f.minutes}
	res, err := c.client.DryRunAppAbuse(ctx, app, req)
	if err != nil {
		return c.fail(fmt.Errorf("dry-run abuse policy for %q: %w", app, err))
	}
	return c.render(res, func() { printAbuseDryRun(c.stdout, res) })
}

// applyAbuseFlags starts from the saved policy, or the suggestion for an app
// with none, so a flag changes only its own field and nothing starts blank.
func applyAbuseFlags(cur apiclient.AppAbuse, fs *flag.FlagSet, f abuseFlags) apiclient.AbusePolicy {
	p := cur.Policy
	if (!cur.Configured && p.RPS == 0) || *f.suggested {
		p.RPS, p.Burst = cur.Suggestion.RPS, cur.Suggestion.Burst
	}
	fs.Visit(func(fl *flag.Flag) {
		switch fl.Name {
		case "mode":
			p.Mode = *f.mode
		case "rps":
			p.RPS = *f.rps
			if !flagWasSet(fs, "burst") {
				p.Burst = 0
			}
		case "burst":
			p.Burst = *f.burst
		case "deny-cidr":
			p.DenyCIDRs = splitAbuseList(*f.cidrs)
		case "deny-agent":
			p.DenyUserAgents = splitAbuseList(*f.agents)
		case "require-user-agent":
			p.RequireUserAgent = *f.requireUA
		}
	})
	return p
}

func splitAbuseList(s string) []string {
	if strings.EqualFold(strings.TrimSpace(s), "none") {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func printAppAbuse(out io.Writer, a apiclient.AppAbuse) {
	p := a.Policy
	_, _ = fmt.Fprintf(out, "app: %s\nmode: %s\n", a.App, p.Mode)
	if p.RPS > 0 {
		_, _ = fmt.Fprintf(out, "rate limit: %g requests/s per client, burst %d\n", p.RPS, p.Burst)
	} else {
		_, _ = fmt.Fprintln(out, "rate limit: off")
	}
	_, _ = fmt.Fprintf(out, "deny cidr: %s\ndeny user agent: %s\nrequire user agent: %t\n", listOrNone(p.DenyCIDRs), listOrNone(p.DenyUserAgents), p.RequireUserAgent)
	_, _ = fmt.Fprintf(out, "status: %s (%s)\n", a.Condition.Reason, a.Condition.Message)
	c := a.Counters
	_, _ = fmt.Fprintf(out, "last %d min: allowed %.0f  would block %.0f  blocked %.0f  fail-open %.0f\n", a.CountersWindowMinutes, c.Allowed, c.WouldBlock, c.Blocked, c.FailOpen)
	if !a.Configured {
		_, _ = fmt.Fprintf(out, "suggested start: %g requests/s, burst %d (peak %.1f requests/s)\n", a.Suggestion.RPS, a.Suggestion.Burst, a.Suggestion.PeakRPS)
	}
}

func listOrNone(l []string) string {
	if len(l) == 0 {
		return "none"
	}
	return strings.Join(l, ", ")
}

func printAbuseDryRun(out io.Writer, r apiclient.AbuseDryRun) {
	_, _ = fmt.Fprintf(out, "%s\n", r.Note)
	_, _ = fmt.Fprintf(out, "replayed %d requests over %.1f of the last %.0f minutes\n", r.Requests, r.CoveredMinutes, r.WindowMinutes)
	_, _ = fmt.Fprintf(out, "would have blocked %d (%.1f%%)\n", r.WouldBlock, r.Percent)
	for _, b := range r.ByReason {
		if b.Count > 0 {
			_, _ = fmt.Fprintf(out, "  %s: %d\n", b.Reason, b.Count)
		}
	}
	if len(r.TopClients) == 0 {
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "\nCLIENT\tREQUESTS\tWOULD BLOCK\tREASON")
	for _, c := range r.TopClients {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%s\n", c.Client, c.Requests, c.WouldBlock, c.TopReason)
	}
	_ = tw.Flush()
}
