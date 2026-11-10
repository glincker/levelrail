package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const sshKeyUsageFmt = `Usage:
  %[1]s apps git-source ssh-key show <name> [flags]                                       show the public key and pinned host fingerprint
  %[1]s apps git-source ssh-key generate <name> (--trust-on-first-use | --host-key FILE|-) [flags]
                                                                                       mint a deploy key and pin the host key (also rotates)
  %[1]s apps git-source ssh-key rotate <name> (--trust-on-first-use | --host-key FILE|-) [flags]
  %[1]s apps git-source ssh-key delete <name> [flags]                                     remove the key and the pinned host key

The app's git source must already use an ssh repo URL. The private key is
generated and kept on the control plane and is never shown. Host keys are
always pinned: pass the server's public key (ssh-keyscan output, verified out
of band) with --host-key, or --trust-on-first-use to scan once and compare the
printed fingerprint with the host's published one.
`

func runAppsGitSourceSSHKey(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, sshKeyUsageFmt, prog)
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprintf(stdout, sshKeyUsageFmt, prog)
		return exitOK
	case "show", "get":
		return runSSHKeyShow(prog, args[1:], stdout, stderr, lookupEnv)
	case "generate", "rotate":
		return runSSHKeySet(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runSSHKeyDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps git-source ssh-key subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprintf(stderr, sshKeyUsageFmt, prog)
		return exitUsage
	}
}

func printSSHKeyHuman(out io.Writer, k apiclient.GitSSHKeyResource) {
	_, _ = fmt.Fprintf(out, "public_key:       %s\n", k.PublicKey)
	_, _ = fmt.Fprintf(out, "fingerprint:      %s\n", k.Fingerprint)
	if k.HostKeyPinned {
		_, _ = fmt.Fprintf(out, "host_fingerprint: %s (pinned)\n", k.HostFingerprint)
	} else {
		_, _ = fmt.Fprintln(out, "host_fingerprint: not pinned")
	}
}

func runSSHKeyShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source ssh-key show", "print the key as JSON to stdout and nothing else", stderr)
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source ssh-key show", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	key, err := client.GetGitSSHKey(context.Background(), name)
	if err != nil {
		code := reportError(stdout, stderr, jsonOut, fmt.Errorf("get ssh deploy key for app %q: %w", name, err))
		if !jsonOut && isAPINotFound(err) {
			_, _ = fmt.Fprintf(stderr, "\nCreate one with: %s apps git-source ssh-key generate %s --trust-on-first-use\n", prog, name)
		}
		return code
	}
	if err := renderResult(stdout, of.Format, of.Query, key, func() { printSSHKeyHuman(stdout, key) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runSSHKeySet(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps git-source ssh-key " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the key as JSON to stdout and nothing else", stderr)
	var hostKeyPath string
	var tofu bool
	fs.StringVar(&hostKeyPath, "host-key", "", "file with the server's public host key in authorized_keys format (or - for stdin)")
	fs.BoolVar(&tofu, "trust-on-first-use", false, "scan the host once and pin what it presents; verify the printed fingerprint")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, sshKeyUsageFmt+"\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, label, "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if (hostKeyPath == "") == !tofu {
		return reportError(stdout, stderr, jsonOut, newValidationError("pass exactly one of --host-key FILE|- or --trust-on-first-use: host keys are always pinned"))
	}
	req := apiclient.SetGitSSHKeyRequest{TrustOnFirstUse: tofu}
	if hostKeyPath != "" {
		hk, err := readSSHKey(hostKeyPath, os.Stdin)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("%s", strings.Replace(err.Error(), "ssh key", "host key", 1)))
		}
		req.HostKey = hk
	}
	key, err := client.SetGitSSHKey(context.Background(), name, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set ssh deploy key for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, key, func() {
		printSSHKeyHuman(stdout, key)
		_, _ = fmt.Fprintln(stdout, "\nAdd the public key to the repository's deploy keys (read-only is enough), and check the host fingerprint against the host's published one.")
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runSSHKeyDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source ssh-key delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source ssh-key delete", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if err := client.DeleteGitSSHKey(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete ssh deploy key for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "ssh deploy key removed for app %q\n", name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
