package main

import (
	"context"
	"fmt"
	"io"
)

// runNetworkSharesCreate implements "network-shares create": POST
// /api/v1/network-shares. --username/--password are required for a
// cifs share, ignored for nfs (an NFS export authenticates by source
// IP/export rules on the server side, not by credential).
func runNetworkSharesCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "network-shares create", "print the created network share as JSON to stdout and nothing else", stderr)
	var name, protocol, host, remotePath, mountOptions, username, password string
	fs.StringVar(&name, "name", "", "display name for the network share (required)")
	fs.StringVar(&protocol, "protocol", "", "nfs or cifs (required)")
	fs.StringVar(&host, "host", "", "share host, e.g. nas.internal (required)")
	fs.StringVar(&remotePath, "remote-path", "", "remote export/share path, e.g. /exports/data (required)")
	fs.StringVar(&mountOptions, "mount-options", "", "optional extra mount options")
	fs.StringVar(&username, "username", "", "username (required for a cifs share)")
	fs.StringVar(&password, "password", "", "password (required for a cifs share)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, networkSharesCreateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	if protocol == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--protocol is required (nfs or cifs)"))
	}
	if host == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--host is required"))
	}
	if remotePath == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--remote-path is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	created, err := client.CreateNetworkShare(context.Background(), createNetworkShareRequest{
		Name: name, Protocol: protocol, Host: host, RemotePath: remotePath,
		MountOptions: mountOptions, Username: username, Password: password,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create network share %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "network share %q (id %s, host %s) connected\n", created.Name, created.ID, created.Host)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func networkSharesCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s network-shares create --name NAME --protocol nfs|cifs --host HOST --remote-path PATH [flags]

Connects a new network share.

Flags:
  --name string             display name for the network share (required)
  --protocol string         nfs or cifs (required)
  --host string             share host, e.g. nas.internal (required)
  --remote-path string      remote export/share path, e.g. /exports/data (required)
  --mount-options string    optional extra mount options
  --username string         username, required for a cifs share
  --password string         password, required for a cifs share
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the created network share as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
