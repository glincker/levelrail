package main

import (
	"fmt"
	"io"
	"os"
)

// runDatabases dispatches "databases <verb> [flags]" to one of
// create/list/get, the same three-verb shape "apps" started with (see
// apps.go's own doc comment on why create/list/get is the minimal set:
// create is the point, list/get are companions for verifying it worked).
func runDatabases(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesUsage(prog))
		return exitUsage
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesUsage(prog))
		return exitOK
	case "create":
		return runDatabasesCreate(prog, rest, stdout, stderr, lookupEnv, os.Stdin)
	case "connect":
		return runDatabasesConnect(prog, rest, os.Stdin, stdout, stderr, lookupEnv)
	case "adopt":
		return runDatabasesAdopt(prog, rest, os.Stdin, stdout, stderr, lookupEnv)
	case "probe":
		return runDatabasesProbe(prog, rest, stdout, stderr, lookupEnv)
	case "list":
		return runDatabasesList(prog, rest, stdout, stderr, lookupEnv)
	case "get":
		return runDatabasesGet(prog, rest, stdout, stderr, lookupEnv)
	case "delete":
		return runDatabasesDelete(prog, rest, stdout, stderr, lookupEnv)
	case "status":
		return runDatabasesStatus(prog, rest, stdout, stderr, lookupEnv)
	case "stop":
		return runDatabasesStop(prog, rest, stdout, stderr, lookupEnv)
	case "start":
		return runDatabasesStart(prog, rest, stdout, stderr, lookupEnv)
	case "resource-recommendation":
		return runDatabasesResourceRecommendation(prog, rest, stdout, stderr, lookupEnv)
	case "metrics":
		return runDatabasesMetrics(prog, rest, stdout, stderr, lookupEnv)
	case "logs":
		return runDatabasesLogs(prog, rest, stdout, stderr, lookupEnv)
	case "slow-queries":
		return runDatabasesSlowQueries(prog, rest, stdout, stderr, lookupEnv)
	case "query":
		return runDatabasesQuery(prog, rest, stdout, stderr, lookupEnv)
	case "schema":
		return runDatabasesSchema(prog, rest, stdout, stderr, lookupEnv)
	case "move-env":
		return runDatabasesMoveEnv(prog, rest, stdout, stderr, lookupEnv)
	case "set-project":
		return runDatabasesSetProject(prog, rest, stdout, stderr, lookupEnv)
	case "clear-project":
		return runDatabasesClearProject(prog, rest, stdout, stderr, lookupEnv)
	case "set-node":
		return runDatabasesSetNode(prog, rest, stdout, stderr, lookupEnv)
	case "clear-node":
		return runDatabasesClearNode(prog, rest, stdout, stderr, lookupEnv)
	case "public-access":
		return runDatabasesPublicAccess(prog, rest, stdout, stderr, lookupEnv)
	case "set-resources":
		return runDatabasesSetResources(prog, rest, stdout, stderr, lookupEnv)
	case "set-version":
		return runDatabasesSetVersion(prog, rest, stdout, stderr, lookupEnv)
	case "major-upgrade":
		return runDatabasesMajorUpgrade(prog, rest, stdout, stderr, lookupEnv)
	case "major-upgrades":
		return runDatabasesMajorUpgrades(prog, rest, stdout, stderr, lookupEnv)
	case "major-upgrade-rollback":
		return runDatabasesMajorUpgradeRollback(prog, rest, stdout, stderr, lookupEnv)
	case "major-upgrade-discard":
		return runDatabasesMajorUpgradeDiscard(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases subcommand %q\n\n", prog, sub)
		_, _ = fmt.Fprint(stderr, databasesUsage(prog))
		return exitUsage
	}
}

func databasesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases create [flags]     create a managed database
  %[1]s databases create --interactive  guided, step-by-step creation
  %[1]s databases connect --name N --engine E --host H [flags]  connect an existing database where it is, no data moved (--password-stdin, --test)
  %[1]s databases adopt --container C [--node N] [flags]  adopt a running database container without touching it (--list to browse)
  %[1]s databases probe <name> [flags]  re-check an external database's connectivity
  %[1]s databases list [flags]         list databases
  %[1]s databases get <name> [flags]   show one database
  %[1]s databases status <name> [flags]   show a database's current reconcile conditions
  %[1]s databases delete <name> [flags]  stop and remove a database, keep its data volume (--force if apps use it)
  %[1]s databases stop <name> [flags]     stop a database's container, keep its data
  %[1]s databases start <name> [flags]    bring a stopped database's container back
  %[1]s databases resource-recommendation <name> [flags]  suggest memory/CPU limits from historical usage
  %[1]s databases metrics <name> --metric NAME [flags]  query a database's metric time series
  %[1]s databases logs <name> [flags]     search a database's stored log entries
  %[1]s databases slow-queries <name> [flags]  list a Postgres/MySQL database's slow query log
  %[1]s databases schema <name> [--columns] [flags]  browse schemas, tables, columns, indexes
  %[1]s databases query <name> --sql "select ..." [flags]  run a read-only statement (--write, --explain)
  %[1]s databases set-project <name> <project-id> [flags]  move a database into a project
  %[1]s databases move-env <name> <environment-id> [--confirm] [flags]  move a database to another environment
  %[1]s databases clear-project <name> [flags]  remove a database's project assignment
  %[1]s databases set-node <name> <node-id> [flags]  move a database to another node
  %[1]s databases clear-node <name> [flags]  move a database back to the local node
  %[1]s databases public-access set <name> [flags]    expose a database on a host port
  %[1]s databases public-access clear <name> [flags]  return a database to internal-network-only
  %[1]s databases set-resources <name> [--memory 512Mi] [--cpu 0.5] [flags]  apply memory/CPU limits
  %[1]s databases set-version <name> <version> [flags]  minor or patch image change, same data
  %[1]s databases major-upgrade <name> --version V [flags]  guarded Postgres major upgrade with rollback snapshot
  %[1]s databases major-upgrades <name> [flags]  list a database's major upgrade attempts
  %[1]s databases major-upgrade-rollback <name> <id> [flags]  restore the pre-upgrade data
  %[1]s databases major-upgrade-discard <name> <id> [flags]  delete a rollback snapshot to free disk

Run "%[1]s databases <subcommand> -h" for a subcommand's own flags.
`, prog)
}
