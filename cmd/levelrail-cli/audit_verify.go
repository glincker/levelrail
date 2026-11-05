package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func runAuditVerify(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "audit-verify", "print the verification report as JSON to stdout and nothing else", stderr)
	var anchorFlag string
	fs.StringVar(&anchorFlag, "anchor", "", "anchor export (file path or https URL) to check the live chain against")
	var rangeFlag string
	fs.StringVar(&rangeFlag, "range", "", `limit the report to a chain sequence range, "FROM-TO" ("-TO" or "FROM-" leave a side open)`)
	var statusFlag bool
	fs.BoolVar(&statusFlag, "anchor-status", false, "show the anchor webhook export state instead of verifying")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, auditVerifyUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if statusFlag {
		return runAuditAnchorStatus(client, jsonOut, of, stdout, stderr)
	}
	from, to, rerr := parseAuditRange(rangeFlag)
	if rerr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, rerr)
		return exitUsage
	}
	report, err := client.VerifyAuditLogRange(context.Background(), from, to)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("verify audit log: %w", err))
	}

	if anchorFlag != "" {
		return runAuditVerifyAnchor(client, anchorFlag, report, jsonOut, of, stdout, stderr)
	}

	if err := renderResult(stdout, of.Format, of.Query, report, func() {
		if report.OK {
			_, _ = fmt.Fprintf(stdout, "audit log intact: %d entries checked, %d checkpoints, head %d %s\n", report.Checked, report.Checkpoints, report.Head, report.HeadHash)
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
fails. With --anchor it also checks the live chain against an off-box
anchor export, which catches a rewrite or rollback done on the control plane.
Requires an admin/root-scoped token, the same as "%[1]s audit-log".

Flags:
  --anchor string         anchor export (file path or https URL) from APP_AUDIT_ANCHOR_*; also checks that the live chain still contains every anchored head
  --range string          limit the report to a chain sequence range, "FROM-TO" (either side may be empty)
  --anchor-status         show the anchor webhook export state (last exported sequence, last error) and exit
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the report as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}

func parseAuditRange(raw string) (from, to int64, err error) {
	if raw == "" {
		return 0, 0, nil
	}
	lo, hi, found := strings.Cut(raw, "-")
	if !found {
		return 0, 0, fmt.Errorf("--range must look like FROM-TO, got %q", raw)
	}
	for i, part := range []string{lo, hi} {
		if part == "" {
			continue
		}
		n, perr := strconv.ParseInt(part, 10, 64)
		if perr != nil || n < 0 {
			return 0, 0, fmt.Errorf("--range bound %q is not a sequence number", part)
		}
		if i == 0 {
			from = n
		} else {
			to = n
		}
	}
	if to != 0 && from > to {
		return 0, 0, fmt.Errorf("--range start is after its end")
	}
	return from, to, nil
}
