package main

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func runDomainsRedirects(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return dispatchPolicy(prog, "redirects", args, stdout, stderr, lookupEnv, func(args []string) policyVerb {
		switch args[0] {
		case "show":
			return runRedirectsShow
		case "force-https":
			return runRedirectsForceHTTPS
		case "canonical":
			return runRedirectsCanonical
		case "alias":
			return runRedirectsAlias
		case "slash":
			return runRedirectsSlash
		case "lowercase":
			return runRedirectsLowercase
		case "clear":
			return runRedirectsClear
		}
		return nil
	}, domainsRedirectsUsage(prog))
}

func domainsRedirectsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains redirects show <app> <domain>
  %[1]s domains redirects force-https <app> <domain> on|off [--status 308|301]
  %[1]s domains redirects canonical <app> <domain> www-to-apex|apex-to-www|both [--status N] [--no-attach] [--replace]
  %[1]s domains redirects alias <app> <primary> --alias a.example.com [--alias b.example.com] [--status N]
  %[1]s domains redirects slash <app> <domain> add|remove|off
  %[1]s domains redirects lowercase <app> <domain> on|off
  %[1]s domains redirects clear <app> <domain>

Path and query are kept on every redirect. Behind a TLS terminating proxy,
force HTTPS trusts X-Forwarded-Proto only from APP_INGRESS_TRUSTED_PROXIES and
redirects to the public HTTPS port. Loops and conflicts are refused.
`, prog)
}

func printRedirectsHuman(out io.Writer, res apiclient.DomainRedirectsResource) {
	s := res.Settings
	_, _ = fmt.Fprintf(out, "domain:        %s\nforce https:   %t (status %d), handled by %s\n", res.Domain, s.ForceHTTPS, s.HTTPSStatus(), res.Effective.HandledBy)
	_, _ = fmt.Fprintf(out, "               %s\n", res.Effective.Explanation)
	_, _ = fmt.Fprintf(out, "trailing slash: %s\nlowercase host: %t\ncanonical:     %s", s.SlashMode(), s.LowercaseHost, res.Canonical.Preset)
	if res.Canonical.Counterpart != "" {
		_, _ = fmt.Fprintf(out, " (counterpart %s, attached %t)", res.Canonical.Counterpart, res.Canonical.CounterpartAttached)
	}
	_, _ = fmt.Fprintln(out)
	if res.Canonical.DNSHint != "" {
		_, _ = fmt.Fprintf(out, "dns:           %s\n", res.Canonical.DNSHint)
	}
	for _, d := range res.AppDomains {
		if d.Redirect != nil {
			_, _ = fmt.Fprintf(out, "  %s -> %s (%d)\n", d.Domain, d.Redirect.TargetURL, d.Redirect.StatusCode)
		}
	}
	for _, sample := range res.Samples {
		var parts []string
		for _, h := range sample.Hops {
			if h.Location != "" {
				parts = append(parts, fmt.Sprintf("%d -> %s", h.Status, h.Location))
			} else {
				parts = append(parts, fmt.Sprintf("%d %s", h.Status, h.Note))
			}
		}
		line := strings.Join(parts, ", ")
		if sample.Error != "" {
			line += " (" + sample.Error + ")"
		}
		_, _ = fmt.Fprintf(out, "  %s: %s\n", sample.URL, line)
	}
	for _, w := range res.Warnings {
		_, _ = fmt.Fprintf(out, "warning: %s\n", w)
	}
}

func redirectsResult(stdout, stderr io.Writer, inv policyInvocation, res apiclient.DomainRedirectsResource, err error) int {
	if err != nil {
		return reportError(stdout, stderr, inv.jsonOut, fmt.Errorf("redirects for domain %q: %w", inv.domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, inv.of, res, func() { printRedirectsHuman(stdout, res) })
}

func runRedirectsShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains redirects show", argsHelp: "<app> <domain>", summary: "Shows redirect settings and how sample URLs are handled."})
	if !ok {
		return code
	}
	res, err := inv.client.GetDomainRedirects(bg(), inv.app, inv.domain)
	return redirectsResult(stdout, stderr, inv, res, err)
}

// updateRedirectSettings reads the settings, applies change, and saves them.
func updateRedirectSettings(stdout, stderr io.Writer, inv policyInvocation, change func(*trafficpolicy.Redirects)) int {
	cur, err := inv.client.GetDomainRedirects(bg(), inv.app, inv.domain)
	if err != nil {
		return redirectsResult(stdout, stderr, inv, cur, err)
	}
	s := cur.Settings
	change(&s)
	res, err := inv.client.SetDomainRedirectSettings(bg(), inv.app, inv.domain, s)
	return redirectsResult(stdout, stderr, inv, res, err)
}

func onOff(v string) (bool, bool) {
	switch v {
	case "on", "true":
		return true, true
	case "off", "false":
		return false, true
	}
	return false, false
}

func runRedirectsForceHTTPS(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var status int
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains redirects force-https", argsHelp: "<app> <domain> on|off", extra: 1, summary: "Redirects plain HTTP to HTTPS for this domain.",
		define: func(fs *flag.FlagSet) { fs.IntVar(&status, "status", 0, "308 (default, keeps the method) or 301") },
	})
	if !ok {
		return code
	}
	on, valid := onOff(inv.extra[0])
	if !valid {
		return usageError(stderr, prog, "force-https takes on or off")
	}
	return updateRedirectSettings(stdout, stderr, inv, func(s *trafficpolicy.Redirects) {
		s.ForceHTTPS = on
		if status != 0 {
			s.ForceHTTPSStatus = status
		}
	})
}

func runRedirectsSlash(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains redirects slash", argsHelp: "<app> <domain> add|remove|off", extra: 1, summary: "Adds or removes a trailing slash with a 308 redirect."})
	if !ok {
		return code
	}
	mode := inv.extra[0]
	if mode != trafficpolicy.SlashAdd && mode != trafficpolicy.SlashRemove && mode != trafficpolicy.SlashOff {
		return usageError(stderr, prog, "slash takes add, remove or off")
	}
	return updateRedirectSettings(stdout, stderr, inv, func(s *trafficpolicy.Redirects) { s.TrailingSlash = mode })
}

func runRedirectsLowercase(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains redirects lowercase", argsHelp: "<app> <domain> on|off", extra: 1, summary: "Redirects a host with upper case letters to its lower case form."})
	if !ok {
		return code
	}
	on, valid := onOff(inv.extra[0])
	if !valid {
		return usageError(stderr, prog, "lowercase takes on or off")
	}
	return updateRedirectSettings(stdout, stderr, inv, func(s *trafficpolicy.Redirects) { s.LowercaseHost = on })
}

func runRedirectsCanonical(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var req apiclient.CanonicalRequest
	var noAttach bool
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains redirects canonical", argsHelp: "<app> <domain> www-to-apex|apex-to-www|both", extra: 1,
		summary: "Sets up www and apex: the non canonical host redirects to the canonical one, path and query kept. The counterpart host is attached to the app when missing.",
		define: func(fs *flag.FlagSet) {
			fs.IntVar(&req.StatusCode, "status", 0, "301 (default), 302, 307 or 308")
			fs.BoolVar(&noAttach, "no-attach", false, "fail instead of adding the counterpart host to the app")
			fs.BoolVar(&req.Replace, "replace", false, "overwrite an existing redirect on either host")
		},
	})
	if !ok {
		return code
	}
	req.Preset = inv.extra[0]
	if noAttach {
		off := false
		req.Attach = &off
	}
	res, err := inv.client.SetDomainCanonical(bg(), inv.app, inv.domain, req)
	return redirectsResult(stdout, stderr, inv, res, err)
}

func runRedirectsAlias(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var aliases stringListFlag
	var status int
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains redirects alias", argsHelp: "<app> <primary>",
		summary: "Makes <primary> the app's primary domain: every --alias redirects to it, other app domains stop redirecting to it. No --alias clears every alias.",
		define: func(fs *flag.FlagSet) {
			fs.Var(&aliases, "alias", "app domain that should redirect to the primary, repeatable")
			fs.IntVar(&status, "status", 0, "301 (default), 302, 307 or 308")
		},
	})
	if !ok {
		return code
	}
	res, err := inv.client.SetDomainAliases(bg(), inv.app, inv.domain, apiclient.AliasesRequest{Aliases: append([]string{}, aliases...), StatusCode: status})
	return redirectsResult(stdout, stderr, inv, res, err)
}

func runRedirectsClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains redirects clear", argsHelp: "<app> <domain>", summary: "Resets force HTTPS, trailing slash and lower case host to their defaults. Host redirects are left alone (see domains redirect clear)."})
	if !ok {
		return code
	}
	res, err := inv.client.ClearDomainRedirectSettings(bg(), inv.app, inv.domain)
	return redirectsResult(stdout, stderr, inv, res, err)
}

func runDomainsPorts(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return dispatchPolicy(prog, "ports", args, stdout, stderr, lookupEnv, func(args []string) policyVerb {
		switch args[0] {
		case "show":
			return runPortsShow
		case "restrict":
			return runPortsRestrict
		case "unrestrict":
			return runPortsUnrestrict
		}
		return nil
	}, domainsPortsUsage(prog))
}

func domainsPortsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains ports show <app> <domain>
  %[1]s domains ports restrict <app> <domain> <port> --source 203.0.113.0/24 [--source ...]
  %[1]s domains ports unrestrict <app> <domain> <port>

Lists the app's raw TCP/UDP streams (create them with "%[1]s apps streams").
restrict writes firewall rules labelled domain-ports:PORT/PROTO: one allow
per source, then a deny for everyone else. unrestrict removes exactly those.
`, prog)
}

