package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsExpiryUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps expiry <app-name> [flags]
  %[1]s apps expiry <app-name> --at TIME | --in DURATION [--delete-data --confirm NAME] [flags]
  %[1]s apps expiry <app-name> --clear [flags]

An app with an expiry gets a notice in the audit log, is stopped at the
expiry, and is deleted only after the grace period. Apps with volumes,
linked databases or a protected environment are only stopped unless
--delete-data is set. --clear always cancels a pending delete; a stopped
app stays stopped until "apps start".

Flags:
  --at string             expiry time in RFC 3339, for example 2026-12-01T18:00:00Z
  --in duration           expiry from now, for example 72h
  --delete-data           allow the delete to remove volumes and data
  --confirm string        the app name, required with --delete-data
  --clear                 remove the expiry and cancel a pending delete
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read
  --json                  print the result as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}

// runAppsExpiry implements "apps expiry".
func runAppsExpiry(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps expiry"
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	var at, confirm string
	var in time.Duration
	var clearFlag, deleteData bool
	fs.StringVar(&at, "at", "", "expiry time in RFC 3339")
	fs.DurationVar(&in, "in", 0, "expiry from now")
	fs.BoolVar(&deleteData, "delete-data", false, "allow the delete to remove volumes and data")
	fs.StringVar(&confirm, "confirm", "", "the app name, required with --delete-data")
	fs.BoolVar(&clearFlag, "clear", false, "remove the expiry and cancel a pending delete")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsExpiryUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	at = strings.TrimSpace(at)
	switch {
	case clearFlag && (at != "" || in != 0):
		return reportError(stdout, stderr, jsonOut, newValidationError("%s: --clear cannot be combined with --at or --in", label))
	case at != "" && in != 0:
		return reportError(stdout, stderr, jsonOut, newValidationError("%s: use --at or --in, not both", label))
	case in < 0:
		return reportError(stdout, stderr, jsonOut, newValidationError("%s: --in must be positive", label))
	case deleteData && confirm != name:
		return reportError(stdout, stderr, jsonOut, newValidationError("%s: --delete-data needs --confirm %s", label, name))
	}
	if in > 0 {
		at = time.Now().Add(in).UTC().Format(time.RFC3339)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var res apiclient.AppExpiry
	var err error
	switch {
	case clearFlag:
		res, err = client.ClearAppExpiry(ctx, name)
	case at != "":
		res, err = client.SetAppExpiry(ctx, name, at, deleteData, confirm)
	default:
		res, err = client.GetAppExpiry(ctx, name)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s %q: %w", label, name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		if res.Expiry == nil {
			_, _ = fmt.Fprintf(stdout, "app %q has no expiry\n", name)
			return
		}
		_, _ = fmt.Fprintf(stdout, "app %q expires at %s (%s, delete data: %t)\n", name, res.Expiry.ExpiresAt, res.State, res.Expiry.DeleteData)
		if !res.DeleteEnabled {
			_, _ = fmt.Fprintln(stdout, "delete disabled by operator: the app is only stopped")
		}
		if res.DeleteAfter != "" {
			_, _ = fmt.Fprintf(stdout, "deleted after %s unless cleared\n", res.DeleteAfter)
		}
		if res.Expiry.KeptReason != "" {
			_, _ = fmt.Fprintf(stdout, "kept, not deleted: %s\n", res.Expiry.KeptReason)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
