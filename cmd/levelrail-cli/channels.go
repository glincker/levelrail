package main

import (
	"fmt"
	"io"
)

// runChannels dispatches "channels <verb> [flags]" to one of
// list/create/delete/test: managing the global, connect-once
// notification channels internal/api/notification_channels.go exposes
// (Settings -> Notification channels in the web UI), the same resource
// apps attach to for deploy-outcome notifications by channel ID.
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
	case "delete":
		return runChannelsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "test":
		return runChannelsTest(prog, args[1:], stdout, stderr, lookupEnv)
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
  %[1]s channels delete <id> [flags]                                    disconnect a channel
  %[1]s channels test <id> [flags]                                       send a real test message to a connected channel

Valid --kind values: generic, slack, discord, telegram, email, pushover, pagerduty, teams.

Run "%[1]s channels <subcommand> -h" for a subcommand's own flags.
`, prog)
}
