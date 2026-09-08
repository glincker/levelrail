package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

// runAppsExport implements "apps export <name> [file]": GET
// /api/v1/apps/{name}/spec (internal/api/app_spec_export.go), written to
// file (default "<name>.yaml" in the current directory). --force is the
// same overwrite-guard convention apps_secrets.go's --force flag
// establishes: a local file the caller must opt into overwriting is
// treated as a mistake to guard against by default, not an oversight to
// silently correct.
func runAppsExport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps export", "print the exported app.yaml as JSON ({\"file\": ...}) to stdout and nothing else", stderr)
	var force bool
	fs.BoolVar(&force, "force", false, "overwrite the output file if it already exists")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsExportUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 1 && len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps export requires an app name and an optional output file\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]
	file := name + ".yaml"
	if len(rest) == 2 {
		file = rest[1]
	}

	if !force {
		if _, statErr := os.Stat(file); statErr == nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("%s already exists; pass --force to overwrite it", file))
		} else if !os.IsNotExist(statErr) {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("check %s: %w", file, statErr))
		}
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	data, err := client.ExportAppSpec(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("export app spec for %q: %w", name, err))
	}

	if err := os.WriteFile(file, data, 0o644); err != nil { //nolint:gosec // app.yaml is meant to be readable by whoever deploys with it, the same permissions git itself would give a checked-in file
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("write %s: %w", file, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]string{"file": file}, func() {
		_, _ = fmt.Fprintf(stdout, "wrote %s\n", file)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appsExportUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps export <name> [file] [flags]

Reconstructs <name>'s current desired state as an app.yaml document and
writes it to [file] (default "<name>.yaml" in the current directory).
Two known, deliberate fidelity losses versus a hand-written app.yaml:
a { secret: true } env var loses its original required flag (written
back as not required), and a { from: ... } cross-resource reference is
unrecoverable (only the already-resolved literal value is stored). The
build: block is always empty: build configuration isn't tracked in
desired state, so fill one in by hand before using the file to deploy.

Flags:
  --force                  overwrite the output file if it already exists
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout and nothing else
  --output string          output format: json, table, or text (default table)
  --query string           JMESPath expression to filter the result before printing
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
