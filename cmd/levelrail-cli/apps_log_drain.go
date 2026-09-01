package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsLogDrain dispatches "apps log-drain <verb> [flags]" to one of
// get/set/clear, mirroring runApps' own top-level dispatch shape.
func runAppsLogDrain(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsLogDrainUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsLogDrainUsage(prog))
		return exitOK
	case "get":
		return runAppsLogDrainGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runAppsLogDrainSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runAppsLogDrainClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps log-drain subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsLogDrainUsage(prog))
		return exitUsage
	}
}

func appsLogDrainUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps log-drain get <name> [flags]     show an app's configured log drain
  %[1]s apps log-drain set <name> --type TYPE --target TARGET [flags]   configure a log drain
  %[1]s apps log-drain clear <name> [flags]   remove an app's log drain

Forwards an app's container logs to an external HTTP or syslog sink, in
addition to (never instead of) the built-in node-local log store.

Run "%[1]s apps log-drain <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsLogDrainGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps log-drain get", "print the log drain as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps log-drain get <name> [flags]\n\nShows an app's currently configured log drain.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "apps log-drain get", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	drain, err := client.GetLogDrain(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get log drain for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, drain, func() { printLogDrainHuman(stdout, drain) })
}

func runAppsLogDrainSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps log-drain set", "print the log drain as JSON to stdout and nothing else", stderr)
	var typeFlag, targetFlag string
	var disabled bool
	fs.StringVar(&typeFlag, "type", "", `sink type: "http" or "syslog" (required)`)
	fs.StringVar(&targetFlag, "target", "", "sink target: an HTTP URL, or a syslog network://host:port (empty for local syslog)")
	fs.BoolVar(&disabled, "disabled", false, "save the drain but leave forwarding paused (default: enabled)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps log-drain set <name> --type TYPE --target TARGET [flags]\n\nConfigures an app's log drain, additive to the built-in log store.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "apps log-drain set", "app name")
	if !ok {
		return exitUsage
	}
	if typeFlag == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps log-drain set requires --type\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	drain, err := client.SetLogDrain(context.Background(), name, setLogDrainRequest{
		Type:    typeFlag,
		Target:  targetFlag,
		Enabled: !disabled,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set log drain for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, drain, func() { printLogDrainHuman(stdout, drain) })
}

func runAppsLogDrainClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps log-drain clear", "print {\"cleared\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps log-drain clear <name> [flags]\n\nRemoves an app's log drain.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "apps log-drain clear", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.ClearLogDrain(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear log drain for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, map[string]bool{"cleared": true}, func() { _, _ = fmt.Fprintf(stdout, "log drain removed for app %q\n", name) })
}

func printLogDrainHuman(out io.Writer, d logDrainResource) {
	status := "enabled"
	if !d.Enabled {
		status = "disabled"
	}
	_, _ = fmt.Fprintf(out, "app:    %s\ntype:   %s\ntarget: %s\nstatus: %s\n", d.AppName, d.Type, d.Target, status)
}
