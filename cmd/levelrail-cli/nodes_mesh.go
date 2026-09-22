package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// runNodesMesh implements "nodes mesh": GET /api/v1/mesh, this control
// plane's own live WireGuard mesh state and every peer it currently
// knows about. Unlike cordon/drain/workloads, this is not scoped to one
// node ID: internal/api/mesh.go's own doc comment explains why there is
// only ever one live mesh view to ask for today (the control plane's own
// node), so this subcommand takes no positional argument, matching
// runNodesJoinToken's own no-argument shape rather than the id-taking
// commands around it.
func runNodesMesh(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes mesh", "print mesh status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesMeshUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "%s: nodes mesh takes no arguments\n\n", prog)
		_, _ = fmt.Fprint(stderr, nodesMeshUsage(prog))
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	status, err := client.GetMeshStatus(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get mesh status: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, status, func() {
		_, _ = fmt.Fprintln(stdout, formatMeshStatusHuman(status))
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// formatMeshStatusHuman renders a meshStatusResource as a short table: one
// header line for this node's own identity, one line per peer. A peer
// with Live: false (known to the store, but this node's device has no
// live entry for it yet) is called out explicitly rather than shown with
// blank handshake data that would look like a stalled connection instead
// of a not-yet-established one.
func formatMeshStatusHuman(s meshStatusResource) string {
	var b strings.Builder
	fmt.Fprintf(&b, "backend: %s   interface: %s   address: %s   public key: %s\n",
		orDash(s.Backend), orDash(s.Interface), orDash(s.MeshAddress), orDash(s.PublicKey))
	if s.Rotation != nil {
		state := "confirming"
		if s.Rotation.Confirmed {
			state = "confirmed"
		}
		fmt.Fprintf(&b, "last rotation: %s (started %s)\n", state, s.Rotation.StartedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	if len(s.Peers) == 0 {
		b.WriteString("no peers\n")
		return strings.TrimRight(b.String(), "\n")
	}
	fmt.Fprintln(&b, "\nNODE\tADDRESS\tHANDSHAKE\tHEALTHY\tLIVE")
	for _, p := range s.Peers {
		name := p.NodeID
		if p.Name != "" {
			name = p.Name
		}
		handshake := "never"
		if p.LastHandshake != nil {
			handshake = p.LastHandshake.Format("2006-01-02T15:04:05Z07:00")
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%t\t%t\n", name, orDash(p.MeshAddress), handshake, p.Healthy, p.Live)
	}
	return strings.TrimRight(b.String(), "\n")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func nodesMeshUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes mesh [flags]

Shows this control plane's own live WireGuard mesh state: backend,
interface, mesh address, public key, and every peer it currently knows
about (name, mesh address, last handshake, health). A peer with live=false
is known from the node registry but has no live device entry yet (the
mesh reconciler has not reached it this pass, or peering hasn't converged
yet).

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print mesh status as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runNodesRotateKey implements "nodes rotate-key <id>": POST
// /api/v1/nodes/{id}/mesh/rotate-key, mirroring runNodesCordon's own
// single-id, no-body POST shape.
func runNodesRotateKey(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes rotate-key", "print the rotation result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesRotateKeyUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "nodes rotate-key", "node id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.RotateNodeMeshKey(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rotate mesh key for node %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, result, func() {
		_, _ = fmt.Fprintf(stdout, "node %q rotated: %s -> %s\n", id, result.OldPublicKey, result.NewPublicKey)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func nodesRotateKeyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes rotate-key <id> [flags]

Generates a fresh WireGuard keypair for node <id> and makes it that
node's live mesh identity immediately. The mesh reconciler propagates the
new public key to every other node on its next pass; watch "%[1]s nodes
mesh" and its "rotation" field to see when every reachable peer has
caught up. A brief reconnect blip on this node's mesh traffic is possible
until it does.

Only the node running the target control plane itself can be rotated
today; rotating a remote node returns a 501 explaining why (the agent
wire extension key rotation needs for a remote node does not exist yet).

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the rotation result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
