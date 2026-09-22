package main

import (
	"context"
	"fmt"
	"io"
)

// runPITRStatus implements "pitr status <database>": GET
// /api/v1/databases/{name}/pitr (internal/api/pitr.go's own
// handleGetPITRStatus), AbilityRead-gated: whether PITR is enabled and,
// if so, the currently recoverable window a restore can target.
func runPITRStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "pitr status", "print pitr status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pitrStatusUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "pitr status", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	status, err := client.GetPITRStatus(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get pitr status for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, status, func() {
		if !status.Enabled {
			_, _ = fmt.Fprintf(stdout, "point-in-time restore is not enabled for database %q\n", name)
			return
		}
		_, _ = fmt.Fprintf(stdout, "enabled since %s\n", status.EnabledAt)
		switch {
		case status.WindowError != "":
			_, _ = fmt.Fprintf(stdout, "recoverable window unavailable right now: %s\n", status.WindowError)
		case status.HasBaseBackup:
			_, _ = fmt.Fprintf(stdout, "recoverable window: %s to %s\n", status.WindowStart, status.WindowEnd)
		default:
			_, _ = fmt.Fprintln(stdout, "no succeeded base backup yet; take one with \"pitr base-backups trigger\"")
		}
	})
}

func pitrStatusUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s pitr status <database> [flags]

Shows whether point-in-time restore is enabled for <database> and, if
so, the exact window (earliest and latest timestamp) a restore can
target right now.

Flags:
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print pitr status as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
