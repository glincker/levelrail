package main

import (
	"context"
	"fmt"
	"io"
)

// runSettingsUpdates dispatches "settings updates <verb> [flags]" to one
// of get/set: the release channel and auto-update-check toggle
// (internal/api/updates_settings.go), the CLI counterpart of the
// dashboard's own Settings > Updates page.
func runSettingsUpdates(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsUpdatesUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsUpdatesUsage(prog))
		return exitOK
	case "get":
		return runSettingsUpdatesGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runSettingsUpdatesSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings updates subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsUpdatesUsage(prog))
		return exitUsage
	}
}

func settingsUpdatesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings updates get [flags]
  %[1]s settings updates set --channel CHANNEL [--auto-update] [flags]

Configures which release channel (stable, beta, or edge) GET
/api/v1/updates compares against, and whether the control plane checks
for it in the background. This never applies an update by itself.

Run "%[1]s settings updates <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runSettingsUpdatesGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[updateSettingsResource]{
		cmdLabel:  "settings updates get",
		jsonUsage: "print the update settings as JSON to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s settings updates get [flags]\n\nShows the current release channel and auto-update-check setting.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) (updateSettingsResource, error) {
			return c.GetUpdateSettings(ctx)
		},
		errVerb: "get update settings",
		print:   printUpdateSettingsHuman,
	})
}

func runSettingsUpdatesSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings updates set", "print the updated update settings as JSON to stdout and nothing else", stderr)
	var channel string
	var autoUpdateEnabled bool
	fs.StringVar(&channel, "channel", "stable", "release channel: stable, beta, or edge")
	fs.BoolVar(&autoUpdateEnabled, "auto-update", false, "check for updates on this channel in the background")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings updates set [flags]\n\nConfigures the release channel and auto-update-check setting.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.SetUpdateSettings(context.Background(), updateSettingsResource{
		Channel:           channel,
		AutoUpdateEnabled: autoUpdateEnabled,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set update settings: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, settings, func() { printUpdateSettingsHuman(stdout, settings) })
}

func printUpdateSettingsHuman(out io.Writer, s updateSettingsResource) {
	_, _ = fmt.Fprintf(out, "channel:             %s\n", s.Channel)
	_, _ = fmt.Fprintf(out, "auto_update_enabled: %v\n", s.AutoUpdateEnabled)
}
