package main

import (
	"context"
	"fmt"
	"io"
)

// runBackupTargetsScanHostKey prints the key an SFTP server offers; it pins nothing by itself.
func runBackupTargetsScanHostKey(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backup-targets scan-host-key", "print the host key as JSON to stdout and nothing else", stderr)
	var endpoint string
	fs.StringVar(&endpoint, "endpoint", "", "SFTP server as HOST or HOST:PORT (required)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s backup-targets scan-host-key --endpoint HOST[:PORT] [flags]\n\nReads the SSH host key the server offers without sending credentials. Compare the fingerprint with one you trust, then pass the key to create with --host-key.\n", prog)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if endpoint == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--endpoint is required"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	scan, err := client.ScanSFTPHostKey(context.Background(), endpoint)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("scan host key of %q: %w", endpoint, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, scan, func() {
		_, _ = fmt.Fprintf(stdout, "%s\n", scan.HostKey)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
