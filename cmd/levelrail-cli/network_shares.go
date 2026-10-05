package main

import (
	"fmt"
	"io"
)

// runNetworkShares dispatches "network-shares <verb> [flags]" to one of
// list/get/create/update/delete/test: managing the NFS/CIFS mounts
// internal/api/network_shares.go exposes (Settings -> Network shares in
// the web UI), the same shares an app's own volume mount can later
// reference by name.
func runNetworkShares(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, networkSharesUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, networkSharesUsage(prog))
		return exitOK
	case "list":
		return runNetworkSharesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runNetworkSharesGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runNetworkSharesCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "update":
		return runNetworkSharesUpdate(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runNetworkSharesDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "test":
		return runNetworkSharesTest(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown network-shares subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, networkSharesUsage(prog))
		return exitUsage
	}
}

func networkSharesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s network-shares list [flags]                                                                         list connected network shares
  %[1]s network-shares get <id> [flags]                                                                      show one network share
  %[1]s network-shares create --name NAME --protocol nfs|cifs --host HOST --remote-path PATH [flags]         connect a new network share
  %[1]s network-shares update <id> --name NAME --protocol nfs|cifs --host HOST --remote-path PATH [flags]    update a network share, optionally rotating its password
  %[1]s network-shares delete <id> [flags]                                                                   disconnect a network share
  %[1]s network-shares test <id> [flags]                                                                     check a share's host is reachable on its protocol's standard port

Run "%[1]s network-shares <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func printNetworkShareHuman(out io.Writer, s networkShareResource) {
	_, _ = fmt.Fprintf(out, "id:             %s\n", s.ID)
	_, _ = fmt.Fprintf(out, "name:           %s\n", s.Name)
	_, _ = fmt.Fprintf(out, "protocol:       %s\n", s.Protocol)
	_, _ = fmt.Fprintf(out, "host:           %s\n", s.Host)
	_, _ = fmt.Fprintf(out, "remote path:    %s\n", s.RemotePath)
	if s.MountOptions != "" {
		_, _ = fmt.Fprintf(out, "mount options:  %s\n", s.MountOptions)
	}
	if s.Username != "" {
		_, _ = fmt.Fprintf(out, "username:       %s\n", s.Username)
	}
	_, _ = fmt.Fprintf(out, "created:        %s\n", s.CreatedAt)
}
