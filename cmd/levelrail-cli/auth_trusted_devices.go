package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// runAuthDevicesGroup routes "auth devices trusted <verb>" and keeps the older
// "auth devices [revoke <id>]" form working for existing scripts.
func runAuthDevicesGroup(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "trusted":
		return runAuthDevicesTrusted(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		return runAuthDevices(prog, args, stdout, stderr, lookupEnv)
	}
}

func runAuthDevicesTrusted(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, authDevicesTrustedUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, authDevicesTrustedUsage(prog))
		return exitOK
	case "list":
		return runAuthTrustedList(prog, args[1:], stdout, stderr, lookupEnv)
	case "revoke":
		return runAuthTrustedRevoke(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown auth devices trusted subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, authDevicesTrustedUsage(prog))
		return exitUsage
	}
}

func authDevicesTrustedUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth devices trusted list [flags]               list browsers trusted for password sign-in
  %[1]s auth devices trusted revoke <id> [--yes] [flags] revoke one; its next sign-in needs approval again

A trusted browser signs in to your account with a password without a new
browser approval. The token must belong to your user.
`, prog)
}

func runAuthTrustedList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth devices trusted list", "print the devices as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, authDevicesTrustedUsage(prog)+"\nFlags:\n")
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if _, ok := requireArgs(fs, stderr, prog, "auth devices trusted list", "no arguments", 0); !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	devices, err := client.ListTrustedDevices(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list trusted devices: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, devices, func() {
		if len(devices) == 0 {
			_, _ = fmt.Fprintln(stdout, "No trusted browsers.")
		}
		for _, d := range devices {
			cur := ""
			if d.Current {
				cur = "  (this browser)"
			}
			_, _ = fmt.Fprintf(stdout, "%s  %s  %s  last used %s  expires %s%s\n", d.ID, d.IP, d.Label,
				d.LastUsedAt.Local().Format(time.RFC822), d.ExpiresAt.Local().Format(time.RFC822), cur)
		}
	})
}

func runAuthTrustedRevoke(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth devices trusted revoke", "print the result as JSON", stderr)
	yes := fs.Bool("yes", false, "revoke without asking; required when stdin is not a terminal")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, authDevicesTrustedUsage(prog)+"\nFlags:\n")
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "auth devices trusted revoke", "exactly one device id", 1)
	if !ok {
		return exitUsage
	}
	if !*yes {
		if err := confirmDestructive(stderr, fmt.Sprintf("Revoke trusted browser %s?", rest[0])); err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
		}
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.RevokeTrustedDevice(context.Background(), rest[0]); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke trusted device %q: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"id": rest[0], "status": "revoked"}, func() {
		_, _ = fmt.Fprintf(stdout, "revoked trusted device %s\n", rest[0])
	})
}

// confirmDestructive asks on the terminal; without one the caller must pass
// --yes, so a pipe or CI job never revokes or deletes by accident.
func confirmDestructive(stderr io.Writer, prompt string) error {
	if f, ok := stdinSource.(*os.File); !ok || ciMode || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // a terminal file descriptor always fits an int
		return errors.New("not a terminal: pass --yes to confirm")
	}
	_, _ = fmt.Fprintf(stderr, "%s [y/N] ", prompt)
	line, _ := bufio.NewReader(stdinSource).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return errors.New("cancelled")
	}
	return nil
}
