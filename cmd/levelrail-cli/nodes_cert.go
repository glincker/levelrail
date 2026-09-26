package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runNodesReenrollToken implements "nodes reenroll-token <id>": POST
// /api/v1/nodes/{id}/reenroll-token. The token is printed once.
func runNodesReenrollToken(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes reenroll-token", "print the new token as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesReenrollTokenUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes reenroll-token", "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreateNodeReenrollToken(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create re-enroll token for node %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "re-enroll token for node %s (shown once, not recoverable again): %s\n", created.NodeID, created.Token)
		_, _ = fmt.Fprintf(stdout, "expires at: %s\n", created.ExpiresAt.Format(time.RFC3339))
		_, _ = fmt.Fprintf(stdout, "run on the node:\n  %s\n", reenrollCommand(created))
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// reenrollCommand is the command an operator runs on the node. The
// control plane address is left as a placeholder: only the operator knows
// how that node reaches it.
func reenrollCommand(t apiclient.NodeReenrollTokenResponse) string {
	cmd := "APP_CONTROL_PLANE_ADDR=<control-plane-host>:9443 APP_REENROLL_TOKEN=" + t.Token
	if t.CAFingerprint != "" {
		cmd += " APP_CA_FINGERPRINT=" + t.CAFingerprint
	}
	return cmd + " ./" + t.AgentBinary + " reenroll"
}

func nodesReenrollTokenUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes reenroll-token <id> [flags]

Mints a one-time token, valid for 15 minutes, that lets node <id> obtain a
new agent certificate while keeping its identity, placements and history.
Use it for a node that was offline past its certificate's expiry, or whose
certificate was revoked. The token is printed once, with the command to run
on the node.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the new token as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runNodesRevokeCert implements "nodes revoke-cert <id>": POST
// /api/v1/nodes/{id}/revoke-cert.
func runNodesRevokeCert(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes revoke-cert", "print the node as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesRevokeCertUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes revoke-cert", "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	node, err := client.RevokeNodeCert(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke certificate for node %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, node, func() {
		_, _ = fmt.Fprintf(stdout, "node %s: agent certificate revoked and session closed; run \"%s nodes reenroll-token %s\" to bring it back\n", id, prog, id)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func nodesRevokeCertUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes revoke-cert <id> [flags]

Revokes node <id>'s agent certificate: the control plane refuses it from
now on and closes the node's live session. Workloads already running on the
node keep running, but the control plane can no longer reach it. Only a
re-enroll token ("nodes reenroll-token") brings the node back.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the node as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// nodeCertColumn is the CERT column in "nodes list": the state plus days
// left, e.g. "ok 63d" or "expired".
func nodeCertColumn(c *apiclient.NodeCertResource) string {
	if c == nil {
		return "-"
	}
	if c.DaysRemaining == nil || c.State == "expired" || c.State == "revoked" {
		return c.State
	}
	return fmt.Sprintf("%s %dd", c.State, *c.DaysRemaining)
}

// nodeAgentColumn is the AGENT column: version, flagged when outdated.
func nodeAgentColumn(a *apiclient.NodeAgentResource) string {
	if a == nil || a.Version == "" {
		if a != nil && a.Outdated {
			return "unknown (outdated)"
		}
		return "-"
	}
	if a.Outdated {
		return a.Version + " (outdated)"
	}
	return a.Version
}

func printNodeCertHuman(out io.Writer, n nodeResource) {
	if c := n.Cert; c != nil {
		_, _ = fmt.Fprintf(out, "agent cert:              %s\n", nodeCertColumn(c))
		if c.NotAfter != nil {
			_, _ = fmt.Fprintf(out, "agent cert expires:      %s\n", c.NotAfter.Format(time.RFC3339))
		}
		if c.RenewedAt != nil {
			_, _ = fmt.Fprintf(out, "agent cert renewed:      %s (generation %d)\n", c.RenewedAt.Format(time.RFC3339), c.Generation)
		}
		if c.KeyOrigin != "" {
			_, _ = fmt.Fprintf(out, "agent key generated by:  %s\n", c.KeyOrigin)
		}
		if c.RevokedAt != nil {
			_, _ = fmt.Fprintf(out, "agent cert revoked:      %s\n", c.RevokedAt.Format(time.RFC3339))
		}
	}
	if a := n.Agent; a != nil {
		_, _ = fmt.Fprintf(out, "agent version:           %s\n", nodeAgentColumn(a))
		if a.OS != "" || a.Arch != "" {
			_, _ = fmt.Fprintf(out, "agent platform:          %s/%s\n", a.OS, a.Arch)
		}
		if a.Commit != "" {
			_, _ = fmt.Fprintf(out, "agent commit:            %s\n", a.Commit)
		}
	}
}
