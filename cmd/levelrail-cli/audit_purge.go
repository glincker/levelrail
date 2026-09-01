package main

import (
	"context"
	"fmt"
	"io"
)

// runAuditPurge implements "audit-purge": POST /api/v1/audit-log/purge
// (internal/api/audit_retention.go), AbilityRoot-gated same as
// "audit-log". Deletes every entry older than the control plane's own
// configured retention window right now, for an operator who wants old
// entries cleared immediately rather than waiting for the next
// automatic sweep.
func runAuditPurge(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "audit-purge", "print the purge result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, auditPurgeUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.PurgeAuditLog(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("purge audit log: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, result); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "purged %d audit log entries\n", result.Deleted)
	return exitOK
}

func auditPurgeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s audit-purge [flags]

Deletes every audit log entry older than the control plane's own
configured retention window (APP_AUDIT_LOG_RETENTION_DAYS, default 90
days) right now, instead of waiting for the next automatic sweep.
Requires an admin/root-scoped token, the same as "%[1]s audit-log".

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the purge result as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
