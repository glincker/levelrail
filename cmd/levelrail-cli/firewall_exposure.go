package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const exposureFlagsHelp = `  --token string          API token (default: %[1]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`

// runFirewallExposure implements "firewall exposure": the read-only audit of
// published container ports behind GET /api/v1/firewall/exposure.
func runFirewallExposure(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "firewall exposure", "print the exposure report as JSON to stdout and nothing else", stderr)
	var node string
	var probe bool
	fs.StringVar(&node, "node", "", "audit only this node (ID or name; default: every node)")
	fs.BoolVar(&probe, "probe", false, "also dial each internet-facing port on the node's public address from this side")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s firewall exposure [--node N] [--probe] [flags]\n\nLists published container ports and how reachable each is. Read-only.\n\nFlags:\n  --node string           audit only this node\n  --probe                 also dial each internet-facing port from this side; a failed dial is reported as \"could not confirm\", never as closed\n", prog)
		_, _ = fmt.Fprintf(stderr, exposureFlagsHelp, envAPIToken, envAPIURL, defaultAPIURL)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	rep, err := client.GetExposure(context.Background(), node, probe)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("firewall exposure: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, rep, func() { printExposure(stdout, rep) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printExposure(out io.Writer, rep apiclient.ExposureReport) {
	for _, n := range rep.Nodes {
		_, _ = fmt.Fprintf(out, "node %s\n", n.NodeName)
		if n.Status != "ok" {
			_, _ = fmt.Fprintf(out, "  could not audit: %s\n", n.Error)
			continue
		}
		if !n.RulesReadable {
			_, _ = fmt.Fprintf(out, "  firewall rules not readable: %s\n", n.RulesNote)
		}
		if len(n.Findings) == 0 {
			_, _ = fmt.Fprintln(out, "  Nothing of yours is published on this node.")
			continue
		}
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  SEVERITY\tPORT\tCONTAINER\tOWNER\tBIND\tSTATE\tOUTSIDE CHECK")
		for _, f := range n.Findings {
			owner := f.Owner.Kind
			if f.Owner.Name != "" {
				owner += ":" + f.Owner.Name
			}
			_, _ = fmt.Fprintf(tw, "  %s\t%d/%s\t%s\t%s\t%s\t%s\t%s\n", f.Severity, f.HostPort, f.Protocol, f.Container, owner, strings.Join(f.Binds, ","), f.Class, f.Outside.Status)
		}
		_ = tw.Flush()
		for _, f := range n.Findings {
			if f.Class == "exposed" || f.Class == "unknown" {
				_, _ = fmt.Fprintf(out, "\n  %d/%s: %s\n", f.HostPort, f.Protocol, f.Explanation)
				if f.Recommendation != "" {
					_, _ = fmt.Fprintf(out, "  Do: %s\n", f.Recommendation)
				}
			}
		}
	}
}

// runFirewallRestrict implements "firewall restrict": a dry-run preview or an
// applied DOCKER-USER allow-list for one published port.
func runFirewallRestrict(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "firewall restrict", "print the plan as JSON to stdout and nothing else", stderr)
	var port int
	var protocol, allow, node string
	var dryRun, apply, local bool
	fs.IntVar(&port, "port", 0, "published host port to restrict (required)")
	fs.StringVar(&protocol, "protocol", "tcp", "\"tcp\" or \"udp\"")
	fs.StringVar(&allow, "allow", "", "comma separated source IPs or CIDRs that may still connect")
	fs.BoolVar(&local, "local-containers", false, "also allow containers on this host (Docker's default 172.16.0.0/12 pool)")
	fs.StringVar(&node, "node", "", "target node (only the control plane's own host is supported)")
	fs.BoolVar(&dryRun, "dry-run", false, "show the exact rules and what they drop, change nothing")
	fs.BoolVar(&apply, "apply", false, "apply the rules; this confirms the traffic described by --dry-run will be dropped")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s firewall restrict --port N --allow CIDR[,CIDR] [--local-containers] (--dry-run | --apply) [flags]\n\nRestricts a published container port with comment-tagged DOCKER-USER rules.\nRefused for ports the control plane, its agents, ingress or SSH need.\nReverse it with \"%s firewall unrestrict --port N\".\n\nFlags:\n  --port int  --protocol string  --allow string  --local-containers  --node string  --dry-run  --apply\n", prog, prog)
		_, _ = fmt.Fprintf(stderr, exposureFlagsHelp, envAPIToken, envAPIURL, defaultAPIURL)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if port < 1 || port > 65535 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--port is required and must be between 1 and 65535"))
	}
	if dryRun == apply {
		return reportError(stdout, stderr, jsonOut, newValidationError("pass exactly one of --dry-run or --apply"))
	}
	req := apiclient.ExposureRestrictRequest{Node: node, Port: port, Protocol: protocol, LocalContainers: local}
	for _, a := range strings.Split(allow, ",") {
		if a = strings.TrimSpace(a); a != "" {
			req.Allow = append(req.Allow, a)
		}
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var plan apiclient.ExposurePlan
	var err error
	if apply {
		req.Confirm = true
		plan, err = client.ApplyExposureRestriction(context.Background(), req)
	} else {
		plan, err = client.PreviewExposureRestriction(context.Background(), req)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("firewall restrict %d/%s: %w", port, protocol, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, plan, func() { printExposurePlan(stdout, plan, apply) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printExposurePlan(out io.Writer, p apiclient.ExposurePlan, applied bool) {
	if applied {
		_, _ = fmt.Fprintf(out, "applied restriction on %d/%s\n", p.Port, p.Protocol)
	} else {
		_, _ = fmt.Fprintln(out, "dry run, nothing changed. Would run, in order:")
	}
	for _, c := range p.Commands {
		_, _ = fmt.Fprintf(out, "  %s\n", c)
	}
	_, _ = fmt.Fprintf(out, "\nEffect: %s\n", p.Drops)
	for _, w := range p.Warnings {
		_, _ = fmt.Fprintf(out, "Warning: %s\n", w)
	}
	_, _ = fmt.Fprintf(out, "Persistence: %s\n", p.Persistence)
}

// runFirewallUnrestrict implements "firewall unrestrict": removes only the
// rules this platform tagged for the port.
func runFirewallUnrestrict(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "firewall unrestrict", "print the result as JSON to stdout and nothing else", stderr)
	var port int
	var protocol string
	fs.IntVar(&port, "port", 0, "published host port to unrestrict (required)")
	fs.StringVar(&protocol, "protocol", "tcp", "\"tcp\" or \"udp\"")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s firewall unrestrict --port N [--protocol tcp|udp] [flags]\n\nRemoves the rules the control plane added for this port. Rules you created yourself are never touched.\n\nFlags:\n  --port int  --protocol string\n", prog)
		_, _ = fmt.Fprintf(stderr, exposureFlagsHelp, envAPIToken, envAPIURL, defaultAPIURL)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if port < 1 || port > 65535 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--port is required and must be between 1 and 65535"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.RemoveExposureRestriction(context.Background(), protocol, port); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("firewall unrestrict %d/%s: %w", port, protocol, err))
	}
	res := map[string]any{"port": port, "protocol": protocol, "removed": true}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		_, _ = fmt.Fprintf(stdout, "restriction on %d/%s removed\n", port, protocol)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
