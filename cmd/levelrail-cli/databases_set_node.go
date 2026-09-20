package main

import (
	"context"
	"fmt"
	"io"
)

// runDatabasesSetNode implements "databases set-node <name> <node-id>":
// client.SetDatabaseNode, the database counterpart to "apps set-node"
// (apps_set_node.go). Unlike the app version, there is no --with-volumes
// option: PUT /api/v1/databases/{name}/node has no move-with-volumes
// counterpart server-side.
func runDatabasesSetNode(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runDatabasesMoveNode(prog, args, stdout, stderr, lookupEnv, databaseMoveNodeConfig{
		cmdLabel:  "databases set-node",
		usage:     databasesSetNodeUsage,
		requireID: true,
	})
}

// runDatabasesClearNode implements "databases clear-node <name>": the
// same move runDatabasesSetNode makes, with an empty node_id, moving the
// database back to this control plane's own local node.
func runDatabasesClearNode(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runDatabasesMoveNode(prog, args, stdout, stderr, lookupEnv, databaseMoveNodeConfig{
		cmdLabel:  "databases clear-node",
		usage:     databasesClearNodeUsage,
		requireID: false,
	})
}

type databaseMoveNodeConfig struct {
	cmdLabel  string
	usage     func(string) string
	requireID bool
}

func runDatabasesMoveNode(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), cfg databaseMoveNodeConfig) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, cfg.cmdLabel, "print the updated database as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, cfg.usage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	wantCount, wantArgs := 1, "a database name"
	if cfg.requireID {
		wantCount, wantArgs = 2, "a database name and a node id"
	}
	if len(rest) != wantCount {
		_, _ = fmt.Fprintf(stderr, "%s: %s requires %s\n\n", prog, cfg.cmdLabel, wantArgs)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]
	nodeID := ""
	if cfg.requireID {
		nodeID = rest[1]
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.SetDatabaseNode(context.Background(), name, nodeID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set node for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, updated, func() {
		dest := updated.NodeID
		if dest == "" {
			dest = "this control plane (local)"
		}
		_, _ = fmt.Fprintf(stdout, "database %q moved to node %q\n", name, dest)
	})
}

func databaseMoveNodeFlagsUsage() string {
	return fmt.Sprintf(`Flags:
  --token string          API token (default: %s env var, then the credentials file)
  --api-url string       control plane base URL (default: %s env var, then %s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated database as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, envAPIToken, envAPIURL, defaultAPIURL)
}

func databasesSetNodeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases set-node <name> <node-id> [flags]

Moves a database's placement to node-id. See "%[1]s databases clear-node -h"
to move it back to this control plane's own local node.

`, prog) + databaseMoveNodeFlagsUsage()
}

func databasesClearNodeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases clear-node <name> [flags]

Moves a database back to this control plane's own local node.

`, prog) + databaseMoveNodeFlagsUsage()
}
