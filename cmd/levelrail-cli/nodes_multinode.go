package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/placement"
)

// runNodesAdd implements "nodes add": mint a short-lived single-use join
// token and print the one command that enrolls a new server with it.
func runNodesAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes add", "print the token and join commands as JSON", stderr)
	var req apiclient.CreateNodeJoinTokenRequest
	fs.StringVar(&req.NodeName, "name", "", "name the new node enrolls with (lowercase, digits, dashes)")
	fs.StringVar(&req.ControlPlaneAddr, "control-plane", "", "host:port the new server dials (default: the control plane's advertised agent address)")
	fs.BoolVar(&req.Mesh, "mesh", false, "join the WireGuard mesh (needed for cross-node database connections)")
	fs.StringVar(&req.Provider, "provider", "", "provider label (aws, gcp, azure, hetzner, digitalocean, home...); default: the agent auto-detects it from cloud metadata")
	fs.StringVar(&req.Region, "region", "", "region label; default: auto-detected")
	fs.StringVar(&req.Zone, "zone", "", "zone label; default: auto-detected")
	var preflight bool
	var timeout, interval time.Duration
	fs.BoolVar(&preflight, "preflight", false, "after printing the command, wait for the node to join and report its preflight checks (needs --name)")
	fs.DurationVar(&timeout, "timeout", defaultPreflightTimeout, "how long --preflight waits for the node")
	fs.DurationVar(&interval, "poll-interval", defaultPreflightInterval, "how often --preflight polls")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesAddUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if preflight && req.NodeName == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --preflight needs --name so the new node can be found\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreateNodeJoinTokenFor(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create join token: %w", err))
	}
	if preflight {
		return addWithPreflight(client, req.NodeName, created, timeout, interval, of, jsonOut, stdout, stderr)
	}
	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		if created.JoinCommands == nil {
			_, _ = fmt.Fprintf(stdout, "join token (single use, shown once): %s\n", created.Token)
			_, _ = fmt.Fprintln(stdout, "no control plane address is known: pass --control-plane host:port to get a ready command")
			return
		}
		printJoinCommands(stdout, created)
		_, _ = fmt.Fprintln(stdout, "Then watch it appear with: nodes list, or re-run with --preflight to wait and check it")
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runNodesMove implements "nodes move <app|database> <name> [--to NODE] [--wait]".
func runNodesMove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes move", "print the move as JSON", stderr)
	var to string
	var wait bool
	var timeout, interval time.Duration
	fs.StringVar(&to, "to", "", "target node id (default: the scheduler picks the least loaded eligible node)")
	fs.BoolVar(&wait, "wait", false, "wait until the move finishes")
	fs.DurationVar(&timeout, "timeout", 15*time.Minute, "how long --wait waits")
	fs.DurationVar(&interval, "poll-interval", 2*time.Second, "how often --wait polls")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesMoveUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	pos, ok := requireArgs(fs, stderr, prog, "nodes move", "<app|database> <name>", 2)
	if !ok {
		return exitUsage
	}
	kind, ok := parseMoveKind(pos[0], stderr, prog)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	m, err := client.StartMove(ctx, kind, pos[1], to)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("move %s %q: %w", kind, pos[1], err))
	}
	if wait {
		m, err = pollUntil(ctx, timeout, interval, m, func(ctx context.Context) (apiclient.NodeMoveResource, error) {
			return client.GetNodeMove(ctx, m.ID)
		}, apiclient.NodeMoveResource.Done)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("wait for move %s: %w", m.ID, err))
		}
	}
	if err := renderResult(stdout, of.Format, of.Query, m, func() { printMoveHuman(stdout, m) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if m.Status == "failed" || m.Status == "rolled_back" {
		return exitAPIError
	}
	return exitOK
}

func parseMoveKind(s string, stderr io.Writer, prog string) (string, bool) {
	switch s {
	case "app", "apps":
		return "app", true
	case "database", "databases", "db":
		return "database", true
	}
	_, _ = fmt.Fprintf(stderr, "%s: kind must be app or database, got %q\n", prog, s)
	return "", false
}

func printMoveHuman(out io.Writer, m apiclient.NodeMoveResource) {
	_, _ = fmt.Fprintf(out, "move %s: %s %s from %s to %s: %s (phase %s)\n", m.ID, m.Kind, m.Resource, nodeLabel(m.FromNodeID), nodeLabel(m.ToNodeID), m.Status, m.Phase)
	if m.Stateful {
		_, _ = fmt.Fprintf(out, "stateful: stops on the source, copies %s, then starts on the target\n", strings.Join(m.Volumes, ", "))
	}
	for _, s := range m.Steps {
		line := "  " + s.Phase + ": " + s.Outcome
		if s.Message != "" {
			line += " (" + s.Message + ")"
		}
		_, _ = fmt.Fprintln(out, line)
	}
	if m.Error != "" {
		_, _ = fmt.Fprintf(out, "error: %s\n", m.Error)
	}
}

// runNodesMoves implements "nodes moves [--kind K] [--resource NAME] [--status S]".
func runNodesMoves(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes moves", "print moves as JSON", stderr)
	var kind, resource, status string
	fs.StringVar(&kind, "kind", "", "app or database")
	fs.StringVar(&resource, "resource", "", "app or database name")
	fs.StringVar(&status, "status", "", "running, succeeded, failed or rolled_back")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "moves [--kind K] [--resource NAME] [--status S]", "Lists node moves, newest first."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	moves, err := client.ListNodeMoves(context.Background(), kind, resource, status)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list moves: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, moves, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tKIND\tNAME\tFROM\tTO\tSTATUS\tPHASE\tSTARTED")
		for _, m := range moves {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", m.ID, m.Kind, m.Resource, nodeLabel(m.FromNodeID), nodeLabel(m.ToNodeID), m.Status, m.Phase, m.StartedAt)
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runNodesLabels implements "nodes labels <id> [key=value ...] [--clear]":
// with no pairs it shows the labels, with pairs it replaces them.
func runNodesLabels(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes labels", "print labels as JSON", stderr)
	var clearAll bool
	fs.BoolVar(&clearAll, "clear", false, "remove every label")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "labels <id> [key=value ...] [--clear]", "Shows a node's labels, or replaces them with the given pairs."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() < 1 {
		_, _ = fmt.Fprintf(stderr, "%s: nodes labels needs a node id\n", prog)
		return exitUsage
	}
	id, pairs := fs.Arg(0), fs.Args()[1:]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var res apiclient.NodeLabels
	var err error
	if len(pairs) == 0 && !clearAll {
		res, err = client.GetNodeLabels(ctx, id)
	} else {
		labels, perr := placement.ParseLabels(pairs)
		if perr != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, perr)
			return exitUsage
		}
		res, err = client.SetNodeLabels(ctx, id, labels)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("node labels %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printKV(stdout, res.Labels, "no labels") }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runNodesPlacement implements "nodes placement <app|database> <name> [key=value ...] [--clear]".
func runNodesPlacement(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes placement", "print placement as JSON", stderr)
	var clearAll bool
	fs.BoolVar(&clearAll, "clear", false, "remove the node selector")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "placement <app|database> <name> [key=value ...] [--clear]",
			"Shows which nodes an app or database may run on, or sets the node labels it requires.\nSetting a selector never moves anything; the next move or drain honors it."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() < 2 {
		_, _ = fmt.Fprintf(stderr, "%s: nodes placement needs <app|database> <name>\n", prog)
		return exitUsage
	}
	kind, ok := parseMoveKind(fs.Arg(0), stderr, prog)
	if !ok {
		return exitUsage
	}
	name, pairs := fs.Arg(1), fs.Args()[2:]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var res apiclient.PlacementResource
	var err error
	if len(pairs) == 0 && !clearAll {
		res, err = client.GetPlacement(ctx, kind, name)
	} else {
		sel, perr := placement.ParseLabels(pairs)
		if perr != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, perr)
			return exitUsage
		}
		res, err = client.SetPlacement(ctx, kind, name, sel)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("placement of %s %q: %w", kind, name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		_, _ = fmt.Fprintf(stdout, "%s %s runs on %s\n", res.Kind, res.Name, nodeLabel(res.NodeID))
		_, _ = fmt.Fprintln(stdout, "node selector:")
		printKV(stdout, res.NodeSelector, "  (none: any node)")
		_, _ = fmt.Fprintf(stdout, "eligible nodes: %s\n", strings.Join(res.Eligible, ", "))
		for _, r := range res.Rejected {
			_, _ = fmt.Fprintf(stdout, "  not %s: %s\n", r.NodeID, r.Reason)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runNodesConnectivity implements "nodes connectivity": WireGuard health of every node pair.
func runNodesConnectivity(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes connectivity", "print the pair matrix as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "connectivity", "Shows the WireGuard handshake state of every node pair, as each node sees it."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetMeshConnectivity(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("mesh connectivity: %w", err))
	}
	bad := false
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "A\tB\tSTATUS\tA->B\tB->A\tHINT")
		for _, p := range res.Pairs {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", p.A, p.B, p.Status, p.AToB.State, p.BToA.State, p.Hint)
		}
		_ = tw.Flush()
		for _, n := range res.Nodes {
			if n.ViewError != "" {
				_, _ = fmt.Fprintf(stdout, "%s: could not read its mesh view: %s\n", n.ID, n.ViewError)
			}
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	for _, p := range res.Pairs {
		bad = bad || p.Status != "ok"
	}
	if bad {
		return exitAPIError
	}
	return exitOK
}

func printKV(out io.Writer, kv map[string]string, empty string) {
	if len(kv) == 0 {
		_, _ = fmt.Fprintln(out, empty)
		return
	}
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(out, "  %s=%s\n", k, kv[k])
	}
}

func nodesAddUsage(prog string) string {
	return nodesSimpleUsage(prog, "add [--name NAME] [--provider P] [--region R] [--zone Z] [--control-plane HOST:PORT] [--mesh] [--preflight]",
		"Mints a single-use join token (15 minutes by default, APP_NODE_JOIN_TOKEN_TTL on the\ncontrol plane changes it) and prints the one command that enrolls a new server.\nProvider, region and zone are detected from the cloud's metadata service when you do\nnot pass them. --preflight (needs --name) then waits for the node to join and prints\neach check with its fix, exiting non-zero if one failed. --timeout (default 10m) and\n--poll-interval (default 3s) control the wait.")
}

func nodesMoveUsage(prog string) string {
	return nodesSimpleUsage(prog, "move <app|database> <name> [--to NODE-ID] [--wait]",
		"Moves an app or database to another node. A stateless app starts on the target, is\nverified ready, then stops on the source. An app with volumes, or a database, stops,\nhas its volumes copied, then starts on the target. A target that never becomes ready\nrolls the move back. --wait exits non-zero when the move fails or rolls back.")
}

func nodesSimpleUsage(prog, synopsis, about string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes %[2]s [flags]

%[3]s

Flags:
  --token string          API token (default: %[4]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[5]s env var, then %[6]s)
  --profile string        named credentials profile to read
  --json                  print JSON to stdout, nothing else
  --output string         output format: json, table, or text
  --query string          JMESPath expression to filter the result
  -h, --help              show this help
`, prog, synopsis, about, envAPIToken, envAPIURL, defaultAPIURL)
}

// addWithPreflight prints the join command, then waits for the node and
// reports its preflight. The token is printed before waiting because it is
// shown only once and the wait can take minutes.
func addWithPreflight(client *apiclient.Client, name string, created apiclient.CreateNodeJoinTokenResponse, timeout, interval time.Duration, of outputFlags, jsonOut bool, stdout, stderr io.Writer) int {
	human := of.Format == "" || of.Format == "table" || of.Format == "text"
	if human && !jsonOut && created.JoinCommands != nil {
		printJoinCommands(stdout, created)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ob, err := waitForNodeOnboarding(ctx, client, name, interval, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := renderResult(stdout, of.Format, of.Query, ob, func() { printOnboarding(stdout, ob) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if ob.Summary.Fail > 0 {
		return exitAPIError
	}
	return exitOK
}

// printJoinCommands prints the Docker, Podman and binary variants so a Podman
// host is not handed a Docker-only command.
func printJoinCommands(out io.Writer, created apiclient.CreateNodeJoinTokenResponse) {
	c := created.JoinCommands
	_, _ = fmt.Fprintf(out, "Run this on the new server with Docker (single use, expires %s):\n\n%s\n\n", created.ExpiresAt.Format(time.RFC3339), c.Docker)
	if c.Podman != "" {
		_, _ = fmt.Fprintf(out, "With Podman (rootful; first run: sudo systemctl enable --now podman.socket):\n\n%s\n\n", c.Podman)
	}
	_, _ = fmt.Fprintf(out, "Or, with the agent binary installed (it auto-detects Docker or Podman):\n\n%s\n\n", c.Binary)
}