func portsResult(stdout, stderr io.Writer, inv policyInvocation, res apiclient.DomainPortsResource, err error) int {
	if err != nil {
		return reportError(stdout, stderr, inv.jsonOut, fmt.Errorf("ports for app %q: %w", inv.app, err))
	}
	return writeScheduledTaskResult(stdout, stderr, inv.of, res, func() {
		if len(res.Streams) == 0 {
			_, _ = fmt.Fprintf(stdout, "app %s has no raw TCP/UDP ports; add one with apps streams\n", res.App)
		}
		for _, s := range res.Streams {
			who := "everyone"
			if !s.OpenToAll {
				who = strings.Join(s.AllowedSources, ", ")
			}
			_, _ = fmt.Fprintf(stdout, "%d/%s -> %s, open to %s\n", s.HostPort, s.Protocol, s.Target, who)
			for _, c := range s.Conflicts {
				_, _ = fmt.Fprintf(stdout, "  conflict: %s\n", c)
			}
		}
		if res.DetectionNote != "" {
			_, _ = fmt.Fprintf(stdout, "note: %s\n", res.DetectionNote)
		}
	})
}

func runPortsShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains ports show", argsHelp: "<app> <domain>", summary: "Shows the app's raw ports, who may reach them and conflicts."})
	if !ok {
		return code
	}
	res, err := inv.client.GetDomainPorts(bg(), inv.app, inv.domain)
	return portsResult(stdout, stderr, inv, res, err)
}

