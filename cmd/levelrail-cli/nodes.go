package main

import (
	"fmt"
	"io"
)

// Routes are AbilityRoot-gated server-side, so the token must carry the root ability.
func runNodes(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, nodesUsage(prog))
		return exitUsage
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, nodesUsage(prog))
		return exitOK
	case "list":
		return runNodesList(prog, rest, stdout, stderr, lookupEnv)
	case "get":
		return runNodesGet(prog, rest, stdout, stderr, lookupEnv)
	case "delete":
		return runNodesDelete(prog, rest, stdout, stderr, lookupEnv)
	case "join-token":
		return runNodesJoinToken(prog, rest, stdout, stderr, lookupEnv)
	case "join-tokens":
		return runNodesJoinTokens(prog, rest, stdout, stderr, lookupEnv)
	case "revoke-join-token":
		return runNodesRevokeJoinToken(prog, rest, stdout, stderr, lookupEnv)
	case "cordon":
		return runNodesCordon(prog, rest, stdout, stderr, lookupEnv)
	case "uncordon":
		return runNodesUncordon(prog, rest, stdout, stderr, lookupEnv)
	case "drain":
		return runNodesDrain(prog, rest, stdout, stderr, lookupEnv)
	case "workloads":
		return runNodesWorkloads(prog, rest, stdout, stderr, lookupEnv)
	case "health":
		return runNodesHealth(prog, rest, stdout, stderr, lookupEnv)
	case "patch-status":
		return runNodesPatchStatus(prog, rest, stdout, stderr, lookupEnv)
	case "events":
		return runNodesEvents(prog, rest, stdout, stderr, lookupEnv)
	case "metrics":
		return runNodesMetrics(prog, rest, stdout, stderr, lookupEnv)
	case "resource-usage":
		return runNodesResourceUsage(prog, rest, stdout, stderr, lookupEnv)
	case "capacity-forecast":
		return runNodesCapacityForecast(prog, rest, stdout, stderr, lookupEnv)
	case "capacity":
		return runNodesCapacity(prog, rest, stdout, stderr, lookupEnv)
	case "cleanup":
		return runNodesCleanup(prog, rest, stdout, stderr, lookupEnv)
	case "mesh":
		return runNodesMesh(prog, rest, stdout, stderr, lookupEnv)
	case "rotate-key":
		return runNodesRotateKey(prog, rest, stdout, stderr, lookupEnv)
	case "rejoin-mesh":
		return runNodesRejoinMesh(prog, rest, stdout, stderr, lookupEnv)
	case "reenroll-token":
		return runNodesReenrollToken(prog, rest, stdout, stderr, lookupEnv)
	case "revoke-cert":
		return runNodesRevokeCert(prog, rest, stdout, stderr, lookupEnv)
	case "security":
		return runNodesSecurity(prog, rest, stdout, stderr, lookupEnv)
	case "cert":
		return runNodesCert(prog, rest, stdout, stderr, lookupEnv)
	case "providers":
		return runNodesProviders(prog, rest, stdout, stderr, lookupEnv)
	case "provision":
		return runNodesProvision(prog, rest, stdout, stderr, lookupEnv)
	case "provisions":
		return runNodesProvisions(prog, rest, stdout, stderr, lookupEnv)
	case "ssh-provision":
		return runNodesSSHProvision(prog, rest, stdout, stderr, lookupEnv)
	case "ssh-provisions":
		return runNodesSSHProvisions(prog, rest, stdout, stderr, lookupEnv)
	case "topology":
		return runNodesTopology(prog, rest, stdout, stderr, lookupEnv)
	case "traffic":
		return runNodesTraffic(prog, rest, stdout, stderr, lookupEnv)
	case "add":
		return runNodesAdd(prog, rest, stdout, stderr, lookupEnv)
	case "move":
		return runNodesMove(prog, rest, stdout, stderr, lookupEnv)
	case "moves":
		return runNodesMoves(prog, rest, stdout, stderr, lookupEnv)
	case "drains":
		return runNodesDrains(prog, rest, stdout, stderr, lookupEnv)
	case "labels":
		return runNodesLabels(prog, rest, stdout, stderr, lookupEnv)
	case "zones":
		return runNodesZones(prog, rest, stdout, stderr, lookupEnv)
	case "simulate":
		return runNodesSimulate(prog, rest, stdout, stderr, lookupEnv)
	case "failure":
		return runNodesFailure(prog, rest, stdout, stderr, lookupEnv)
	case "failure-policy":
		return runNodesFailurePolicy(prog, rest, stdout, stderr, lookupEnv)
	case "lost":
		return runNodesLost(prog, rest, stdout, stderr, lookupEnv)
	case "failure-resolve":
		return runNodesFailureResolve(prog, rest, stdout, stderr, lookupEnv)
	case "allocations":
		return runNodesAllocations(prog, rest, stdout, stderr, lookupEnv)
	case "placement":
		return runNodesPlacement(prog, rest, stdout, stderr, lookupEnv)
	case "connectivity":
		return runNodesConnectivity(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown nodes subcommand %q\n\n", prog, sub)
		_, _ = fmt.Fprint(stderr, nodesUsage(prog))
		return exitUsage
	}
}

func nodesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes list [flags]                                          list every node
  %[1]s nodes add [--name N] [--mesh] [flags]                        print a one-command join for a new server (single-use token)
  %[1]s nodes get <id> [flags]                                      show one node
  %[1]s nodes delete <id> [flags]                                    delete a node (fails if it still has placements, drain first)
  %[1]s nodes join-token [flags]                                     mint a one-time enrollment token, shown once
  %[1]s nodes join-tokens [flags]                                    list pending (unused, unexpired) join tokens
  %[1]s nodes revoke-join-token <id>|--all [flags]                   revoke pending join tokens at once
  %[1]s nodes cordon <id> [flags]                                    mark a node unschedulable, without evacuating it
  %[1]s nodes uncordon <id> [flags]                                  mark a node schedulable again
  %[1]s nodes drain <id> [--target ID] [--wait] [flags]              cordon, then move every database and app off a node
  %[1]s nodes drains <id> [flags]                                    show a node's drains and their moves
  %[1]s nodes move <app|database> <name> [--to ID] [--wait] [flags]  move one app or database, verified, with rollback
  %[1]s nodes moves [--kind K] [--resource N] [flags]                list node moves
  %[1]s nodes labels <id> [key=value ...] [flags]                    show or replace a node's labels
  %[1]s nodes placement <app|database> <name> [key=value ...]        show or set which node labels a resource requires
  %[1]s nodes zones [flags]                                          failure domains with their nodes and workloads
  %[1]s nodes simulate [--cpu M] [--memory MiB] [--count N]          dry run: where new apps would land and why
  %[1]s nodes failure --zone Z | --node ID                           dry run: where workloads go if a zone or node dies
  %[1]s nodes failure-policy get|set <id> [--mode M] [--grace S]    what happens when a node stays unreachable
  %[1]s nodes lost <id> [--dry-run] [--yes]                          declare a node lost, showing the plan first
  %[1]s nodes failure-resolve <id> <kind> <name> --wait|--restore    decide for an app or database with node-local data
  %[1]s nodes allocations [flags]                                requested versus allocatable CPU, memory and disk per node
  %[1]s nodes connectivity [flags]                                   WireGuard health of every node pair
  %[1]s nodes workloads <id> --accepts-app --accepts-build [flags]   set a node's accepted workload kinds
  %[1]s nodes health <id> [flags]                                    show a node's current reconcile conditions
  %[1]s nodes patch-status <id> [flags]                              show a node's latest available-OS-updates reading
  %[1]s nodes events <id> [--limit N] [flags]                        show a node's recent online/offline/cordon transitions
  %[1]s nodes metrics <id> --metric NAME [flags]                     query a node's metric time series
  %[1]s nodes resource-usage [flags]                                 show every node's latest CPU/memory/disk usage, plus a fleet rollup
  %[1]s nodes capacity-forecast <id> [flags]                        project disk/memory usage forward, roughly how many days until full
  %[1]s nodes capacity <node> [flags]                               show disk, inode, memory and swap headroom against thresholds
  %[1]s nodes cleanup <node> --dry-run|--yes [flags]                preview or run the safe cleanup on the control plane node
  %[1]s nodes mesh [flags]                                           show this control plane's live WireGuard mesh state and peers
  %[1]s nodes rotate-key <id> [flags]                                rotate a node's WireGuard key (local or remote)
  %[1]s nodes rejoin-mesh <id> [flags]                               force an immediate mesh resync for a peer that looks stuck
  %[1]s nodes reenroll-token <id> [flags]                            mint a one-time token to re-issue a node's agent certificate, shown once
  %[1]s nodes revoke-cert <id> [flags]                               revoke a node's agent certificate and disconnect it
  %[1]s nodes security <id> [--rejected] [flags]                     show how a node's agent is verified: fingerprints, renewal, recent security events
  %[1]s nodes cert renew <id> [flags]                                ask a connected node to renew its agent certificate now
  %[1]s nodes cert drill <id> [flags]                                rotate its certificate end to end and report each step
  %[1]s nodes providers list|set-credential [flags]                  manage cloud provider credentials (hetzner, digitalocean)
  %[1]s nodes provision --provider --region --size --name [flags]    create a server at a cloud provider and enroll it as a node
  %[1]s nodes provisions list|show <id> [flags]                      track a cloud node provision through to enrollment
  %[1]s nodes ssh-provision --host --user (--key-file|--password) --name [flags]   adopt a machine you already have over SSH and enroll it as a node
  %[1]s nodes ssh-provisions list|show <id> [flags]                  track an SSH node provision through to enrollment
  %[1]s nodes topology [flags]                                       whole-mesh summary: nodes, apps, databases, load balancers, connections
  %[1]s nodes traffic [flags]                                        per-domain ingress reachability and TLS status (the Traffic dashboard page)

Run "%[1]s nodes <subcommand> -h" for a subcommand's own flags.
`, prog)
}
