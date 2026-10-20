package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const securityEventsShown = 20

// runNodesSecurity implements "nodes security <id>": the node's trust
// summary plus its recent security events.
func runNodesSecurity(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes security", "print the trust summary and events as JSON to stdout and nothing else", stderr)
	rejectedOnly := fs.Bool("rejected", false, "show only rejected events")
	limit := fs.Int("limit", securityEventsShown, "max events to show")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesSecurityUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes security", "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	trust, err := client.GetNodeTrust(ctx, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get security summary for node %q: %w", id, err))
	}
	outcome := ""
	if *rejectedOnly {
		outcome = "rejected"
	}
	var events []apiclient.NodeSecurityEvent
	if !trust.IsLocal {
		if events, err = client.ListNodeSecurityEvents(ctx, id, outcome, *limit); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list security events for node %q: %w", id, err))
		}
	}
	result := struct {
		Trust  apiclient.NodeTrust           `json:"trust"`
		Events []apiclient.NodeSecurityEvent `json:"events"`
	}{trust, events}
	if err := renderResult(stdout, of.Format, of.Query, result, func() { printNodeSecurity(stdout, trust, events) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runNodesCert dispatches "nodes cert <subcommand>".
func runNodesCert(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, nodesCertUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, nodesCertUsage(prog))
		return exitOK
	case "renew":
		return runNodesCertRenew(prog, args[1:], stdout, stderr, lookupEnv)
	case "drill":
		return runNodesCertDrill(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown nodes cert subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, nodesCertUsage(prog))
		return exitUsage
	}
}

func runNodesCertRenew(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes cert renew", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesCertRenewUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes cert renew", "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.RenewNodeCert(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("request certificate renewal for node %q: %w", id, err))
	}
	result := map[string]any{"node_id": id, "requested": true}
	if err := renderResult(stdout, of.Format, of.Query, result, func() {
		_, _ = fmt.Fprintf(stdout, "node %s: renewal requested; check progress with \"%s nodes security %s\"\n", id, prog, id)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(time.RFC3339)
}

func groupFingerprint(fp string) string {
	if fp == "" {
		return "-"
	}
	var b strings.Builder
	for i := 0; i < len(fp); i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(strings.ToUpper(fp[i:min(i+2, len(fp))]))
	}
	return b.String()
}

func printNodeSecurity(out io.Writer, t apiclient.NodeTrust, events []apiclient.NodeSecurityEvent) {
	if t.IsLocal {
		_, _ = fmt.Fprintln(out, "This node runs inside the control plane process. There is no network link to secure, so no agent certificate is used.")
		return
	}
	_, _ = fmt.Fprintf(out, "status:               %s\n", trustStatusLine(t.Status))
	if v := t.Validation; v != nil {
		_, _ = fmt.Fprintf(out, "last %dh:             %d accepted, %d rejected\n", v.WindowHours, v.Accepted, v.Rejected)
	}
	if ca := t.CA; ca != nil {
		_, _ = fmt.Fprintf(out, "CA fingerprint:       %s\n", groupFingerprint(ca.Fingerprint))
		_, _ = fmt.Fprintf(out, "CA pin at enrollment: %s\n", orDash(ca.PinState))
	}
	if c := t.Cert; c != nil {
		_, _ = fmt.Fprintf(out, "cert fingerprint:     %s\n", groupFingerprint(c.Fingerprint))
		_, _ = fmt.Fprintf(out, "cert serial:          %s\n", orDash(c.Serial))
		_, _ = fmt.Fprintf(out, "cert issued:          %s\n", fmtTime(c.IssuedAt))
		_, _ = fmt.Fprintf(out, "cert expires:         %s\n", fmtTime(c.NotAfter))
		_, _ = fmt.Fprintf(out, "private key made by:  %s\n", keyOriginLabel(c.KeyOrigin))
	}
	if r := t.Renewal; r != nil {
		next := fmtTime(r.NextRenewAt)
		if r.NextRenewEstimated {
			next += " (estimated)"
		}
		_, _ = fmt.Fprintf(out, "next renewal:         %s\n", next)
		_, _ = fmt.Fprintf(out, "last renewed:         %s (%d renewals)\n", fmtTime(r.LastRenewedAt), r.RenewalCount)
		if r.LastError != "" {
			_, _ = fmt.Fprintf(out, "last renewal failure: %s at %s (%d in a row), next retry %s\n", r.LastError, fmtTime(r.LastFailedAt), r.ConsecutiveFailures, fmtTime(r.NextRetryAt))
		}
	}
	_, _ = fmt.Fprintln(out)
	if len(events) == 0 {
		_, _ = fmt.Fprintln(out, "No security events recorded.")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TIME\tKIND\tOUTCOME\tREASON\tFROM\tCOUNT")
	for _, e := range events {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\n", e.LastAt.Format("2006-01-02 15:04:05"), e.Kind, e.Outcome, orDash(e.Reason), orDash(e.SourceIP), e.Count)
	}
	_ = tw.Flush()
}

func trustStatusLine(s apiclient.NodeTrustStatus) string {
	switch s.State {
	case "verified":
		return "connection verified: mutual TLS, certificate matches this node"
	case "rejected":
		return "connections rejected recently (" + orDash(s.Reason) + ")"
	case "revoked":
		return "certificate revoked: every connection is refused until the node is re-enrolled"
	case "expired":
		return "certificate expired: re-enroll the node"
	case "idle":
		return "not connected now; its last connection was verified"
	}
	return "no connection recorded yet"
}

func nodesSecurityUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes security <id> [--rejected] [--limit N] [flags]

Shows how the control plane verifies node <id>: the CA and agent certificate
fingerprints, the renewal schedule and last failure, the last 24 hours of
connection checks, and the recent security events (enrollment, CA pin,
accepted and rejected connections, certificates issued, revocations).

Flags:
  --rejected              show only rejected events
  --limit int             max events to show (default %[5]d)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the summary and events as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, securityEventsShown)
}

func nodesCertUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes cert renew <id> [flags]   ask a connected node to renew its agent certificate now
  %[1]s nodes cert drill <id> [flags]   rotate the certificate end to end and report each step

Run "%[1]s nodes cert <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func nodesCertRenewUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes cert renew <id> [flags]

Asks node <id>'s agent, over its live session, to renew its certificate
right now instead of at its scheduled time. The agent verifies the new
certificate before it replaces the old one. Fails when the node is not
connected (it renews by itself when it reconnects) or its certificate is
revoked (re-enroll it instead).

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
