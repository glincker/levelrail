package main

import (
	"fmt"
	"io"
	"os"
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
		return runAppsCreate(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
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
	case "stop":
		return runAppsStop(prog, args[1:], stdout, stderr, lookupEnv)
	case "start":
		return runAppsStart(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runAppsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		return runAppsStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "network":
		return runAppsNetwork(prog, args[1:], stdout, stderr, lookupEnv)
	case "logs":
		return runAppsLogs(prog, args[1:], stdout, stderr, lookupEnv)
	case "exec":
		return runAppsExec(prog, args[1:], stdout, stderr, lookupEnv)
	case "log-drain":
		return runAppsLogDrain(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "scheduled-tasks":
		return runAppsScheduledTasks(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "organizations":
		return runAppsOrganizations(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "projects":
		return runAppsProjects(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "environments":
		return runAppsEnvironments(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // args is non-empty here: the len(args)==0 guard above already returned, same as every other case in this switch
	case "set-environment":
		return runAppsSetEnvironment(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "clear-environment":
		return runAppsClearEnvironment(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "set-project":
		return runAppsSetProject(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "clear-project":
		return runAppsClearProject(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "previews":
		return runAppsPreviews(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "secrets":
		return runAppsSecrets(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "git-source":
		return runAppsGitSource(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps subcommand %q\n\n", prog, args[0]) //nolint:gosec // same guard as above
		_, _ = fmt.Fprint(stderr, appsUsage(prog))
		return exitUsage
	}
}

func appsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps create [flags]         create an app (existing image, git build, --file, or --interactive)
  %[1]s apps list [flags]             list apps
  %[1]s apps get <name> [flags]       show one app
  %[1]s apps deploy <name> [flags]   deploy an image to an existing app
  %[1]s apps deploy-compose <name> --file compose.yaml [flags]   deploy a Docker Compose file as an app
  %[1]s apps deploy-spec <name> --file app.yaml --repo-url <url> --ref <ref> [flags]   fan an app.yaml's services: map out into N independent builds under one app
  %[1]s apps group <name> [flags]   show name's sibling services under the same multi-service app
  %[1]s apps rollback <name> [flags]   redeploy an older image (same endpoint as deploy)
  %[1]s apps restart <name> [flags]     recreate the running container, no image change
  %[1]s apps stop <name> [flags]        stop an app's running container
  %[1]s apps start <name> [flags]       start an app previously stopped
  %[1]s apps delete <name> [flags]      remove an app's desired state
  %[1]s apps status <name> [flags]   show an app's current reconcile conditions
  %[1]s apps network <name> [flags]   show the live traffic path: container port, host port, running
  %[1]s apps logs <name> [flags]     search an app's stored log entries
  %[1]s apps exec <name> -- <cmd> [args...]   run a command in the app's container, exits with its real exit code
  %[1]s apps log-drain get|set|clear <name> [flags]   configure an external log drain
  %[1]s apps scheduled-tasks <verb> [flags]   manage cron-scheduled commands run inside the app's container
  %[1]s apps organizations <verb> [flags]   manage organizations, which group projects
  %[1]s apps projects <verb> [flags]   manage projects, which group apps and databases
  %[1]s apps environments <verb> [flags]   manage a project's environments (staging, production, ...)
  %[1]s apps set-environment <name> <environment-id> [flags]   tag an app with an environment
  %[1]s apps clear-environment <name> [flags]   remove an app's environment tag
  %[1]s apps set-project <name> <project-id> [flags]   move an app into a project
  %[1]s apps clear-project <name> [flags]   remove an app's project assignment
  %[1]s apps previews <verb> [flags]   manage preview environments per pull request
  %[1]s apps secrets <verb> [flags]   manage an app's encrypted secret values
  %[1]s apps git-source <verb> [flags]   connect a repo for auto-deploy-on-push

Run "%[1]s apps <subcommand> -h" for a subcommand's own flags.
`, prog)
}