func runPortsRestrict(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var sources stringListFlag
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains ports restrict", argsHelp: "<app> <domain> <port>", extra: 1, summary: "Opens <port> only to the given sources.",
		define: func(fs *flag.FlagSet) {
			fs.Var(&sources, "source", "IP or CIDR allowed to connect, repeatable (required)")
		},
	})
	if !ok {
		return code
	}
	port, err := strconv.Atoi(inv.extra[0])
	if err != nil {
		return usageError(stderr, prog, "port must be a number")
	}
	if len(sources) == 0 {
		return usageError(stderr, prog, "at least one --source is required")
	}
	res, err := inv.client.RestrictDomainPort(bg(), inv.app, inv.domain, port, append([]string{}, sources...))
	return portsResult(stdout, stderr, inv, res, err)
}

func runPortsUnrestrict(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains ports unrestrict", argsHelp: "<app> <domain> <port>", extra: 1, summary: "Removes the restriction rules for <port>."})
	if !ok {
		return code
	}
	port, err := strconv.Atoi(inv.extra[0])
	if err != nil {
		return usageError(stderr, prog, "port must be a number")
	}
	res, err := inv.client.UnrestrictDomainPort(bg(), inv.app, inv.domain, port)
	return portsResult(stdout, stderr, inv, res, err)
}
