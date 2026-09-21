package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsDatabase dispatches "apps database <verb> [flags]" to one of
// set/clear: PUT/DELETE /api/v1/apps/{name}/database
// (internal/api/apps_database.go), attaching or detaching an
// already-created managed database as an app's connection-env-var
// source, the same "set/clear, no get" shape apps_storage.go's own
// "apps storage" already establishes (the currently attached database
// already shows up on "apps get <name>").
func runAppsDatabase(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsDatabaseUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsDatabaseUsage(prog))
		return exitOK
	case "set":
		return runAppsDatabaseSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runAppsDatabaseClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps database subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsDatabaseUsage(prog))
		return exitUsage
	}
}

func appsDatabaseUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps database set <name> --database-name NAME [--env-var VAR] [--field FIELD]   attach a managed database
  %[1]s apps database clear <name> [flags]                                                detach an app's database

Attaching a database injects a connection env var (default DATABASE_URL,
field "url") into the app's container at container-create time. See
"%[1]s databases list" for existing databases.

Run "%[1]s apps database <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsDatabaseSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps database set", "print the resulting attachment as JSON to stdout and nothing else", stderr)
	var databaseName, envVar, field string
	fs.StringVar(&databaseName, "database-name", "", "an already-created database's name (required)")
	fs.StringVar(&envVar, "env-var", "", "env var name to inject (default DATABASE_URL)")
	fs.StringVar(&field, "field", "", "database field to resolve (default \"url\", engine-dependent otherwise)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps database set <name> --database-name NAME [flags]\n\nAttaches an already-created managed database to <name> as its\nconnection-env-var source.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "apps database set", "app name")
	if !ok {
		return exitUsage
	}
	if databaseName == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps database set requires --database-name\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.SetAppDatabaseAttachment(context.Background(), name, setAppDatabaseRequest{
		DatabaseName: databaseName,
		EnvVar:       envVar,
		Field:        field,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set database for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printAppDatabaseHuman(stdout, result) })
}

func runAppsDatabaseClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps database clear", "print {\"cleared\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps database clear <name> [flags]\n\nDetaches whatever database <name> currently resolves its connection\nenv var from.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps database clear", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	if err := client.ClearAppDatabaseAttachment(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear database for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"cleared": true}, func() {
		_, _ = fmt.Fprintf(stdout, "database detached for app %q\n", name)
	})
}

func printAppDatabaseHuman(out io.Writer, r appDatabaseResource) {
	_, _ = fmt.Fprintf(out, "app_name:      %s\n", r.AppName)
	_, _ = fmt.Fprintf(out, "database_name: %s\n", r.DatabaseName)
	_, _ = fmt.Fprintf(out, "env_var:       %s\n", r.EnvVar)
	_, _ = fmt.Fprintf(out, "field:         %s\n", r.Field)
}
