package main

import (
	"fmt"
	"io"
)

// Channels are global, not per-app; apps reference them by ID.
func runChannels(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, channelsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, channelsUsage(prog))
		return exitOK
	case "list":
		return runChannelsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runChannelsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "update":
		return runChannelsUpdate(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runChannelsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "test":
		return runChannelsTest(prog, args[1:], stdout, stderr, lookupEnv)
	case "deliveries":
		return runChannelsDeliveries(prog, args[1:], stdout, stderr, lookupEnv)
	case "quiet-hours":
		return runChannelsQuietHours(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown channels subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, channelsUsage(prog))
		return exitUsage
	}
}

func channelsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s channels list [flags]                                           list connected notification channels
  %[1]s channels create --name NAME --kind KIND [flags]              connect a new notification channel
  %[1]s channels update <id> --name NAME --kind KIND [flags]         fully replace a channel's configuration
  %[1]s channels delete <id> [flags]                                    disconnect a channel
  %[1]s channels test <id> [flags]                                       send a real test message to a connected channel
  %[1]s channels deliveries <id> [flags]                                lists a channel's recorded send history, newest first
  %[1]s channels quiet-hours <id> [--start HH:MM --end HH:MM | --clear]  hold non-critical alerts during a daily window

Valid --kind values: generic, slack, discord, telegram, email, pushover, pagerduty, teams, resend, ntfy, gotify, mattermost, lark, rocketchat, opsgenie, webex, googlechat.

Run "%[1]s channels <subcommand> -h" for a subcommand's own flags.
`, prog)
}
