package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func instanceUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s instance show [flags]

Shows this server's Instance ID, its fingerprint, the Backup set ID, the server version
and whether this profile is pinned to the server. The Instance ID is an identifier, not a
secret. It is never sent in the update check or the announcement fetch.

Flags:
  --api-url string   control plane base URL (default: %[2]s env var, then %[3]s)
  --token string     API token (default: saved login or the token env var)
  --profile string   named credentials profile (overrides APP_PROFILE, default "default")
  --json             print the result as JSON to stdout, nothing else
  --output string    output format: json, ndjson, yaml, table, wide, text or name
  --query string     JMESPath expression to filter the result before printing
  -h, --help         show this help
`, prog, apiURLEnvHelp, defaultAPIURL)
}

type instanceShowResult struct {
	InstanceID  string `json:"instance_id"`
	Fingerprint string `json:"fingerprint"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	BackupSetID string `json:"backup_set_id,omitempty"`
	Pinned      bool   `json:"pinned"`
	Moved       bool   `json:"moved"`
	MovedAt     string `json:"moved_at,omitempty"`
	PreviousID  string `json:"previous_instance_id,omitempty"`
}

// pinnedTo reports whether the profile's saved pin names this server.
func pinnedTo(prog, profile string, id apiclient.InstanceIdentity) bool {
	c, err := readCredentialsFile(prog, profile)
	return err == nil && c.InstanceID != "" && c.InstanceID == id.InstanceID && c.Fingerprint == id.Fingerprint
}

func printInstanceShow(w io.Writer, r instanceShowResult) {
	_, _ = fmt.Fprintf(w, "instance id:    %s\nfingerprint:    %s\nserver:         %s %s\n", r.InstanceID, r.Fingerprint, r.Name, r.Version)
	if r.BackupSetID != "" {
		_, _ = fmt.Fprintf(w, "backup set id:  %s\n", r.BackupSetID)
	}
	_, _ = fmt.Fprintf(w, "pinned:         %t\n", r.Pinned)
	if r.Moved {
		_, _ = fmt.Fprintf(w, "moved:          yes, at %s (previous %s); a root operator confirms \"same instance\" in Settings, CLI access\n", r.MovedAt, apiclient.ShortInstanceID(r.PreviousID))
	}
}

func runInstance(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, instanceUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, instanceUsage(prog))
		return exitOK
	case "show":
		return runInstanceShow(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown instance subcommand %q\n\n%s", prog, args[0], instanceUsage(prog))
	return exitUsage
}

func runInstanceShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "instance show", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, instanceUsage(prog)) }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return code
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	ctx := context.Background()
	id, err := client.GetInstanceIdentity(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("instance show: %w", err))
	}
	res := instanceShowResult{
		InstanceID: id.InstanceID, Fingerprint: id.Fingerprint, Name: id.Name, Version: id.Version,
		Pinned: pinnedTo(prog, resolveProfile(profile, lookupEnv), id), Moved: id.Moved, MovedAt: id.MovedAt, PreviousID: id.PreviousInstanceID,
	}
	if dr, derr := client.GetControlPlaneDR(ctx); derr == nil {
		res.BackupSetID = dr.InstallID
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printInstanceShow(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
