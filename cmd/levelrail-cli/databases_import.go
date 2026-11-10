package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

// runDatabasesImport is the reverse of "databases export": it streams a dump into a managed database.
func runDatabasesImport(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases import", "print the import result as JSON to stdout and nothing else", stderr)
	var in string
	var force, yes bool
	fs.StringVar(&in, "in", "", "dump file to import, or - for stdin (required)")
	fs.BoolVar(&force, "force", false, "replace data the database already holds")
	fs.BoolVar(&yes, "yes", false, "skip the typed name confirmation that --force asks for")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesImportUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "databases import", "database name")
	if !ok {
		return exitUsage
	}
	if !validDatabaseName(name) {
		return reportError(stdout, stderr, jsonOut, newValidationError("database names use letters, digits, '-' and '_' and are at most 63 characters"))
	}
	if in == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--in is required: a dump file, or - for stdin"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	check, err := client.CheckDatabaseImport(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check database %q before import: %w", name, err))
	}
	confirm := ""
	if !check.Empty {
		if !force {
			return reportError(stdout, stderr, jsonOut, newValidationError("database %q already holds data; pass --force to replace it", name))
		}
		// Stdin may carry the dump, so a prompt can only read it when the dump comes from a file.
		switch {
		case yes:
		case in == "-":
			return reportError(stdout, stderr, jsonOut, newValidationError("--force with --in - needs --yes, stdin carries the dump"))
		default:
			if err := confirmDatabaseName(name, "", "Importing replaces the data in database "+name+".", stdin, stderr); err != nil {
				return reportError(stdout, stderr, jsonOut, err)
			}
		}
		confirm = name
	}

	src := stdin
	if in != "-" {
		f, err := os.Open(in) //nolint:gosec // the operator names the local dump to import
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("open %s: %w", in, err))
		}
		defer func() { _ = f.Close() }()
		src = f
	}
	res, err := client.ImportDatabase(ctx, name, src, !check.Empty, confirm)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("import into database %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "imported %d bytes into database %s\n", res.Bytes, name)
	})
}

func databasesImportUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases import <name> --in FILE|- [--force] [--yes] [flags]

Restores a dump into a managed database, the reverse of "databases export".
The dump must come from the same engine (postgres, mysql, mariadb, mongodb,
redis or valkey). A database that already holds data is refused unless you pass
--force, which also asks you to type the database name; --yes skips that
prompt (needed with --in -). The dump is streamed, never printed or logged,
and its size is capped by the databases.import_max_bytes setting.

Flags:
  --in string        dump file to import, or - for stdin
  --force            replace data the database already holds
  --yes              skip the typed name confirmation
  --token string     API token (default: %[2]s env var, then the credentials file)
  --api-url string   control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string   named credentials profile to read (overrides APP_PROFILE, default "default")
  -h, --help         show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
