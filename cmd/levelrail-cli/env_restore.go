package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
)

// runEnvRestore implements "env restore <app> <revision> --confirm <app>". History is kept: the restore is a new revision.
func runEnvRestore(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "env restore", "print the result as JSON to stdout and nothing else", stderr)
	var confirm string
	fs.StringVar(&confirm, "confirm", "", "the app name again, required to restore")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, envRestoreUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: env restore takes an app name and a revision number\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := fs.Arg(0)
	rev, err := strconv.Atoi(fs.Arg(1))
	if err != nil || rev < 1 {
		return reportError(stdout, stderr, jsonOut, newValidationError("revision must be a positive number"))
	}
	if confirm != name {
		return reportError(stdout, stderr, jsonOut, newValidationError("restoring env replaces the app's variables: pass --confirm %s to proceed", name))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.RestoreEnvRevision(context.Background(), name, rev, confirm)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore env revision %d of app %q: %w", rev, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		printEnvNameDiff(stdout, res.Added, res.Changed, res.Removed)
		_, _ = fmt.Fprintf(stdout, "restored revision %d of app %q as new revision %d; restart the app to apply\n", res.Restored, name, res.Revision)
	})
}

// printEnvNameDiff prints variable names only, never values.
func printEnvNameDiff(w io.Writer, added, changed, removed []string) {
	for _, k := range added {
		_, _ = fmt.Fprintf(w, "+ %s (added)\n", k)
	}
	for _, k := range changed {
		_, _ = fmt.Fprintf(w, "~ %s (changed)\n", k)
	}
	for _, k := range removed {
		_, _ = fmt.Fprintf(w, "- %s (removed)\n", k)
	}
}

func envRestoreUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s env restore <app> <revision> --confirm <app> [flags]

Puts an app's variables back to what an earlier revision held. The restore is
saved as a new revision, so history is never rewritten. Only variable names
are printed. The change reaches the running container at the next restart.

Flags:
  --confirm string   the app name again, required
  --token string     API token
  --api-url string   control plane base URL
  --profile string   named credentials profile to read
  --json             print the result as JSON to stdout, nothing else
  --output string    output format: json, table, or text
  --query string     JMESPath expression to filter the result before printing
  -h, --help         show this help
`, prog)
}
