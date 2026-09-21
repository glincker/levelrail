package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsEgress dispatches "apps egress <verb> [flags]" to one of
// get/set/clear, the CLI counterpart of internal/api/apps_egress.go's own
// GET/PUT/DELETE /api/v1/apps/{name}/egress-policy routes: an app's
// outbound network allowlist, enforced by an egress sidecar
// (internal/reconcile/application's reconcileEgress). Unconfigured
// (the default, and the only state every app had before this feature
// existed) means unrestricted egress; opting in is per-app, never
// implicit.
func runAppsEgress(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsEgressUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsEgressUsage(prog))
		return exitOK
	case "get":
		return runAppsEgressGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runAppsEgressSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runAppsEgressClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps egress subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsEgressUsage(prog))
		return exitUsage
	}
}

func appsEgressUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps egress get <name> [flags]                                    show an app's current outbound network allowlist
  %[1]s apps egress set <name> --allow host:port [--allow host:port ...]  restrict an app's outbound traffic to an allowlist
  %[1]s apps egress clear <name> [flags]                                  opt an app back out to unrestricted egress

Unconfigured (the default) means unrestricted egress, unchanged from
today. "set" restricts an app's container to only reach the declared
host:port pairs, enforced by a reconciled sidecar that re-resolves each
host on an interval; DNS and loopback traffic stay open regardless. The
same policy can also be declared in app.yaml's own egress: block.

Run "%[1]s apps egress <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsEgressGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps egress get", "print the current policy as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps egress get <name> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps egress get", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	result, err := client.GetAppEgressPolicy(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get egress policy for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printAppEgressPolicyHuman(stdout, result) })
}

func runAppsEgressSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps egress set", "print the resulting policy as JSON to stdout and nothing else", stderr)
	var allow egressAllowFlag
	fs.Var(&allow, "allow", "a \"host:port\" this app may reach, repeatable (at least one required)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps egress set <name> --allow host:port [--allow host:port ...] [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "apps egress set", "app name")
	if !ok {
		return exitUsage
	}
	if len(allow) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: apps egress set requires at least one --allow host:port\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.SetAppEgressPolicy(context.Background(), name, setAppEgressPolicyRequest{Mode: "allowlist", Allow: allow})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set egress policy for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printAppEgressPolicyHuman(stdout, result) })
}

func runAppsEgressClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps egress clear", "print {\"cleared\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps egress clear <name> [flags]\n\nOpts <name> back out to unrestricted egress, tearing down its egress\nsidecar on the next reconcile pass.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps egress clear", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	if err := client.ClearAppEgressPolicy(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear egress policy for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"cleared": true}, func() {
		_, _ = fmt.Fprintf(stdout, "egress policy cleared for app %q (unrestricted egress)\n", name)
	})
}

func printAppEgressPolicyHuman(out io.Writer, r appEgressPolicyResource) {
	if r.Mode == "" {
		_, _ = fmt.Fprintf(out, "app_name: %s\n", r.AppName)
		_, _ = fmt.Fprintln(out, "egress:   unrestricted (no policy configured)")
		return
	}
	_, _ = fmt.Fprintf(out, "app_name: %s\n", r.AppName)
	_, _ = fmt.Fprintf(out, "mode:     %s\n", r.Mode)
	_, _ = fmt.Fprintln(out, "allow:")
	for _, a := range r.Allow {
		_, _ = fmt.Fprintf(out, "  - %s:%d\n", a.Host, a.Port)
	}
}
