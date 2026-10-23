package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsSecretsRotationUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps secrets rotation list <name> [flags]                      list an app's rotation policies and their status
  %[1]s apps secrets rotation due [flags]                              list every policy across apps (root)
  %[1]s apps secrets rotation set <name> <key> --interval-days N [--grace-hours N] [--owner TEXT] [--auto] [flags]
  %[1]s apps secrets rotation remove <name> <key> [flags]              remove a policy
  %[1]s apps secrets rotation rotate <name> <key> --yes [flags]        rotate a generated secret now and restart the app

  %[1]s apps secrets rotation replace <name> <key> [--value-file PATH|-] --yes [flags]   write a new value (generated without --value-file), keep the old one for the grace window, no restart

Only secrets marked --auto are ever given a generated value. Values are never printed.
`, prog)
}

func runAppsSecretsRotation(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsSecretsRotationUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsSecretsRotationUsage(prog))
		return exitOK
	case "list":
		return runRotationList(prog, args[1:], stdout, stderr, lookupEnv, false)
	case "due":
		return runRotationList(prog, args[1:], stdout, stderr, lookupEnv, true)
	case "set":
		return runRotationSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "remove":
		return runRotationRemove(prog, args[1:], stdout, stderr, lookupEnv)
	case "rotate":
		return runRotationRotate(prog, args[1:], stdout, stderr, lookupEnv)
	case "replace":
		return runRotationReplace(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps secrets rotation subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsSecretsRotationUsage(prog))
		return exitUsage
	}
}

func runRotationList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), all bool) int {
	verb := "apps secrets rotation list"
	if all {
		verb = "apps secrets rotation due"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, verb, "print the policies as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s [<name>] [flags]\n\nLists rotation policies with status and next due date. Never a value.\n\nFlags:\n", prog, verb)
		fs.PrintDefaults()
	}
	ptrs := apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}
	var (
		client   *apiclient.Client
		name     string
		jsonOut  bool
		of       outputFlags
		exitCode int
		ok       bool
	)
	if all {
		tokenFlag, apiURLFlag, profileFlag, j, o, code, good := parseAPIFlags(fs, args, ptrs, prog, stderr)
		if !good {
			return code
		}
		client = apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
		jsonOut, of = j, o
	} else {
		client, name, jsonOut, of, exitCode, ok = parseSingleArgClient(fs, args, ptrs, stderr, singleArgCmd{prog, verb, "app name"}, lookupEnv)
		if !ok {
			return exitCode
		}
	}
	var policies []apiclient.SecretRotationPolicyResource
	var err error
	if all {
		policies, err = client.ListAllSecretRotation(context.Background())
	} else {
		policies, err = client.ListSecretRotation(context.Background(), name)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list rotation policies: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, policies, func() { printRotationHuman(stdout, policies) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printRotationHuman(out io.Writer, policies []apiclient.SecretRotationPolicyResource) {
	if len(policies) == 0 {
		_, _ = fmt.Fprintln(out, "no rotation policies")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "APP\tKEY\tEVERY\tAUTO\tSTATUS\tSTATE\tNEXT DUE\tOWNER")
	for _, p := range policies {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%dd\t%t\t%s\t%s\t%s\t%s\n", p.App, p.Key, p.IntervalDays, p.AutoRotate, p.Status, p.State, p.NextDueAt, p.Owner)
	}
	_ = tw.Flush()
}

func runRotationSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, _, _, _ := apiFlagSet(prog, "apps secrets rotation set", "unused for this subcommand", stderr)
	var req apiclient.SetSecretRotationPolicyRequest
	fs.IntVar(&req.IntervalDays, "interval-days", 0, "rotate every N days (required)")
	fs.IntVar(&req.GraceHours, "grace-hours", 0, "keep the previous value under <KEY>_PREVIOUS for N hours")
	fs.StringVar(&req.Owner, "owner", "", "who owns this rotation (default: you)")
	fs.BoolVar(&req.AutoRotate, "auto", false, "let the control plane generate and apply new values on schedule")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps secrets rotation set <name> <key> --interval-days N [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps secrets rotation set", "an app name and a key", 2)
	if !ok {
		return exitUsage
	}
	if req.IntervalDays <= 0 {
		_, _ = fmt.Fprintln(stderr, "--interval-days is required and must be positive")
		return exitUsage
	}
	client := apiClientFromFlags(prog, *apiURLFlagP, *tokenFlagP, *profileFlagP, lookupEnv)
	p, err := client.SetSecretRotation(context.Background(), rest[0], rest[1], req)
	if err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("set rotation policy for %q on app %q: %w", rest[1], rest[0], err))
	}
	_, _ = fmt.Fprintf(stdout, "rotation policy for %q on app %q saved, next due %s\n", p.Key, rest[0], p.NextDueAt)
	return exitOK
}

func runRotationRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, _, _, _ := apiFlagSet(prog, "apps secrets rotation remove", "unused for this subcommand", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps secrets rotation remove <name> <key> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps secrets rotation remove", "an app name and a key", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, *apiURLFlagP, *tokenFlagP, *profileFlagP, lookupEnv)
	if err := client.DeleteSecretRotation(context.Background(), rest[0], rest[1]); err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("remove rotation policy for %q on app %q: %w", rest[1], rest[0], err))
	}
	_, _ = fmt.Fprintf(stdout, "rotation policy for %q on app %q removed\n", rest[1], rest[0])
	return exitOK
}

func runRotationRotate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, _, _, _ := apiFlagSet(prog, "apps secrets rotation rotate", "unused for this subcommand", stderr)
	var yes bool
	fs.BoolVar(&yes, "yes", false, "confirm: replace the value now and restart the app")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps secrets rotation rotate <name> <key> --yes [flags]\n\nReplaces the value with a generated one and restarts the app. A failed\nrestart leaves the policy in restart_failed, rerun to retry only the restart.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps secrets rotation rotate", "an app name and a key", 2)
	if !ok {
		return exitUsage
	}
	if !yes {
		_, _ = fmt.Fprintln(stderr, "rotating replaces the stored value and restarts the app, pass --yes to confirm")
		return exitUsage
	}
	client := apiClientFromFlags(prog, *apiURLFlagP, *tokenFlagP, *profileFlagP, lookupEnv)
	p, err := client.RotateSecretNow(context.Background(), rest[0], rest[1])
	if err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("rotate %q on app %q: %w", rest[1], rest[0], err))
	}
	_, _ = fmt.Fprintf(stdout, "secret %q on app %q rotated, next due %s\n", p.Key, rest[0], p.NextDueAt)
	return exitOK
}
