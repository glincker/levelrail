package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runDatabasesGet implements "databases get <name>": GET
// /api/v1/databases/{name}.
func runDatabasesGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" databases get", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag string
	var jsonOut bool
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	var profileFlag string
	fs.StringVar(&profileFlag, "profile", "", "named credentials profile to read (overrides "+envProfile+", default \""+defaultProfile+"\")")
	fs.BoolVar(&jsonOut, "json", false, "print the database as JSON to stdout and nothing else")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases get <name> [flags]\n\nShows one managed database's current desired state.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: databases get requires exactly one database name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	profile := resolveProfile(profileFlag, lookupEnv)
	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog, profile), resolveToken(tokenFlag, lookupEnv, prog, profile))

	db, err := client.GetDatabase(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get database %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, db); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printDatabaseHuman(stdout, db)
	return exitOK
}
