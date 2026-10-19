package main

import (
	"context"
	"fmt"
	"io"
)

func runAuditVerify(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "audit-verify", "print the verification report as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, auditVerifyUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	report, err := client.VerifyAuditLog(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("verify audit log: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, report, func() {
		if report.OK {
			_, _ = fmt.Fprintf(stdout, "audit log intact: %d entries checked, %d checkpoints\n", report.Checked, report.Checkpoints)
			return
		}
		_, _ = fmt.Fprintf(stdout, "audit log FAILED verification: %d problems\n", len(report.Problems))
		for _, p := range report.Problems {
			_, _ = fmt.Fprintf(stdout, "  entry %d: %s\n", p.Seq, p.Reason)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if !report.OK {
		return exitCheckFailed
	}
	return exitOK
}

func auditVerifyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s audit-verify [flags]

Walks the audit log's hash chain and signed checkpoints and reports any
entry that was edited, removed or cut off the end. Exits 1 when a check
fails. Requires an admin/root-scoped token, the same as "%[1]s audit-log".

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the report as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
