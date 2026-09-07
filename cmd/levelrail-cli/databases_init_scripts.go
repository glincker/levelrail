package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

// runDatabasesInitScripts dispatches "databases init-scripts <verb>
// [args] [flags]" to list/create/update/delete, mirroring
// runDomainsBasicAuth's own nested-dispatch shape for a different
// per-database sub-resource.
func runDatabasesInitScripts(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesInitScriptsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesInitScriptsUsage(prog))
		return exitOK
	case "list":
		return runDatabasesInitScriptsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runDatabasesInitScriptsCreate(prog, args[1:], stdout, stderr, lookupEnv, stdin)
	case "update":
		return runDatabasesInitScriptsUpdate(prog, args[1:], stdout, stderr, lookupEnv, stdin)
	case "delete":
		return runDatabasesInitScriptsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases init-scripts subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesInitScriptsUsage(prog))
		return exitUsage
	}
}

func databasesInitScriptsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases init-scripts list <database> [flags]
  %[1]s databases init-scripts create <database> --filename NAME --file PATH [flags]
  %[1]s databases init-scripts update <database> <id> --filename NAME --file PATH [flags]
  %[1]s databases init-scripts delete <database> <id> [flags]

Named SQL/shell files mounted into a managed database's container at
/docker-entrypoint-initdb.d, the directory postgres/mysql/mariadb/mongodb's
own official images already auto-execute, in filename-sorted order, the
first time (and only the first time) the container starts against an
empty data volume. Adding or editing a script has no effect on a
database that has already initialized once, unless it's recreated from
scratch: that's Docker's own documented behavior, not something this
platform can override. --file - reads content from stdin.

Run "%[1]s databases init-scripts <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func printDatabaseInitScriptsTable(out io.Writer, scripts []databaseInitScriptResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tFILENAME\tUPDATED_AT")
	for _, s := range scripts {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", s.ID, s.Filename, s.UpdatedAt)
	}
	_ = tw.Flush()
}

func runDatabasesInitScriptsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases init-scripts list", "print init scripts as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases init-scripts list <database> [flags]\n\nLists every init script attached to <database>, ordered by filename.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "databases init-scripts list", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	scripts, err := client.ListDatabaseInitScripts(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list init scripts for %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, scripts, func() { printDatabaseInitScriptsTable(stdout, scripts) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// readInitScriptContent reads --file's content: "-" means stdin, any
// other value is a real file path, matching the same "-" convention
// this CLI's other file-accepting flags already use.
func readInitScriptContent(file string, stdin io.Reader) (string, error) {
	if file == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(file) //nolint:gosec // operator-supplied CLI flag, same pattern apps_deploy_compose.go's own --file read uses
	if err != nil {
		return "", fmt.Errorf("read %s: %w", file, err)
	}
	return string(data), nil
}

func runDatabasesInitScriptsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases init-scripts create", "print the created init script as JSON to stdout and nothing else", stderr)
	var filename, file string
	fs.StringVar(&filename, "filename", "", "name the script executes under, e.g. 01-extensions.sql (required)")
	fs.StringVar(&file, "file", "", "path to the script's content, or - for stdin (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesInitScriptsCreateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "databases init-scripts create", "database name")
	if !ok {
		return exitUsage
	}
	if filename == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--filename is required"))
	}
	if file == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--file is required"))
	}
	content, err := readInitScriptContent(file, stdin)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreateDatabaseInitScript(context.Background(), name, setDatabaseInitScriptRequest{Filename: filename, Content: content})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create init script for %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "created init script %q (%s) for database %q\n", created.Filename, created.ID, name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func databasesInitScriptsCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases init-scripts create <database> --filename NAME --file PATH [flags]

Flags:
  --filename string       name the script executes under, e.g. 01-extensions.sql (required)
  --file string           path to the script's content, or - for stdin (required)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the created init script as JSON to stdout, nothing else
  --output string         output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string          JMESPath expression to filter the result before printing
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runDatabasesInitScriptsUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases init-scripts update", "print the updated init script as JSON to stdout and nothing else", stderr)
	var filename, file string
	fs.StringVar(&filename, "filename", "", "new filename (required)")
	fs.StringVar(&file, "file", "", "path to the script's new content, or - for stdin (required)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases init-scripts update <database> <id> --filename NAME --file PATH [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: databases init-scripts update requires a database name and a script id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, id := rest[0], rest[1]
	if filename == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--filename is required"))
	}
	if file == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--file is required"))
	}
	content, err := readInitScriptContent(file, stdin)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.UpdateDatabaseInitScript(context.Background(), name, id, setDatabaseInitScriptRequest{Filename: filename, Content: content})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("update init script %q for %q: %w", id, name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, updated, func() {
		_, _ = fmt.Fprintf(stdout, "updated init script %q (%s) for database %q\n", updated.Filename, updated.ID, name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runDatabasesInitScriptsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases init-scripts delete", "print nothing on success", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases init-scripts delete <database> <id> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: databases init-scripts delete requires a database name and a script id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, id := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteDatabaseInitScript(context.Background(), name, id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete init script %q for %q: %w", id, name, err))
	}
	_, _ = fmt.Fprintf(stderr, "deleted init script %q for database %q\n", id, name)
	return exitOK
}
