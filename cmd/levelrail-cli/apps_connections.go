package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// This file: "apps connect"/"apps disconnect"/"apps connections
// list|suggest", the CLI counterpart of internal/api's
// POST/GET/DELETE /api/v1/apps/{name}/connections[/{env_var}] and
// GET /api/v1/apps/{name}/connectable-databases (apps_connections.go):
// letting an app declare more than one managed database connection
// (store.DesiredService.DatabaseEnv) without app.yaml, the multi-
// connection sibling of "apps database set/clear" (apps_database.go's
// single-attachment shape).

// runAppsConnect dispatches "apps connect <app> <database> [flags]":
// POST /api/v1/apps/{name}/connections.
func runAppsConnect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps connect", "print the resulting connection as JSON to stdout and nothing else", stderr)
	var field, envVar string
	fs.StringVar(&field, "field", "", "database field to resolve: url, host, port, username, password, database (default \"url\")")
	fs.StringVar(&envVar, "env-var", "", "env var name to inject (default derived from the database name and field, e.g. MAIN_DATABASE_URL)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps connect <app> <database> [--field FIELD] [--env-var NAME] [flags]\n\nConnects <app> to an already-created managed database, injecting its\nresolved connection value as an env var at the app's next container\nstart. The database may be on a different node: it resolves to the\nmesh's own DNS name when mesh networking is configured, a plain Docker\ncontainer name otherwise (see \"apps connections list\").\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	cmd := twoArgCmd{prog: prog, cmdLabel: "apps connect", argsLabel: "an app name and a database name"}
	client, appName, dbName, jsonOut, of, exitCode, ok := parseTwoArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, cmd, lookupEnv)
	if !ok {
		return exitCode
	}

	result, err := client.CreateAppConnection(context.Background(), appName, createAppConnectionRequest{
		Database: dbName,
		Field:    field,
		EnvVar:   envVar,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("connect app %q to database %q: %w", appName, dbName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printAppConnectionHuman(stdout, result) })
}

// runAppsDisconnect dispatches "apps disconnect <app> <env-var> [flags]":
// DELETE /api/v1/apps/{name}/connections/{env_var}.
func runAppsDisconnect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps disconnect", "print {\"disconnected\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps disconnect <app> <env-var> [flags]\n\nRemoves one database connection from <app> by its env var name, as\nshown by \"apps connections list\". Idempotent: disconnecting an env var\nthat was never connected is not an error.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	cmd := twoArgCmd{prog: prog, cmdLabel: "apps disconnect", argsLabel: "an app name and an env var name"}
	client, appName, envVar, jsonOut, of, exitCode, ok := parseTwoArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, cmd, lookupEnv)
	if !ok {
		return exitCode
	}

	if err := client.DeleteAppConnection(context.Background(), appName, envVar); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disconnect %q from app %q: %w", envVar, appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"disconnected": true}, func() {
		_, _ = fmt.Fprintf(stdout, "%q disconnected from app %q\n", envVar, appName)
	})
}

// runAppsConnections dispatches "apps connections <verb> [flags]" to one
// of list/suggest.
func runAppsConnections(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsConnectionsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsConnectionsUsage(prog))
		return exitOK
	case "list":
		return runAppsConnectionsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "suggest":
		return runAppsConnectionsSuggest(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps connections subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsConnectionsUsage(prog))
		return exitUsage
	}
}

func appsConnectionsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps connections list <app> [flags]      list an app's current database connections
  %[1]s apps connections suggest <app> [flags]   list managed databases this app could connect to

See "%[1]s apps connect" and "%[1]s apps disconnect" to add or remove a
connection.

Run "%[1]s apps connections <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsConnectionsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps connections list", "print the connections as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps connections list <app> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, appName, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps connections list", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	connections, err := client.ListAppConnections(context.Background(), appName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list connections for app %q: %w", appName, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, connections, func() { printAppConnectionsHuman(stdout, connections) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runAppsConnectionsSuggest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps connections suggest", "print the candidate databases as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps connections suggest <app> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, appName, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps connections suggest", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	candidates, err := client.ListConnectableDatabases(context.Background(), appName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list connectable databases for app %q: %w", appName, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, candidates, func() { printConnectableDatabasesHuman(stdout, candidates) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printAppConnectionHuman(out io.Writer, r appConnectionResource) {
	_, _ = fmt.Fprintf(out, "env_var:       %s\n", r.EnvVar)
	_, _ = fmt.Fprintf(out, "database_name: %s\n", r.DatabaseName)
	_, _ = fmt.Fprintf(out, "field:         %s\n", r.Field)
	_, _ = fmt.Fprintf(out, "host:          %s (mesh_dns=%v cross_node=%v)\n", r.Host, r.MeshDNS, r.CrossNode)
}

func printAppConnectionsHuman(out io.Writer, connections []appConnectionResource) {
	if len(connections) == 0 {
		_, _ = fmt.Fprintln(out, "no database connections")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ENV_VAR\tDATABASE\tFIELD\tHOST\tMESH_DNS\tCROSS_NODE")
	for _, c := range connections {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%v\t%v\n", c.EnvVar, c.DatabaseName, c.Field, c.Host, c.MeshDNS, c.CrossNode)
	}
	_ = tw.Flush()
}

func printConnectableDatabasesHuman(out io.Writer, candidates []connectableDatabaseResource) {
	if len(candidates) == 0 {
		_, _ = fmt.Fprintln(out, "no managed databases to connect to")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tENGINE\tCROSS_NODE\tALREADY_CONNECTED\tCONNECTED_ENV_VARS")
	for _, d := range candidates {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%v\t%v\t%s\n", d.Name, d.Engine, d.CrossNode, d.AlreadyConnected, strings.Join(d.ConnectedEnvVars, ","))
	}
	_ = tw.Flush()
}
