package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runDatabasesPooler dispatches "databases pooler enable|disable|show <database>".
func runDatabasesPooler(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesPoolerUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesPoolerUsage(prog))
		return exitOK
	case "enable", "disable", "show":
		return runDatabasesPoolerVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases pooler subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesPoolerUsage(prog))
		return exitUsage
	}
}

func databasesPoolerUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases pooler show <database> [flags]      pooler state, pooled and direct URLs
  %[1]s databases pooler enable <database> [--pool-size N] [--max-client-conn N]   run a transaction pooler beside a Postgres database
  %[1]s databases pooler disable <database> [flags]   remove the pooler, apps go back to the direct URL

Linked apps get the pooled URL from the "url" field; "direct_url" always
bypasses the pooler, use it for migrations.
`, prog)
}

func runDatabasesPoolerVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cmd := "databases pooler " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, cmd, "print the pooler state as JSON to stdout and nothing else", stderr)
	var poolSize, maxClientConn *int
	if verb == "enable" {
		poolSize = fs.Int("pool-size", 0, "server connections per database (0 keeps the control plane default)")
		maxClientConn = fs.Int("max-client-conn", 0, "client connections the pooler accepts (0 keeps the control plane default)")
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s <database> [flags]\n\nFlags:\n", prog, cmd)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, cmd, "database name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	var res apiclient.DatabasePoolerResource
	var err error
	if verb == "show" {
		res, err = client.GetDatabasePooler(context.Background(), name)
	} else if verb == "enable" {
		res, err = client.EnableDatabasePooler(context.Background(), name, apiclient.DatabasePoolerTuning{PoolSize: *poolSize, MaxClientConn: *maxClientConn})
	} else {
		res, err = client.SetDatabasePooler(context.Background(), name, verb == "enable")
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for database %q: %w", cmd, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		printPooler(stdout, name, res)
	})
}

func printPooler(w io.Writer, name string, res apiclient.DatabasePoolerResource) {
	if !res.Supported {
		_, _ = fmt.Fprintf(w, "database %q does not support the connection pooler (postgres only)\n", name)
		return
	}
	state := "disabled"
	if res.Enabled {
		state = "enabled"
		if res.Status != "" {
			state += " (" + res.Status + ")"
		}
	}
	_, _ = fmt.Fprintf(w, "pooler:     %s\n", state)
	if res.PoolSize > 0 || res.MaxClientConn > 0 {
		_, _ = fmt.Fprintf(w, "sizing:     pool-size=%d max-client-conn=%d (0 = default)\n", res.PoolSize, res.MaxClientConn)
	}
	if res.PooledURL != "" {
		_, _ = fmt.Fprintf(w, "pooled url: %s\n", res.PooledURL)
	}
	_, _ = fmt.Fprintf(w, "direct url: %s\n", res.DirectURL)
	if res.Reason != "" && res.Status != "running" {
		_, _ = fmt.Fprintf(w, "reason:     %s %s\n", res.Reason, res.Message)
	}
	if !res.ImageConfigured {
		_, _ = fmt.Fprintln(w, "note:       no pooler image is configured on the control plane")
	}
}
