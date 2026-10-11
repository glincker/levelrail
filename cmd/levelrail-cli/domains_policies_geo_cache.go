package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func runDomainsGeo(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return dispatchPolicy(prog, "geo", args, stdout, stderr, lookupEnv, func(args []string) policyVerb {
		switch args[0] {
		case "show":
			return runGeoShow
		case "set":
			return runGeoSet
		case "clear":
			return runGeoClear
		case "lookup":
			return runGeoLookup
		}
		return nil
	}, domainsGeoUsage(prog))
}

func domainsGeoUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains geo show <app> <domain>
  %[1]s domains geo set <app> <domain> --mode allow|deny --countries US,CA [--action block|redirect|error_page] [flags]
  %[1]s domains geo clear <app> <domain>
  %[1]s domains geo lookup <ip>

Country comes from a trusted proxy's header (APP_GEOIP_COUNTRY_HEADER,
default CF-IPCountry, only from APP_INGRESS_TRUSTED_PROXIES) or a local MMDB
file (APP_GEOIP_DB). Private and loopback addresses are never blocked.
`, prog)
}

func geoResult(stdout, stderr io.Writer, inv policyInvocation, res apiclient.GeoPolicyResource, err error) int {
	if err != nil {
		return reportError(stdout, stderr, inv.jsonOut, fmt.Errorf("geo rule for domain %q: %w", inv.domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, inv.of, res, func() {
		if res.Spec == nil {
			_, _ = fmt.Fprintf(stdout, "%s: no geo rule, every country is allowed\n", res.Domain)
			return
		}
		g := res.Spec
		_, _ = fmt.Fprintf(stdout, "%s: %s %s, action %s, unknown %s, %d exempt\n", res.Domain, g.Mode, strings.Join(g.Countries, ","), g.Action, g.UnknownMode(), len(g.Exempt))
	})
}

func runGeoShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains geo show", argsHelp: "<app> <domain>", summary: "Shows a domain's country rule."})
	if !ok {
		return code
	}
	res, err := inv.client.GetDomainGeo(bg(), inv.app, inv.domain)
	return geoResult(stdout, stderr, inv, res, err)
}

func runGeoSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var g trafficpolicy.Geo
	var countries, exempt, bodyFile string
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains geo set", argsHelp: "<app> <domain>", summary: "Sets a domain's country allow or deny list.",
		define: func(fs *flag.FlagSet) {
			fs.StringVar(&g.Mode, "mode", "", "allow (only these countries) or deny (all but these) (required)")
			fs.StringVar(&countries, "countries", "", "comma separated ISO 3166 codes, e.g. US,CA (required)")
			fs.StringVar(&g.Action, "action", trafficpolicy.GeoActionBlock, "block, redirect or error_page")
			fs.StringVar(&g.RedirectURL, "redirect-url", "", "target for --action redirect")
			fs.StringVar(&bodyFile, "body-file", "", "HTML file for --action error_page")
			fs.IntVar(&g.StatusCode, "status", 0, "403 (default), 404 or 451")
			fs.StringVar(&exempt, "exempt", "", "comma separated IPs or CIDRs that always pass")
			fs.StringVar(&g.Unknown, "unknown", "", "allow (default) or block requests with no known country")
		},
	})
	if !ok {
		return code
	}
	g.Countries, g.Exempt = splitCSVUpper(countries), splitCSV(exempt)
	if bodyFile != "" {
		data, err := readFileString(bodyFile)
		if err != nil {
			return reportError(stdout, stderr, inv.jsonOut, err)
		}
		g.Body = data
	}
	res, err := inv.client.SetDomainGeo(bg(), inv.app, inv.domain, g)
	return geoResult(stdout, stderr, inv, res, err)
}

func runGeoClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains geo clear", argsHelp: "<app> <domain>", summary: "Removes the country rule."})
	if !ok {
		return code
	}
	res, err := inv.client.ClearDomainGeo(bg(), inv.app, inv.domain)
	return geoResult(stdout, stderr, inv, res, err)
}

func runGeoLookup(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains geo lookup", policyJSONUsage, stderr)
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains geo lookup", "an IP address", 1)
	if !ok {
		return exitUsage
	}
	res, err := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv).GeoIPLookup(bg(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("geoip lookup: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "sources: %s\n", strings.Join(res.Status.Sources, ", "))
		country := res.Country
		if country == "" {
			country = "unknown"
		}
		_, _ = fmt.Fprintf(stdout, "%s: %s", res.IP, country)
		if res.Note != "" {
			_, _ = fmt.Fprintf(stdout, " (%s)", res.Note)
		}
		_, _ = fmt.Fprintln(stdout)
	})
}

func runDomainsCache(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return dispatchPolicy(prog, "cache", args, stdout, stderr, lookupEnv, func(args []string) policyVerb {
		switch args[0] {
		case "show":
			return runCacheShow
		case "set":
			return runCacheSet
		case "add":
			return runCacheAdd
		case "purge":
			return runCachePurge
		case "stats":
			return runCacheStats
		case "clear":
			return runCacheClear
		}
		return nil
	}, domainsCacheUsage(prog))
}

func domainsCacheUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains cache show <app> <domain>
  %[1]s domains cache add <app> <domain> --path /static --ttl 300 [flags]
  %[1]s domains cache set <app> <domain> --file cache.json
  %[1]s domains cache purge <app> <domain> (--url /a?b=1 | --prefix /static | --all)
  %[1]s domains cache stats <app> <domain>
  %[1]s domains cache clear <app> <domain>

Only GET and HEAD are cached. Requests with Authorization or a Cookie and
responses with Set-Cookie, private, no-store or no-cache are never cached.
Responses carry X-Cache: HIT, MISS, STALE or BYPASS.
`, prog)
}

