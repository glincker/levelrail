package main

import (
	"fmt"
	"io"
)

// runNodes dispatches "nodes <verb> [args] [flags]" to one of
// list/get/delete/join-token/cordon/uncordon/drain/health/workloads, the
// same multi-verb dispatch shape runBackups uses. Every route these
// subcommands call is AbilityRoot-gated server-side (internal/api/nodes.go),
// so like "backups" these use the normal bearer token resolution every
// other command in this CLI uses, provided the token actually carries
// the root ability.
func runNodes(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, nodesUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, nodesUsage(prog))
		return exitOK
	case "list":
		return runNodesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runNodesGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runNodesDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "join-token":
		return runNodesJoinToken(prog, args[1:], stdout, stderr, lookupEnv)
	case "cordon":
		return runNodesCordon(prog, args[1:], stdout, stderr, lookupEnv)
	case "uncordon":
		return runNodesUncordon(prog, args[1:], stdout, stderr, lookupEnv)
	case "drain":
		return runNodesDrain(prog, args[1:], stdout, stderr, lookupEnv)
	case "workloads":
		return runNodesWorkloads(prog, args[1:], stdout, stderr, lookupEnv)
	case "health":
		return runNodesHealth(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown nodes subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, nodesUsage(prog))
		return exitUsage
	}
}

func nodesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes list [flags]                                          list every node
  %[1]s nodes get <id> [flags]                                      show one node
  %[1]s nodes delete <id> [flags]                                    delete a node (fails if it still has placements, drain first)
  %[1]s nodes join-token [flags]                                     mint a one-time enrollment token, shown once
  %[1]s nodes cordon <id> [flags]                                    mark a node unschedulable, without evacuating it
  %[1]s nodes uncordon <id> [flags]                                  mark a node schedulable again
  %[1]s nodes drain <id> [--target ID] [flags]                       move every service and database off a node
  %[1]s nodes workloads <id> --accepts-app --accepts-build [flags]   set a node's accepted workload kinds
  %[1]s nodes health <id> [flags]                                    show a node's current reconcile conditions

Run "%[1]s nodes <subcommand> -h" for a subcommand's own flags.
`, prog)
}
