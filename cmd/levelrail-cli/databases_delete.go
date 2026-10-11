package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runDatabasesDelete implements "databases delete <name>": DELETE
// /api/v1/databases/{name}.
func runDatabasesDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesDeleteUsage(prog)) }
	force := fs.Bool("force", false, "delete even while apps still connect to this database")
	deleteData := fs.Bool("delete-data", false, "also destroy the data volumes (needs --confirm NAME)")
	confirm := fs.String("confirm", "", "the database name again, required with --delete-data")

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "databases delete", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if *deleteData && *confirm != name {
		return reportError(stdout, stderr, jsonOut, newValidationError("--delete-data needs --confirm %s: the data is destroyed and cannot be recovered", name))
	}
	var err error
	if *deleteData {
		err = client.DeleteDatabaseAndData(context.Background(), name, *force)
	} else {
		err = client.DeleteDatabase(context.Background(), name, *force)
	}
	var apiErr *apiclient.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		err = client.DeleteExternalDatabase(context.Background(), name, *force)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete database %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "database %q deleted\n", name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func databasesDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases delete <name> [flags]

Stops a managed database and removes it from the control plane. Its data
volume and any backups in storage are kept. For an external database only
the local record is removed; the remote database is never contacted. Refuses while apps still connect
to it unless --force is given.

Flags:
  --force                   delete even while apps still connect to this database
  --delete-data             also destroy the data volumes (needs --confirm NAME); without it they are kept as orphaned data
  --confirm string          the database name again, required with --delete-data
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {"deleted": true} as JSON to stdout on success, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
