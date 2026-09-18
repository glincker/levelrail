package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// moveNodePollInterval is how often "apps set-node --with-volumes" polls
// GET /api/v1/apps/{name}/moves/{id} while a move is still running,
// deliberately not a flag (unlike "apps wait --poll-interval"): a volume
// move is expected to finish in seconds to low minutes, not the long
// build/deploy windows "apps wait" watches, so there is no real case for
// tuning it per invocation.
const moveNodePollInterval = 500 * time.Millisecond

// runAppsSetNode implements "apps set-node <name> <node-id> [--with-volumes]":
// client.SetAppNode (instant, fresh empty volumes on the new node) without
// the flag, client.MoveAppWithVolumes (stop, copy each volume, move, resume)
// with it. See runAppsClearNode for moving back to the local node.
func runAppsSetNode(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runAppsMoveNode(prog, args, stdout, stderr, lookupEnv, moveNodeConfig{
		cmdLabel:  "apps set-node",
		usage:     appsSetNodeUsage,
		requireID: true,
	})
}

// runAppsClearNode implements "apps clear-node <name> [--with-volumes]":
// the same move runAppsSetNode makes, with an empty node_id, the store's
// own "empty node_id means this control plane's own local node"
// convention (store.DesiredService.NodeID's own doc comment).
func runAppsClearNode(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runAppsMoveNode(prog, args, stdout, stderr, lookupEnv, moveNodeConfig{
		cmdLabel:  "apps clear-node",
		usage:     appsClearNodeUsage,
		requireID: false,
	})
}

// moveNodeConfig holds what differs between "apps set-node" (an explicit
// destination node id argument) and "apps clear-node" (none, always
// moves to local), the same shared-implementation-plus-config shape
// deployOrRollbackConfig (apps_deploy.go) already establishes for a
// similar two-commands-one-mechanism pair.
type moveNodeConfig struct {
	cmdLabel  string
	usage     func(string) string
	requireID bool
}

func runAppsMoveNode(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), cfg moveNodeConfig) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, cfg.cmdLabel, "print the updated app (or, with --with-volumes, the finished move record) as JSON to stdout and nothing else", stderr)
	var withVolumes bool
	fs.BoolVar(&withVolumes, "with-volumes", false, "archive and restore every named Docker volume onto the destination node too; the app is stopped for the duration, briefly unavailable. Without this flag, only placement changes and the new node gets a fresh, empty volume instead of the old data")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, cfg.usage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	wantCount, wantArgs := 1, "an app name"
	if cfg.requireID {
		wantCount, wantArgs = 2, "an app name and a node id"
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
	ctx := context.Background()

	if !withVolumes {
		updated, err := client.SetAppNode(ctx, name, nodeID)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set node for app %q: %w", name, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, updated, func() {
			dest := updated.NodeID
			if dest == "" {
				dest = "this control plane (local)"
			}
			_, _ = fmt.Fprintf(stdout, "app %q moved to node %q\n", name, dest)
		})
	}

	move, err := client.MoveAppWithVolumes(ctx, name, nodeID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("move app %q with volumes: %w", name, err))
	}
	for move.Status == "running" {
		time.Sleep(moveNodePollInterval)
		move, err = client.GetAppVolumeMove(ctx, name, move.ID)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("poll move %q for app %q: %w", move.ID, name, err))
		}
	}
	if move.Status != "succeeded" {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("move app %q with volumes: %s (see step %q for what actually ran)", name, move.Error, lastMoveStepName(move)))
	}
	return writeScheduledTaskResult(stdout, stderr, of, move, func() {
		dest := move.ToNodeID
		if dest == "" {
			dest = "this control plane (local)"
		}
		_, _ = fmt.Fprintf(stdout, "app %q moved to node %q, volumes included\n", name, dest)
	})
}

// lastMoveStepName returns the name of the last step a move record ever
// attempted, "none" if it never got past starting: what a failed move's
// error message points a caller at, so "apps set-node --with-volumes"
// failing mid-flight tells them where to look (e.g. "apps get" or
// "apps status") rather than just "it failed".
func lastMoveStepName(move appVolumeMoveResource) string {
	if len(move.Steps) == 0 {
		return "none"
	}
	return move.Steps[len(move.Steps)-1].Name
}

// moveNodeFlagsUsage is the "Flags:" block appsSetNodeUsage and
// appsClearNodeUsage otherwise both spell out verbatim: dest is the only
// thing that differs between them ("the destination node" vs "the local
// node").
func moveNodeFlagsUsage(dest string) string {
	return fmt.Sprintf(`Flags:
  --with-volumes            archive and restore every named volume onto %s too (app briefly stopped)
  --token string          API token (default: %s env var, then the credentials file)
  --api-url string       control plane base URL (default: %s env var, then %s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, dest, envAPIToken, envAPIURL, defaultAPIURL)
}

func appsSetNodeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps set-node <name> <node-id> [flags]

Moves an app's placement to node-id. With --with-volumes, also archives
and restores every one of the app's named Docker volumes onto that node,
so its data moves too instead of the destination starting with fresh,
empty volumes (Dokploy calls this "transfer a service"; see
docs/multi-node.md's own "Moving an app with its volumes" section for
exactly what does and doesn't survive a partial failure).

`, prog) + moveNodeFlagsUsage("the destination node")
}

func appsClearNodeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps clear-node <name> [flags]

Moves an app back to this control plane's own local node. With
--with-volumes, also archives and restores every one of the app's named
Docker volumes onto the local node; see "%[1]s apps set-node -h" for the
full explanation, this is the identical mechanism with an empty
destination.

`, prog) + moveNodeFlagsUsage("the local node")
}
