package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runAppsMoves dispatches "apps moves <verb> [flags]" to one of
// list/get: GET /api/v1/apps/{name}/moves and .../moves/{id}
// (internal/api/apps_move_with_volumes.go), every "apps set-node
// --with-volumes"/"apps clear-node --with-volumes" attempt's own
// history. "apps set-node --with-volumes" already polls
// GET .../moves/{id} internally while a move is running; this is the
// standalone surface for inspecting past attempts afterward.
func runAppsMoves(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsMovesUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsMovesUsage(prog))
		return exitOK
	case "list":
		return runAppsMovesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runAppsMovesGet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps moves subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsMovesUsage(prog))
		return exitUsage
	}
}

func appsMovesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps moves list <name> [flags]      every node-to-node move attempt for name, newest first
  %[1]s apps moves get <name> <id> [flags]   show one move attempt's step-by-step progress

A "move" is what "apps set-node --with-volumes" (or "apps clear-node
--with-volumes") starts. Moving an app without --with-volumes doesn't
create one of these records: it's a plain, synchronous placement change.

Run "%[1]s apps moves <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsMovesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps moves list", "print moves as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps moves list <name> [flags]\n\nLists every node-to-node move attempt for <name>, newest first.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps moves list", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	moves, err := client.ListAppVolumeMoves(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list moves for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, moves, func() { printAppVolumeMovesTable(stdout, moves) })
}

func printAppVolumeMovesTable(out io.Writer, moves []appVolumeMoveResource) {
	if len(moves) == 0 {
		_, _ = fmt.Fprintln(out, "no moves")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tFROM_NODE\tTO_NODE\tSTATUS\tSTARTED_AT")
	for _, m := range moves {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", m.ID, m.FromNodeID, m.ToNodeID, m.Status, m.StartedAt)
	}
	_ = tw.Flush()
}

func runAppsMovesGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps moves get", "print the move as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps moves get <name> <id> [flags]\n\nShows one move attempt's step-by-step progress.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps moves get", "an app name and a move id", 2)
	if !ok {
		return exitUsage
	}
	name, id := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	move, err := client.GetAppVolumeMove(context.Background(), name, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get move %q for app %q: %w", id, name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, move, func() { printAppVolumeMoveHuman(stdout, move) })
}

func printAppVolumeMoveHuman(out io.Writer, m appVolumeMoveResource) {
	_, _ = fmt.Fprintf(out, "id:           %s\n", m.ID)
	_, _ = fmt.Fprintf(out, "service_name: %s\n", m.ServiceName)
	_, _ = fmt.Fprintf(out, "from_node_id: %s\n", m.FromNodeID)
	_, _ = fmt.Fprintf(out, "to_node_id:   %s\n", m.ToNodeID)
	_, _ = fmt.Fprintf(out, "status:       %s\n", m.Status)
	if m.Error != "" {
		_, _ = fmt.Fprintf(out, "error:        %s\n", m.Error)
	}
	_, _ = fmt.Fprintf(out, "started_at:   %s\n", m.StartedAt)
	if m.FinishedAt != "" {
		_, _ = fmt.Fprintf(out, "finished_at:  %s\n", m.FinishedAt)
	}
	if len(m.Steps) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "steps:")
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tSTATUS\tERROR")
	for _, s := range m.Steps {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Name, s.Status, s.Error)
	}
	_ = tw.Flush()
}
