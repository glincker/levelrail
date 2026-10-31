package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// doctorOutput is the --fix result: the report after fixing plus what was applied or left alone.
type doctorOutput struct {
	OK      bool                            `json:"ok"`
	Checks  []apiclient.DoctorCheckResource `json:"checks"`
	Applied []apiclient.FixResult           `json:"applied"`
	Skipped []doctorSkippedFix              `json:"skipped"`
}

type doctorSkippedFix struct {
	FixID  string `json:"fix_id"`
	Reason string `json:"reason"`
}

// runDoctor runs the control plane's health checks, or local-only ones with no server configured.
func runDoctor(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 {
		switch args[0] {
		case "paths":
			return runDoctorPaths(prog, args[1:], stdout, stderr, lookupEnv)
		}
	}
	diskOnly := len(args) > 0 && args[0] == "disk"
	if diskOnly {
		args = args[1:]
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "doctor", "print the doctor report as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, doctorUsage(prog)) }
	fix := fs.Bool("fix", false, "apply safe automatic fixes after printing the plan")
	yes := fs.Bool("yes", false, "with --fix: apply without asking for confirmation")
	local := fs.Bool("local", false, "run only the local checks (Docker, disk, ports), never contact a control plane")

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	ctx := context.Background()

	if *local || !doctorServerConfigured(tokenFlag, apiURLFlag, profileFlag, prog, lookupEnv) {
		if *fix {
			return reportError(stdout, stderr, jsonOut, newValidationError("--fix needs a control plane: sign in first, or pass --api-url and --token"))
		}
		report := filterDiskChecks(localDoctorReport(ctx, lookupEnv), diskOnly)
		if of.Format == outputTable {
			_, _ = fmt.Fprintln(stderr, "no control plane configured, running local checks only (Docker, disk, ports)")
		}
		return finishDoctor(stdout, stderr, of, report, report)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	report, err := client.GetSystemDoctor(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get system doctor report: %w", err))
	}
	if diskOnly {
		report = filterDiskChecks(report, true)
		return finishDoctor(stdout, stderr, of, report, report)
	}
	if !*fix {
		return finishDoctor(stdout, stderr, of, report, report)
	}

	out, err := applyDoctorFixes(ctx, client, report, *yes, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
	}
	return finishDoctor(stdout, stderr, of, out, systemDoctorResource{OK: out.OK, Checks: out.Checks})
}

func doctorServerConfigured(tokenFlag, apiURLFlag, profileFlag, prog string, lookupEnv func(string) (string, bool)) bool {
	if apiURLFlag != "" {
		return true
	}
	if v, ok := lookupEnv(envAPIURL); ok && v != "" {
		return true
	}
	return resolveToken(tokenFlag, lookupEnv, prog, resolveProfile(profileFlag, lookupEnv)) != ""
}

func finishDoctor(stdout, stderr io.Writer, of outputFlags, data any, report systemDoctorResource) int {
	if err := renderResult(stdout, of.Format, of.Query, data, func() { printSystemDoctorHuman(stdout, report) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if of.Format == outputTable && !report.OK {
		// Reuses exitUsage's value (1) as the generic not-all-clean exit code.
		return exitUsage
	}
	return exitOK
}

// applyDoctorFixes previews every auto fix the report offers, applies only the safe ones after confirmation, then re-runs the report.
func applyDoctorFixes(ctx context.Context, client *Client, report systemDoctorResource, yes bool, stderr io.Writer) (doctorOutput, error) {
	out := doctorOutput{OK: report.OK, Checks: report.Checks, Applied: []apiclient.FixResult{}, Skipped: []doctorSkippedFix{}}
	var plan []apiclient.Fix
	seen := map[string]bool{}
	for _, c := range report.Checks {
		if c.FixID == "" || seen[c.FixID] || (c.Status != "fail" && c.Status != "warn") {
			continue
		}
		seen[c.FixID] = true
		f, err := client.PreviewFix(ctx, c.FixID)
		if err != nil {
			return out, fmt.Errorf("preview fix %s: %w", c.FixID, err)
		}
		switch {
		case f.Kind != "auto":
			out.Skipped = append(out.Skipped, doctorSkippedFix{c.FixID, "not automatic (" + f.Kind + "), see the check's fix text"})
		case f.Risk != "safe":
			out.Skipped = append(out.Skipped, doctorSkippedFix{c.FixID, "risk is " + f.Risk + ", apply it on purpose with: fixes apply " + c.FixID})
		case f.State == "rate_limited" || f.State == "demoted":
			out.Skipped = append(out.Skipped, doctorSkippedFix{c.FixID, f.Blocked})
		default:
			plan = append(plan, f)
		}
	}
	for _, s := range out.Skipped {
		_, _ = fmt.Fprintf(stderr, "skipped %s: %s\n", s.FixID, s.Reason)
	}
	if len(plan) == 0 {
		_, _ = fmt.Fprintln(stderr, "no safe automatic fixes to apply")
		return out, nil
	}
	_, _ = fmt.Fprintf(stderr, "plan: %d safe fix(es)\n", len(plan))
	for _, f := range plan {
		printFixDetail(stderr, f)
		_, _ = fmt.Fprintln(stderr)
	}
	if !yes {
		if err := confirmDoctorFix(stderr, len(plan)); err != nil {
			return out, err
		}
	}
	for _, f := range plan {
		res, err := client.ApplyFix(ctx, f.ID, nil)
		if err != nil {
			return out, fmt.Errorf("apply fix %s: %w", f.ID, err)
		}
		_, _ = fmt.Fprintf(stderr, "%s: %s: %s\n", f.ID, res.Outcome, res.Detail)
		out.Applied = append(out.Applied, res)
	}
	after, err := client.GetSystemDoctor(ctx)
	if err != nil {
		return out, fmt.Errorf("re-run doctor: %w", err)
	}
	out.OK, out.Checks = after.OK, after.Checks
	return out, nil
}

func confirmDoctorFix(stderr io.Writer, n int) error {
	if f, ok := stdinSource.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // a terminal file descriptor always fits an int
		return errors.New("not a terminal: pass --yes to apply without confirmation")
	}
	_, _ = fmt.Fprintf(stderr, "Apply %d fix(es)? [y/N] ", n)
	line, _ := bufio.NewReader(stdinSource).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return errors.New("cancelled")
	}
	return nil
}

func doctorUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s doctor [flags]
  %[1]s doctor disk [flags]
  %[1]s doctor paths [flags]

Runs the control plane's health checks: Docker, disk and inodes, ports,
firewall, clock, DNS, certificates, backups and restore drills, node agents,
the WireGuard mesh, orphaned Docker data and dashboard https. The dashboard's
Doctor page runs the same checks.

With no control plane configured, only local checks run (Docker, disk, ports).

"doctor disk" is read-only: it shows only the disk checks (free space on the
data dir and Docker root, Docker image, build cache, volume and container
sizes, the top consumers) and a suggested cleanup command it never runs.
The warning level comes from APP_DOCTOR_DISK_WARN_FREE_PCT (percent free).

"doctor paths" is offline: it prints the binary, releases dir, data dir and
systemd unit locations on this host and the APP_HTTP_ADDR the unit binds.

Exit code is 0 if every check is ok or warn, 1 if any check fails.

Flags:
  --fix                     apply safe automatic fixes: prints the plan first and asks to confirm
  --yes                     with --fix, apply without asking
  --local                   run only the local checks, never contact a control plane
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the doctor report as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// filterDiskChecks keeps only the disk checks for "doctor disk"; other reports pass through.
func filterDiskChecks(r systemDoctorResource, diskOnly bool) systemDoctorResource {
	if !diskOnly {
		return r
	}
	var kept []doctorCheckResource
	for _, c := range r.Checks {
		switch c.Code {
		case "disk_space", "docker_root_disk", "disk_hygiene", "inodes", "docker_orphans":
			kept = append(kept, c)
		}
	}
	r.Checks = kept
	return r
}
