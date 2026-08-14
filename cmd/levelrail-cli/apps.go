package main

import (
	"fmt"
	"io"
)

// runApps dispatches "apps <verb> [flags]" to one of create/list/get.
func runApps(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsUsage(prog))
		return exitOK
	case "create":
		return runAppsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runAppsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runAppsGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "deploy":
		return runAppsDeploy(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		return runAppsStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "logs":
		return runAppsLogs(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsUsage(prog))
		return exitUsage
	}
}

func appsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps create [flags]         create an app (existing image, git build, or --file)
  %[1]s apps list [flags]             list apps
  %[1]s apps get <name> [flags]       show one app
  %[1]s apps deploy <name> [flags]   deploy an image to an existing app (also how rollback works)
  %[1]s apps status <name> [flags]   show an app's current reconcile conditions
  %[1]s apps logs <name> [flags]     search an app's stored log entries

Run "%[1]s apps <subcommand> -h" for a subcommand's own flags.
`, prog)
}
