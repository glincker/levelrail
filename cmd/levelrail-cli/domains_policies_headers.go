package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func runDomainsHeaders(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return dispatchPolicy(prog, "headers", args, stdout, stderr, lookupEnv, func(args []string) policyVerb {
		switch args[0] {
		case "show":
			return runHeadersShow
		case "set":
			return runHeadersSet
		case "add":
			return runHeadersAdd
		case "preset":
			return runHeadersPreset
		case "clear":
			return runHeadersClear
		}
		return nil
	}, domainsHeadersUsage(prog))
}

func domainsHeadersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains headers show <app> <domain>
  %[1]s domains headers set <app> <domain> --file headers.json
  %[1]s domains headers add <app> <domain> --side request|response --op set|add|remove --name NAME [--value V]
  %[1]s domains headers preset <app> <domain> security|cors|hide-server [flags]
  %[1]s domains headers clear <app> <domain>

Rules apply in order. Response rules apply after the app answered, so they
win over the app's own headers. Host, Content-Length, Transfer-Encoding,
Connection, Upgrade and X-Forwarded-* are managed by the ingress and refused.
Changes take effect on the next ingress reconcile pass.
`, prog)
}

func printHeadersHuman(out io.Writer, res apiclient.HeadersPolicyResource) {
	if !res.Configured {
		_, _ = fmt.Fprintf(out, "%s: no header rules (default)\n", res.Domain)
		return
	}
	_, _ = fmt.Fprintf(out, "%s: %d header rules\n", res.Domain, len(res.Spec.Rules))
	for i, r := range res.Spec.Rules {
		_, _ = fmt.Fprintf(out, "  %d. %s %s %s %s\n", i+1, r.Side, r.Op, r.Name, r.Value)
	}
	if s := res.Spec.Security; s != nil {
		_, _ = fmt.Fprintf(out, "  security preset: hsts=%t frame=%s referrer=%s\n", s.HSTS, s.FrameOptions, s.ReferrerPolicy)
	}
	if c := res.Spec.CORS; c != nil {
		_, _ = fmt.Fprintf(out, "  cors: origins=%v credentials=%t preflight=%t\n", c.Origins, c.Credentials, c.Preflight)
	}
	if res.Spec.HideServer {
		_, _ = fmt.Fprintln(out, "  Server and X-Powered-By removed")
	}
}

func headersResult(stdout, stderr io.Writer, inv policyInvocation, res apiclient.HeadersPolicyResource, err error) int {
	if err != nil {
		return reportError(stdout, stderr, inv.jsonOut, fmt.Errorf("headers for domain %q: %w", inv.domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, inv.of, res, func() { printHeadersHuman(stdout, res) })
}

func runHeadersShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains headers show", argsHelp: "<app> <domain>", summary: "Shows a domain's header rules."})
	if !ok {
		return code
	}
	res, err := inv.client.GetDomainHeaders(bg(), inv.app, inv.domain)
	return headersResult(stdout, stderr, inv, res, err)
}

func runHeadersSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var file string
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains headers set", argsHelp: "<app> <domain>", summary: "Replaces the whole header policy with a JSON file.",
		define: func(fs *flag.FlagSet) {
			fs.StringVar(&file, "file", "", "JSON file with rules, security, cors, hide_server (required)")
		},
	})
	if !ok {
		return code
	}
	if file == "" {
		return usageError(stderr, prog, "--file is required")
	}
	var spec trafficpolicy.Headers
	if err := readStrictJSON(file, &spec); err != nil {
		return reportError(stdout, stderr, inv.jsonOut, err)
	}
	res, err := inv.client.SetDomainHeaders(bg(), inv.app, inv.domain, spec)
	return headersResult(stdout, stderr, inv, res, err)
}

func runHeadersAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var rule trafficpolicy.HeaderRule
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains headers add", argsHelp: "<app> <domain>", summary: "Appends one header rule.",
		define: func(fs *flag.FlagSet) {
			fs.StringVar(&rule.Side, "side", trafficpolicy.SideResponse, "request or response")
			fs.StringVar(&rule.Op, "op", trafficpolicy.OpSet, "set, add or remove")
			fs.StringVar(&rule.Name, "name", "", "header name (required)")
			fs.StringVar(&rule.Value, "value", "", "header value (empty for remove)")
		},
	})
	if !ok {
		return code
	}
	if rule.Name == "" {
		return usageError(stderr, prog, "--name is required")
	}
	cur, err := inv.client.GetDomainHeaders(bg(), inv.app, inv.domain)
	if err != nil {
		return headersResult(stdout, stderr, inv, cur, err)
	}
	cur.Spec.Rules = append(cur.Spec.Rules, rule)
	res, err := inv.client.SetDomainHeaders(bg(), inv.app, inv.domain, cur.Spec)
	return headersResult(stdout, stderr, inv, res, err)
}

func runHeadersPreset(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var origins stringListFlag
	var methods, headers string
	var credentials, noPreflight, noHSTS bool
	var maxAge int
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains headers preset", argsHelp: "<app> <domain> security|cors|hide-server", extra: 1,
		summary: "Applies a one click preset on top of the existing rules. security adds HSTS only when the domain has a publicly trusted certificate.",
		define: func(fs *flag.FlagSet) {
			fs.Var(&origins, "origin", "cors: allowed origin, repeatable, or * for any")
			fs.StringVar(&methods, "methods", "", "cors: comma separated methods (default GET, HEAD, POST)")
			fs.StringVar(&headers, "allow-headers", "", "cors: comma separated request headers (default: echo the request)")
			fs.BoolVar(&credentials, "credentials", false, "cors: allow credentials (not with *)")
			fs.IntVar(&maxAge, "max-age", 600, "cors: preflight cache seconds")
			fs.BoolVar(&noPreflight, "no-preflight", false, "cors: pass OPTIONS preflights to the app")
			fs.BoolVar(&noHSTS, "no-hsts", false, "security: leave out Strict-Transport-Security")
		},
	})
	if !ok {
		return code
	}
	cur, err := inv.client.GetDomainHeaders(bg(), inv.app, inv.domain)
	if err != nil {
		return headersResult(stdout, stderr, inv, cur, err)
	}
	switch inv.extra[0] {
	case "security":
		all, err := inv.client.GetDomainPolicies(bg(), inv.app, inv.domain)
		if err != nil {
			return headersResult(stdout, stderr, inv, cur, err)
		}
		preset := all.Security
		if noHSTS {
			preset.HSTS = false
		}
		cur.Spec.Security = &preset
	case "cors":
		if len(origins) == 0 {
			return usageError(stderr, prog, "cors needs at least one --origin")
		}
		cur.Spec.CORS = &trafficpolicy.CORSPreset{
			Origins: origins, Methods: splitCSVUpper(methods), Headers: splitCSV(headers),
			Credentials: credentials, MaxAgeSeconds: maxAge, Preflight: !noPreflight,
		}
	case "hide-server":
		cur.Spec.HideServer = true
	default:
		return usageError(stderr, prog, "preset must be security, cors or hide-server")
	}
	res, err := inv.client.SetDomainHeaders(bg(), inv.app, inv.domain, cur.Spec)
	return headersResult(stdout, stderr, inv, res, err)
}

func runHeadersClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains headers clear", argsHelp: "<app> <domain>", summary: "Removes every header rule and preset."})
	if !ok {
		return code
	}
	res, err := inv.client.ClearDomainHeaders(bg(), inv.app, inv.domain)
	return headersResult(stdout, stderr, inv, res, err)
}
