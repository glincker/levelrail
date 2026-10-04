package main

import (
	"context"
	"fmt"
	"io"
)

// runNetworkSharesUpdate implements "network-shares update <id>": PUT
// /api/v1/network-shares/{id}, a full replace of
// name/protocol/host/remote-path/mount-options/username. --password is
// optional here, unlike "network-shares create": omitted, the share
// keeps its existing stored password; given, it rotates it.
func runNetworkSharesUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "network-shares update", "print the updated network share as JSON to stdout and nothing else", stderr)
	var name, protocol, host, remotePath, mountOptions, username, password string
	fs.StringVar(&name, "name", "", "display name for the network share (required)")
	fs.StringVar(&protocol, "protocol", "", "nfs or cifs (required)")
	fs.StringVar(&host, "host", "", "share host, e.g. nas.internal (required)")
	fs.StringVar(&remotePath, "remote-path", "", "remote export/share path, e.g. /exports/data (required)")
	fs.StringVar(&mountOptions, "mount-options", "", "optional extra mount options")
	fs.StringVar(&username, "username", "", "username (required for a cifs share)")
	fs.StringVar(&password, "password", "", "new password; omit to keep the existing one")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, networkSharesUpdateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "network-shares update", "network share id")
	if !ok {
		return exitUsage
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

	updated, err := client.UpdateNetworkShare(context.Background(), id, updateNetworkShareRequest{
		Name: name, Protocol: protocol, Host: host, RemotePath: remotePath,
		MountOptions: mountOptions, Username: username, Password: password,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("update network share %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, updated, func() {
		_, _ = fmt.Fprintf(stdout, "network share %q (id %s) updated\n", updated.Name, updated.ID)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func networkSharesUpdateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s network-shares update <id> --name NAME --protocol nfs|cifs --host HOST --remote-path PATH [flags]

Updates a network share's name/protocol/host/remote path/mount options/
username. Add --password to rotate its stored password in the same
call; omit it to leave the password unchanged.

Flags:
  --name string             display name for the network share (required)
  --protocol string         nfs or cifs (required)
  --host string             share host, e.g. nas.internal (required)
  --remote-path string      remote export/share path, e.g. /exports/data (required)
  --mount-options string    optional extra mount options
  --username string         username, required for a cifs share
  --password string         new password, omit to keep the existing one
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the updated network share as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
