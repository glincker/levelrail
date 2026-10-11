package main

import (
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func runDomainsForwarders(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return dispatchPolicy(prog, "forwarders", args, stdout, stderr, lookupEnv, func(args []string) policyVerb {
		switch args[0] {
		case "show":
			return runForwardersShow
		case "set":
			return runForwardersSet
		case "add":
			return runForwardersAdd
		case "remove":
			return runForwardersRemove
		case "clear":
			return runForwardersClear
		}
		return nil
	}, domainsForwardersUsage(prog))
}

func domainsForwardersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains forwarders show <app> <domain>
  %[1]s domains forwarders add <app> <domain> --path /api (--app NAME | --url URL | --redirect URL) [flags]
  %[1]s domains forwarders remove <app> <domain> <number>
  %[1]s domains forwarders set <app> <domain> --file forwarders.json
  %[1]s domains forwarders clear <app> <domain>

Rules are checked in order and the first match wins; a request no rule
matches goes to the domain's own app. External URLs must resolve to a public
address unless APP_FORWARDER_ALLOW_PRIVATE_NETWORKS or
APP_FORWARDER_ALLOW_LOOPBACK allow otherwise.
`, prog)
}

func printForwardersHuman(out io.Writer, res apiclient.ForwardersPolicyResource) {
	if len(res.Spec.Rules) == 0 {
		_, _ = fmt.Fprintf(out, "%s: no forwarders, every request goes to the app\n", res.Domain)
		return
	}
	_, _ = fmt.Fprintf(out, "%s: %d forwarders (first match wins)\n", res.Domain, len(res.Spec.Rules))
	for i, f := range res.Spec.Rules {
		target := f.App
		switch f.Action {
		case trafficpolicy.ActionURL, trafficpolicy.ActionRedirect:
			target = f.URL
		}
		_, _ = fmt.Fprintf(out, "  %d. %s -> %s %s\n", i+1, f.Match.Describe(), f.Action, target)
	}
}

func forwardersResult(stdout, stderr io.Writer, inv policyInvocation, res apiclient.ForwardersPolicyResource, err error) int {
	if err != nil {
		return reportError(stdout, stderr, inv.jsonOut, fmt.Errorf("forwarders for domain %q: %w", inv.domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, inv.of, res, func() { printForwardersHuman(stdout, res) })
}

func runForwardersShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains forwarders show", argsHelp: "<app> <domain>", summary: "Shows a domain's path forwarders in order."})
	if !ok {
		return code
	}
	res, err := inv.client.GetDomainForwarders(bg(), inv.app, inv.domain)
	return forwardersResult(stdout, stderr, inv, res, err)
}

func runForwardersSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var file string
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains forwarders set", argsHelp: "<app> <domain>", summary: "Replaces every forwarder with a JSON file ({\"rules\": [...]}).",
		define: func(fs *flag.FlagSet) { fs.StringVar(&file, "file", "", "JSON file (required)") },
	})
	if !ok {
		return code
	}
	if file == "" {
		return usageError(stderr, prog, "--file is required")
	}
	var spec trafficpolicy.Forwarders
	if err := readStrictJSON(file, &spec); err != nil {
		return reportError(stdout, stderr, inv.jsonOut, err)
	}
	res, err := inv.client.SetDomainForwarders(bg(), inv.app, inv.domain, spec)
	return forwardersResult(stdout, stderr, inv, res, err)
}

type forwarderFlags struct {
	f                       trafficpolicy.Forwarder
	methods, app, url, redi string
	noWebSocket             bool
	position                int
}

func (ff *forwarderFlags) define(fs *flag.FlagSet) {
	fs.StringVar(&ff.f.Name, "name", "", "optional label")
	fs.StringVar(&ff.f.Match.Path, "path", "", "path to match (required)")
	fs.StringVar(&ff.f.Match.Kind, "kind", trafficpolicy.MatchPrefix, "prefix, exact or regex")
	fs.StringVar(&ff.methods, "methods", "", "comma separated methods; empty matches every method")
	fs.StringVar(&ff.app, "app", "", "forward to this Levelrail app")
	fs.StringVar(&ff.url, "url", "", "forward to this external http(s) URL")
	fs.StringVar(&ff.redi, "redirect", "", "redirect to this URL instead of forwarding")
	fs.IntVar(&ff.f.RedirectStatus, "redirect-status", 0, "301, 302 (default), 307 or 308")
	fs.BoolVar(&ff.f.StripPrefix, "strip-prefix", false, "remove the matched prefix before forwarding")
	fs.StringVar(&ff.f.RewritePrefix, "rewrite-prefix", "", "prepend this path before forwarding")
	fs.StringVar(&ff.f.Host, "host", "", "Host header: preserve (apps default), upstream (URLs default) or custom")
	fs.StringVar(&ff.f.HostValue, "host-value", "", "Host header value for --host custom")
	fs.BoolVar(&ff.noWebSocket, "no-websocket", false, "refuse WebSocket upgrades on this path")
	fs.IntVar(&ff.f.TimeoutSeconds, "timeout", 0, "response header timeout in seconds (0 uses APP_FORWARDER_TIMEOUT)")
	fs.IntVar(&ff.position, "position", 0, "insert at this 1-based position (default: last)")
}

// build turns the flags into a rule, refusing anything but exactly one target.
func (ff *forwarderFlags) build() (trafficpolicy.Forwarder, error) {
	f := ff.f
	f.Match.Methods = splitCSVUpper(ff.methods)
	set := 0
	for _, v := range []string{ff.app, ff.url, ff.redi} {
		if v != "" {
			set++
		}
	}
	if set != 1 {
		return f, fmt.Errorf("choose exactly one of --app, --url or --redirect")
	}
	switch {
	case ff.app != "":
		f.Action, f.App = trafficpolicy.ActionApp, ff.app
	case ff.url != "":
		f.Action, f.URL = trafficpolicy.ActionURL, ff.url
	default:
		f.Action, f.URL = trafficpolicy.ActionRedirect, ff.redi
	}
	if ff.noWebSocket {
		off := false
		f.WebSocket = &off
	}
	if f.Match.Path == "" {
		return f, fmt.Errorf("--path is required")
	}
	return f, nil
}

func runForwardersAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var ff forwarderFlags
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains forwarders add", argsHelp: "<app> <domain>", summary: "Adds one path forwarder.", define: ff.define,
	})
	if !ok {
		return code
	}
	rule, err := ff.build()
	if err != nil {
		return usageError(stderr, prog, err.Error())
	}
	cur, err := inv.client.GetDomainForwarders(bg(), inv.app, inv.domain)
	if err != nil {
		return forwardersResult(stdout, stderr, inv, cur, err)
	}
	rules := cur.Spec.Rules
	pos := len(rules)
	if ff.position > 0 && ff.position-1 < pos {
		pos = ff.position - 1
	}
	rules = append(rules[:pos], append([]trafficpolicy.Forwarder{rule}, rules[pos:]...)...)
	res, err := inv.client.SetDomainForwarders(bg(), inv.app, inv.domain, trafficpolicy.Forwarders{Rules: rules})
	return forwardersResult(stdout, stderr, inv, res, err)
}

func runForwardersRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains forwarders remove", argsHelp: "<app> <domain> <number>", extra: 1, summary: "Removes the forwarder at a 1-based position (see show)."})
	if !ok {
		return code
	}
	n, err := strconv.Atoi(inv.extra[0])
	if err != nil || n < 1 {
		return usageError(stderr, prog, "number must be a positive position from show")
	}
	cur, err := inv.client.GetDomainForwarders(bg(), inv.app, inv.domain)
	if err != nil {
		return forwardersResult(stdout, stderr, inv, cur, err)
	}
	if n > len(cur.Spec.Rules) {
		return usageError(stderr, prog, fmt.Sprintf("there are only %d forwarders", len(cur.Spec.Rules)))
	}
	rules := append(cur.Spec.Rules[:n-1], cur.Spec.Rules[n:]...)
	res, err := inv.client.SetDomainForwarders(bg(), inv.app, inv.domain, trafficpolicy.Forwarders{Rules: rules})
	return forwardersResult(stdout, stderr, inv, res, err)
}

func runForwardersClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains forwarders clear", argsHelp: "<app> <domain>", summary: "Removes every forwarder; every request goes to the app again."})
	if !ok {
		return code
	}
	res, err := inv.client.ClearDomainForwarders(bg(), inv.app, inv.domain)
	return forwardersResult(stdout, stderr, inv, res, err)
}
