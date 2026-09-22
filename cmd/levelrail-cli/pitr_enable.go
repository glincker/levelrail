package main

import (
	"context"
	"fmt"
	"io"
)

// runPITREnable implements "pitr enable <database>": POST
// /api/v1/databases/{name}/pitr (internal/api/pitr.go's own
// handleEnablePITR), AbilityWriteSensitive-gated. Returns 400 if the
// database's engine isn't postgres, the only engine PITR supports today.
func runPITREnable(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "pitr enable", `print {"status": "enabled"} as JSON to stdout on success and nothing else`, stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pitrEnableUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "pitr enable", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.EnablePITR(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("enable pitr for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"status": "enabled"}, func() {
		_, _ = fmt.Fprintf(stdout, "point-in-time restore enabled for database %q; take a base backup with \"%s pitr base-backups trigger %s --target ID\"\n", name, prog, name)
	})
}

func pitrEnableUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s pitr enable <database> [flags]

Turns on continuous WAL archiving for <database> going forward. Only
recoverable by timestamp from this moment onward; nothing before it.

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
