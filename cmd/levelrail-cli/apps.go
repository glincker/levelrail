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
	case "deploy-compose":
		return runAppsDeployCompose(prog, args[1:], stdout, stderr, lookupEnv)
	case "deploy-spec":
		return runAppsDeploySpec(prog, args[1:], stdout, stderr, lookupEnv)
	case "group":
		return runAppsGroup(prog, args[1:], stdout, stderr, lookupEnv)
	case "rollback":
		return runAppsRollback(prog, args[1:], stdout, stderr, lookupEnv)
	case "restart":
		return runAppsRestart(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		return runAppsStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "network":
		return runAppsNetwork(prog, args[1:], stdout, stderr, lookupEnv)
	case "logs":
		return runAppsLogs(prog, args[1:], stdout, stderr, lookupEnv)
	case "exec":
		return runAppsExec(prog, args[1:], stdout, stderr, lookupEnv)
	case "log-drain":
		return runAppsLogDrain(prog, args[1:], stdout, stderr, lookupEnv)
	case "scheduled-tasks":
		return runAppsScheduledTasks(prog, args[1:], stdout, stderr, lookupEnv)
	case "organizations":
		return runAppsOrganizations(prog, args[1:], stdout, stderr, lookupEnv)
	case "environments":
		return runAppsEnvironments(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // args is non-empty here: the len(args)==0 guard above already returned, same as every other case in this switch
	case "set-environment":
		return runAppsSetEnvironment(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "clear-environment":
		return runAppsClearEnvironment(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps subcommand %q\n\n", prog, args[0]) //nolint:gosec // same guard as above
		_, _ = fmt.Fprint(stderr, appsUsage(prog))
		return exitUsage
	}
}

func appsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps create [flags]         create an app (existing image, git build, or --file)
  %[1]s apps list [flags]             list apps
  %[1]s apps get <name> [flags]       show one app
  %[1]s apps deploy <name> [flags]   deploy an image to an existing app
  %[1]s apps deploy-compose <name> --file compose.yaml [flags]   deploy a Docker Compose file as an app
  %[1]s apps deploy-spec <name> --file app.yaml --repo-url <url> --ref <ref> [flags]   fan an app.yaml's services: map out into N independent builds under one app
  %[1]s apps group <name> [flags]   show name's sibling services under the same multi-service app
  %[1]s apps rollback <name> [flags]   redeploy an older image (same endpoint as deploy)
  %[1]s apps restart <name> [flags]     recreate the running container, no image change
  %[1]s apps status <name> [flags]   show an app's current reconcile conditions
  %[1]s apps network <name> [flags]   show the live traffic path: container port, host port, running
  %[1]s apps logs <name> [flags]     search an app's stored log entries
  %[1]s apps exec <name> -- <cmd> [args...]   run a command in the app's container, exits with its real exit code
  %[1]s apps log-drain get|set|clear <name> [flags]   configure an external log drain
  %[1]s apps scheduled-tasks <verb> [flags]   manage cron-scheduled commands run inside the app's container
  %[1]s apps organizations <verb> [flags]   manage organizations, which group projects
  %[1]s apps environments <verb> [flags]   manage a project's environments (staging, production, ...)
  %[1]s apps set-environment <name> <environment-id> [flags]   tag an app with an environment
  %[1]s apps clear-environment <name> [flags]   remove an app's environment tag

Run "%[1]s apps <subcommand> -h" for a subcommand's own flags.
`, prog)
}
