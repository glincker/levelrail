package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

const (
	cachePresetStatic = "static"
	purgeScopeTag     = "tag"
)

func cacheUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s cache status [<app>]
  %[1]s cache rules show <app> [--domain D]
  %[1]s cache rules set <app> (--preset static | --file cache.json) [--domain D]
  %[1]s cache rules clear <app> [--domain D]
  %[1]s cache purge <app> (--url /a?b=1 | --prefix /static | --tag NAME | --all) [--domain D]
  %[1]s cache history <app>

The response cache lives in the ingress. Only GET and HEAD are cached, and
responses with Set-Cookie, private, no-store or no-cache never are.
With no --domain a command covers every domain of the app.
`, prog)
}

// runCache implements "cache": the app-level view of the ingress response cache.
func runCache(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, cacheUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, cacheUsage(prog))
		return exitOK
	case "status":
		return runCacheStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "rules":
		return runCacheRules(prog, args[1:], stdout, stderr, lookupEnv)
	case "purge":
		return runCacheAppPurge(prog, args[1:], stdout, stderr, lookupEnv)
	case "history":
		return runCacheHistory(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown cache subcommand %q\n\n%s", prog, args[0], cacheUsage(prog))
	return exitUsage
}

type cacheCall struct {
	client *Client
	app    string
	domain string
	jsonO  bool
	of     outputFlags
	rest   []string
}

// parseCacheCall parses flags shared by every cache verb. wantApp is the
// positional count: 1 for a required app, 0 for none, -1 for zero or one.
func parseCacheCall(prog, label string, args []string, stderr io.Writer, lookupEnv func(string) (string, bool), wantApp int, define func(*flag.FlagSet)) (cacheCall, int, bool) {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	var domain string
	fs.StringVar(&domain, "domain", "", "limit to one of the app's domains")
	if define != nil {
		define(fs)
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, cacheUsage(prog)) }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return cacheCall{}, code, false
	}
	rest := fs.Args()
	if wantApp >= 0 && len(rest) != wantApp || wantApp < 0 && len(rest) > 1 {
		_, _ = fmt.Fprintf(stderr, "%s: %s takes %s\n\n", prog, label, map[bool]string{true: "an app name", false: "no arguments"}[wantApp != 0])
		fs.Usage()
		return cacheCall{}, exitUsage, false
	}
	c := cacheCall{client: apiClientFromFlags(prog, apiURL, token, profile, lookupEnv), domain: strings.ToLower(domain), jsonO: jsonOut, of: of, rest: rest}
	if len(rest) > 0 {
		c.app = rest[0]
	}
	return c, exitOK, true
}

func (c cacheCall) domains(cur apiclient.AppCacheResource) ([]string, error) {
	var out []string
	for _, d := range cur.Domains {
		if c.domain == "" || strings.EqualFold(d.Domain, c.domain) {
			out = append(out, d.Domain)
		}
	}
	if len(out) == 0 {
		return nil, newValidationError("app %q has no matching domain", c.app)
	}
	return out, nil
}

func pct(rate float64) string { return fmt.Sprintf("%.1f%%", rate*100) }

func runCacheStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCacheCall(prog, "cache status", args, stderr, lookupEnv, -1, nil)
	if !ok {
		return code
	}
	if c.app == "" {
		res, err := c.client.GetSystemCache(bg())
		if err != nil {
			return reportError(stdout, stderr, c.jsonO, fmt.Errorf("cache status: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
			_, _ = fmt.Fprintf(stdout, "cache: %d of %d bytes used, hit rate %s, %d entries\n", res.Limits.UsedBytes, res.Limits.MaxBytes, pct(res.Totals.HitRate), res.Totals.Entries)
			tw := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "APP\tDOMAINS ON\tREQUESTS\tHIT RATE\tENTRIES")
			for _, a := range res.Apps {
				_, _ = fmt.Fprintf(tw, "%s\t%d/%d\t%d\t%s\t%d\n", a.App, a.EnabledDomains, a.Domains, a.Requests, pct(a.HitRate), a.Entries)
			}
			_ = tw.Flush()
		})
	}
	res, err := c.client.GetAppCache(bg(), c.app)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("cache status for app %q: %w", c.app, err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		_, _ = fmt.Fprintf(stdout, "%s: hit rate %s, %d hits, %d stale, %d misses, %d bypasses, %d bytes saved\n",
			res.App, pct(res.Totals.HitRate), res.Totals.Hits, res.Totals.Stale, res.Totals.Misses, res.Totals.Bypasses, res.Totals.HitBytes)
		for _, d := range res.Domains {
			state := "off"
			if d.Enabled {
				state = "on"
			}
			_, _ = fmt.Fprintf(stdout, "  %s: cache %s, %d rules, hit rate %s\n", d.Domain, state, d.Rules, pct(d.HitRate))
		}
	})
}

func runCacheRules(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, cacheUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "show":
		return runCacheRulesShow(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runCacheRulesSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runCacheRulesClear(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown cache rules subcommand %q\n\n%s", prog, args[0], cacheUsage(prog))
	return exitUsage
}

func runCacheRulesShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCacheCall(prog, "cache rules show", args, stderr, lookupEnv, 1, nil)
	if !ok {
		return code
	}
	cur, err := c.client.GetAppCache(bg(), c.app)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("cache rules for app %q: %w", c.app, err))
	}
	domains, err := c.domains(cur)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, err)
	}
	out := map[string]apiclient.CachePolicyResource{}
	for _, d := range domains {
		res, gerr := c.client.GetDomainCache(bg(), c.app, d)
		if gerr != nil {
			return reportError(stdout, stderr, c.jsonO, fmt.Errorf("cache rules for domain %q: %w", d, gerr))
		}
		out[d] = res
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, out, func() {
		for _, d := range domains {
			spec := out[d].Spec
			_, _ = fmt.Fprintf(stdout, "%s: cache %s, %d rules\n", d, map[bool]string{true: "on", false: "off"}[spec.Enabled], len(spec.Rules))
			for _, r := range spec.Rules {
				verb := fmt.Sprintf("ttl %ds", r.TTLSeconds)
				if r.Never {
					verb = "never cache"
				}
				_, _ = fmt.Fprintf(stdout, "  %s %s: %s\n", r.Match.Kind, r.Match.Path, verb)
			}
		}
	})
}

func runCacheRulesSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var preset, file string
	c, code, ok := parseCacheCall(prog, "cache rules set", args, stderr, lookupEnv, 1, func(fs *flag.FlagSet) {
		fs.StringVar(&preset, "preset", "", "apply a preset: static (common static file types)")
		fs.StringVar(&file, "file", "", "replace the cache policy with this JSON file")
	})
	if !ok {
		return code
	}
	if (preset == "") == (file == "") || (preset != "" && preset != cachePresetStatic) {
		return usageError(stderr, prog, "choose exactly one of --preset static or --file")
	}
	if preset != "" {
		res, err := c.client.ApplyAppCachePreset(bg(), c.app, c.domain)
		if err != nil {
			return reportError(stdout, stderr, c.jsonO, fmt.Errorf("apply cache preset to app %q: %w", c.app, err))
		}
		return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
			for _, d := range res.Domains {
				state := "already set"
				if d.Applied {
					state = "applied"
				}
				_, _ = fmt.Fprintf(stdout, "%s: static assets preset %s, %d rules\n", d.Domain, state, d.Rules)
			}
		})
	}
	var spec trafficpolicy.Cache
	if err := readStrictJSON(file, &spec); err != nil {
		return reportError(stdout, stderr, c.jsonO, newValidationError("%v", err))
	}
	cur, err := c.client.GetAppCache(bg(), c.app)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("cache rules for app %q: %w", c.app, err))
	}
	domains, err := c.domains(cur)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, err)
	}
	out := map[string]apiclient.CachePolicyResource{}
	for _, d := range domains {
		res, serr := c.client.SetDomainCache(bg(), c.app, d, spec)
		if serr != nil {
			return reportError(stdout, stderr, c.jsonO, fmt.Errorf("set cache for domain %q: %w", d, serr))
		}
		out[d] = res
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, out, func() {
		for _, d := range domains {
			_, _ = fmt.Fprintf(stdout, "%s: cache policy saved, %d rules\n", d, len(out[d].Spec.Rules))
		}
	})
}

func runCacheRulesClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCacheCall(prog, "cache rules clear", args, stderr, lookupEnv, 1, nil)
	if !ok {
		return code
	}
	cur, err := c.client.GetAppCache(bg(), c.app)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("cache rules for app %q: %w", c.app, err))
	}
	domains, err := c.domains(cur)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, err)
	}
	for _, d := range domains {
		if _, cerr := c.client.ClearDomainCache(bg(), c.app, d); cerr != nil {
			return reportError(stdout, stderr, c.jsonO, fmt.Errorf("clear cache for domain %q: %w", d, cerr))
		}
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, map[string][]string{"cleared": domains}, func() {
		_, _ = fmt.Fprintf(stdout, "cache turned off and rules removed for %s\n", strings.Join(domains, ", "))
	})
}

func runCacheAppPurge(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var urlPath, prefix, tag string
	var all bool
	c, code, ok := parseCacheCall(prog, "cache purge", args, stderr, lookupEnv, 1, func(fs *flag.FlagSet) {
		fs.StringVar(&urlPath, "url", "", "one path and query, e.g. /page?x=1 (every Vary variant)")
		fs.StringVar(&prefix, "prefix", "", "every path under this prefix")
		fs.StringVar(&tag, "tag", "", "every object the app tagged with this cache tag")
		fs.BoolVar(&all, "all", false, "everything cached for the app")
	})
	if !ok {
		return code
	}
	req := apiclient.AppCachePurgeRequest{Domain: c.domain}
	chosen := 0
	for _, pick := range []struct {
		set          bool
		scope, value string
	}{{urlPath != "", purgeScopeURL, urlPath}, {prefix != "", purgeScopePrefix, prefix}, {tag != "", purgeScopeTag, tag}, {all, purgeScopeAll, ""}} {
		if pick.set {
			chosen++
			req.Scope, req.Value = pick.scope, pick.value
		}
	}
	if chosen != 1 {
		return usageError(stderr, prog, "choose exactly one of --url, --prefix, --tag or --all")
	}
	res, err := c.client.PurgeAppCache(bg(), c.app, req)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("purge cache for app %q: %w", c.app, err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		_, _ = fmt.Fprintf(stdout, "%d cached responses purged for %s\n", res.Removed, res.App)
	})
}

func runCacheHistory(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCacheCall(prog, "cache history", args, stderr, lookupEnv, 1, nil)
	if !ok {
		return code
	}
	res, err := c.client.ListAppCachePurges(bg(), c.app)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("cache history for app %q: %w", c.app, err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		if len(res.Purges) == 0 {
			_, _ = fmt.Fprintln(stdout, "no purges recorded")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "WHEN\tWHO\tDOMAIN\tSCOPE\tVALUE\tREMOVED")
		for _, p := range res.Purges {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\n", p.At, p.Actor, p.Domain, p.Scope, p.Value, p.Removed)
		}
		_ = tw.Flush()
	})
}
