package main

import (
	"context"
	"fmt"
	"io"
)

// runPITRDisable implements "pitr disable <database>": DELETE
// /api/v1/databases/{name}/pitr (internal/api/pitr.go's own
// handleDisablePITR), AbilityWriteSensitive-gated. Existing base backups
// and already-archived WAL are kept; only new archiving stops.
func runPITRDisable(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "pitr disable", `print {"status": "disabled"} as JSON to stdout on success and nothing else`, stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pitrDisableUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "pitr disable", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DisablePITR(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disable pitr for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"status": "disabled"}, func() {
		_, _ = fmt.Fprintf(stdout, "point-in-time restore disabled for database %q; existing base backups and archived WAL are kept\n", name)
	})
}

func pitrDisableUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s pitr disable <database> [flags]

Turns off continuous WAL archiving for <database>. Existing base
backups and already-archived WAL stay on disk; only new archiving
stops, so points up through now remain recoverable.

Flags:
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print a minimal JSON result to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