func cacheResult(stdout, stderr io.Writer, inv policyInvocation, res apiclient.CachePolicyResource, err error) int {
	if err != nil {
		return reportError(stdout, stderr, inv.jsonOut, fmt.Errorf("cache for domain %q: %w", inv.domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, inv.of, res, func() {
		state := "off"
		if res.Spec.Enabled {
			state = "on"
		}
		_, _ = fmt.Fprintf(stdout, "%s: cache %s, %d rules\n", res.Domain, state, len(res.Spec.Rules))
		for i, r := range res.Spec.Rules {
			_, _ = fmt.Fprintf(stdout, "  %d. %s ttl=%ds override=%t\n", i+1, r.Match.Describe(), r.TTLSeconds, r.OverrideUpstream)
		}
	})
}

func runCacheShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains cache show", argsHelp: "<app> <domain>", summary: "Shows a domain's cache rules."})
	if !ok {
		return code
	}
	res, err := inv.client.GetDomainCache(bg(), inv.app, inv.domain)
	return cacheResult(stdout, stderr, inv, res, err)
}

func runCacheSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var file string
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains cache set", argsHelp: "<app> <domain>", summary: "Replaces the cache policy with a JSON file.",
		define: func(fs *flag.FlagSet) { fs.StringVar(&file, "file", "", "JSON file (required)") },
	})
	if !ok {
		return code
	}
	if file == "" {
		return usageError(stderr, prog, "--file is required")
	}
	var spec trafficpolicy.Cache
	if err := readStrictJSON(file, &spec); err != nil {
		return reportError(stdout, stderr, inv.jsonOut, err)
	}
	res, err := inv.client.SetDomainCache(bg(), inv.app, inv.domain, spec)
	return cacheResult(stdout, stderr, inv, res, err)
}

func runCacheAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var r trafficpolicy.CacheRule
	var statuses, vary, methods string
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains cache add", argsHelp: "<app> <domain>", summary: "Adds one cache rule and turns caching on.",
		define: func(fs *flag.FlagSet) {
			fs.StringVar(&r.Match.Path, "path", "", "path to cache (required)")
			fs.StringVar(&r.Match.Kind, "kind", trafficpolicy.MatchPrefix, "prefix, exact or regex")
			fs.StringVar(&methods, "methods", "", "GET and/or HEAD (default both)")
			fs.IntVar(&r.TTLSeconds, "ttl", 0, "seconds to keep a response when the app sends no max-age")
			fs.BoolVar(&r.OverrideUpstream, "override", false, "use --ttl even when the app sends Cache-Control")
			fs.IntVar(&r.StaleWhileRevalidateSeconds, "stale-while-revalidate", 0, "seconds an expired entry may be served while it refreshes")
			fs.StringVar(&statuses, "status-codes", "", "comma separated statuses to cache (default 200)")
			fs.StringVar(&vary, "vary", "", "comma separated request headers that split the cache")
			fs.BoolVar(&r.CacheWithCookies, "with-cookies", false, "keep caching when the request has a Cookie")
		},
	})
	if !ok {
		return code
	}
	if r.Match.Path == "" {
		return usageError(stderr, prog, "--path is required")
	}
	codes, err := splitCSVInts(statuses)
	if err != nil {
		return usageError(stderr, prog, "--status-codes: "+err.Error())
	}
	r.StatusCodes, r.Vary, r.Match.Methods = codes, splitCSV(vary), splitCSVUpper(methods)
	cur, err := inv.client.GetDomainCache(bg(), inv.app, inv.domain)
	if err != nil {
		return cacheResult(stdout, stderr, inv, cur, err)
	}
	cur.Spec.Enabled = true
	cur.Spec.Rules = append(cur.Spec.Rules, r)
	res, err := inv.client.SetDomainCache(bg(), inv.app, inv.domain, cur.Spec)
	return cacheResult(stdout, stderr, inv, res, err)
}

func runCachePurge(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var urlPath, prefix string
	var all bool
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains cache purge", argsHelp: "<app> <domain>", summary: "Removes cached responses for this domain.",
		define: func(fs *flag.FlagSet) {
			fs.StringVar(&urlPath, "url", "", "one path and query, e.g. /page?x=1 (every Vary variant)")
			fs.StringVar(&prefix, "prefix", "", "every path under this prefix")
			fs.BoolVar(&all, "all", false, "everything cached for this domain")
		},
	})
	if !ok {
		return code
	}
	req := apiclient.PurgeCacheRequest{}
	switch {
	case all && urlPath == "" && prefix == "":
		req.Scope = purgeScopeAll
	case urlPath != "" && prefix == "" && !all:
		req.Scope, req.Value = purgeScopeURL, urlPath
	case prefix != "" && urlPath == "" && !all:
		req.Scope, req.Value = purgeScopePrefix, prefix
	default:
		return usageError(stderr, prog, "choose exactly one of --url, --prefix or --all")
	}
	res, err := inv.client.PurgeDomainCache(bg(), inv.app, inv.domain, req)
	if err != nil {
		return reportError(stdout, stderr, inv.jsonOut, fmt.Errorf("purge cache for domain %q: %w", inv.domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, inv.of, res, func() {
		_, _ = fmt.Fprintf(stdout, "%s: %d cached responses purged\n", res.Domain, res.Purged)
	})
}

func runCacheStats(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains cache stats", argsHelp: "<app> <domain>", summary: "Shows hit, miss and size counters since the control plane started."})
	if !ok {
		return code
	}
	res, err := inv.client.GetDomainCacheStats(bg(), inv.app, inv.domain)
	if err != nil {
		return reportError(stdout, stderr, inv.jsonOut, fmt.Errorf("cache stats for domain %q: %w", inv.domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, inv.of, res, func() {
		_, _ = fmt.Fprintf(stdout, "hits %d, stale %d, misses %d, bypasses %d, %d entries, %d bytes\n", res.Hits, res.Stale, res.Misses, res.Bypasses, res.Entries, res.Bytes)
	})
}

func runCacheClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains cache clear", argsHelp: "<app> <domain>", summary: "Turns caching off and removes every rule."})
	if !ok {
		return code
	}
	res, err := inv.client.ClearDomainCache(bg(), inv.app, inv.domain)
	return cacheResult(stdout, stderr, inv, res, err)
}
