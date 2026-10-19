package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func secretsKeysUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s secrets key-status [flags]
  %[1]s secrets rotate-dek <app> [flags]
  %[1]s secrets rotate-dek --all [flags]
  %[1]s secrets key-policy [--max-age-days N] [--remind-every-days N] [flags]

key-status shows the master key age, each app's data key age and the
last rotate-all run. rotate-dek replaces an app's data key (or every
app's with --all) and re-encrypts its secrets in one transaction per app;
an interrupted --all run resumes where it stopped. It needs a root token
and asks for your password (and authenticator code). key-policy sets the
maximum key age that raises a reminder (0 turns it off).
`, prog)
}

func runSecretsKeyStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "secrets key-status", "print the status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, secretsKeysUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	status, err := client.GetEncryptionKeys(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get encryption keys: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, status, func() { printKeyStatusHuman(stdout, status) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runSecretsRotateDEK(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "secrets rotate-dek", "print the result as JSON to stdout and nothing else", stderr)
	all := fs.Bool("all", false, "rotate every app's data key (resumes an interrupted run)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, secretsKeysUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	app := ""
	switch rest := fs.Args(); {
	case *all && len(rest) == 0:
	case !*all && len(rest) == 1 && strings.TrimSpace(rest[0]) != "":
		app = strings.TrimSpace(rest[0])
	default:
		return reportError(stdout, stderr, jsonOut, newValidationError("give exactly one of <app> or --all"))
	}

	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.RotateDEKResult, error) {
		return client.RotateDEK(ctx, app, p)
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rotate data key: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printRotateDEKHuman(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if res.FailedCount > 0 || !res.Complete {
		return exitCheckFailed
	}
	return exitOK
}

func runSecretsKeyPolicy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "secrets key-policy", "print the status as JSON to stdout and nothing else", stderr)
	maxAge := fs.Int("max-age-days", -1, "remind when a key is older than this many days (0 turns reminders off)")
	every := fs.Int("remind-every-days", 0, "repeat the reminder every this many days")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, secretsKeysUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	var upd apiclient.KeyPolicyUpdate
	if *maxAge >= 0 {
		upd.MaxKeyAgeDays = maxAge
	}
	if *every > 0 {
		upd.RemindEveryDays = every
	}
	if upd.MaxKeyAgeDays == nil && upd.RemindEveryDays == nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("set --max-age-days and/or --remind-every-days"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	status, err := client.UpdateKeyPolicy(context.Background(), upd)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("update key policy: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, status, func() { printKeyStatusHuman(stdout, status) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printKeyStatusHuman(out io.Writer, s apiclient.EncryptionKeysStatus) {
	if s.MasterKey.AgeDays != nil {
		_, _ = fmt.Fprintf(out, "master key:  %d days old\n", *s.MasterKey.AgeDays)
	} else {
		_, _ = fmt.Fprintln(out, "master key:  never rotated")
	}
	if s.Policy.MaxKeyAgeDays > 0 {
		_, _ = fmt.Fprintf(out, "policy:      remind after %d days, every %d days\n", s.Policy.MaxKeyAgeDays, s.Policy.RemindEveryDays)
	} else {
		_, _ = fmt.Fprintln(out, "policy:      off")
	}
	for _, a := range s.Apps {
		flagged := ""
		if a.Overdue {
			flagged = "  OVERDUE"
		}
		_, _ = fmt.Fprintf(out, "  %-28s %4d days  %d values%s\n", a.App, a.AgeDays, a.ValueCount, flagged)
	}
	if r := s.LastRun; r != nil {
		state := "finished"
		if r.FinishedAt == nil {
			state = "interrupted, rerun rotate-dek --all to resume"
		}
		_, _ = fmt.Fprintf(out, "last run:    %d/%d apps, %d failed, %s\n", r.Done, r.Total, r.Failed, state)
	}
	for _, h := range s.Hints {
		_, _ = fmt.Fprintf(out, "hint:        %s\n", h.Message)
	}
}

func printRotateDEKHuman(out io.Writer, r apiclient.RotateDEKResult) {
	if r.App != "" {
		_, _ = fmt.Fprintf(out, "rotated %s: %d values re-encrypted at %s\n", r.App, r.Values, r.RotatedAt.Local().Format(time.RFC3339))
		return
	}
	_, _ = fmt.Fprintf(out, "rotated: %d, already current: %d, failed: %d (of %d)\n", r.Rotated, r.Skipped, r.FailedCount, r.Total)
	if r.Resumed {
		_, _ = fmt.Fprintln(out, "resumed an interrupted run")
	}
	if !r.Complete {
		_, _ = fmt.Fprintln(out, "the run was cut short, rerun to resume")
	}
	for _, f := range r.Failed {
		_, _ = fmt.Fprintf(out, "  %s: %s\n", f.App, f.Reason)
	}
}
