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
		return runAppsDeploy(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	case "wait":
		return runAppsWait(prog, args[1:], stdout, stderr, lookupEnv)
	case "deploy-compose":
		return runAppsDeployCompose(prog, args[1:], stdout, stderr, lookupEnv)
	case "deploy-spec":
		return runAppsDeploySpec(prog, args[1:], stdout, stderr, lookupEnv)
	case "group":
		return runAppsGroup(prog, args[1:], stdout, stderr, lookupEnv)
	case "hook-runs":
		return runAppsHookRuns(prog, args[1:], stdout, stderr, lookupEnv)
	case "rollback":
		return runAppsRollback(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	case "freeze":
		return runAppsFreeze(prog, args[1:], stdout, stderr, lookupEnv)
	case "auto-rollback":
		return runAppsAutoRollback(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "cancel-superseded":
		return runAppsCancelSuperseded(prog, args[1:], stdout, stderr, lookupEnv)
	case "deploys":
		return runAppsDeploys(prog, args[1:], stdout, stderr, lookupEnv)
	case "promote":
		return runAppsPromote(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin) //nolint:gosec // same guard as below
	case "timeline":
		return runAppsTimeline(prog, args[1:], stdout, stderr, lookupEnv)
	case "apply":
		return runAppsApply(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "domains":
		return runAppsDomains(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "restart":
		return runAppsRestart(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "stop":
		return runAppsStop(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "start":
		return runAppsStart(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "delete":
		return runAppsDelete(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "status":
		return runAppsStatus(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "diagnose":
		return runAppsDiagnose(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "preflight":
		return runAppsPreflight(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "resource-recommendation":
		return runAppsResourceRecommendation(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "network":
		return runAppsNetwork(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "logs":
		return runAppsLogs(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "metrics":
		return runAppsMetrics(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "requests":
		return runAppsRequests(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "resource-usage":
		return runAppsResourceUsage(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "exec":
		return runAppsExec(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "exec-access":
		return runAppsExecAccess(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "log-drain":
		return runAppsLogDrain(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "scheduled-tasks":
		return runAppsScheduledTasks(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "build-cache":
		return runAppsBuildCache(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "alerts":
		return runAppsAlerts(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
	case "deploy-notify-targets":
		return runAppsDeployNotifyTargets(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as below
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
	case "set-node":
		return runAppsSetNode(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "clear-node":
		return runAppsClearNode(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "previews":
		return runAppsPreviews(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "env":
		return runAppsEnv(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "secrets":
		return runAppsSecrets(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "git-source":
		return runAppsGitSource(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "webhook-deliveries":
		return runAppsWebhookDeliveries(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "bulk":
		return runAppsBulk(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin) //nolint:gosec // same guard as below
	case "clone":
		return runAppsClone(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "images":
		return runAppsImages(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "storage":
		return runAppsStorage(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "database":
		return runAppsDatabase(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "builds":
		return runAppsBuilds(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "moves":
		return runAppsMoves(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "vault-env":
		return runAppsVaultEnv(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "preview-env":
		return runAppsPreviewEnv(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "branch-env":
		return runAppsBranchEnv(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "tag":
		return runAppsTag(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "untag":
		return runAppsUntag(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "egress":
		return runAppsEgress(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "health":
		return runAppsHealth(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
	case "integrations":
		return runAppsIntegrations(prog, args[1:], stdout, stderr, lookupEnv) //nolint:gosec // same guard as above
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
  %[1]s apps wait <name> [flags]        poll until a deploy attempt actually converges, exit accordingly (a CI gate for "apps deploy")
  %[1]s apps deploy-compose <name> --file compose.yaml [flags]   deploy a Docker Compose file as an app
  %[1]s apps deploy-spec <name> --file app.yaml --repo-url <url> --ref <ref> [flags]   fan an app.yaml's services: map out into N independent builds under one app
  %[1]s apps group <name> [flags]   show name's sibling services under the same multi-service app
  %[1]s apps hook-runs <name> [flags]   show the most recent outcome of name's pre/post-deploy hooks
  %[1]s apps rollback <name> [flags]   redeploy an older image (same endpoint as deploy)
  %[1]s apps freeze set|show|clear <name> [flags]   deploy freeze windows: hold automatic deploys on a cron schedule
  %[1]s apps cancel-superseded enable|disable|status <name> [flags]   let a newer queued deploy replace older queued ones of the same branch
  %[1]s apps auto-rollback enable|disable|status <name> [flags]   opt an app into (or out of) automatic rollback when a crashloop alert fires
  %[1]s apps deploys list <name> [flags]                          real, row-per-attempt deploy history, newest first
  %[1]s apps deploys compare <name> --from ID [--to ID] [flags]   diff two deploy attempts, or one against the current live state
  %[1]s apps promote <name> --to ENVIRONMENT_ID [--target NAME] [--preview] [flags]   promote name's image onto a sibling app in another environment
  %[1]s apps restart <name> [flags]     recreate the running container, no image change
  %[1]s apps timeline <name> [--limit N] [flags]   what happened to an app: deploys, restarts, env, secret and config changes
  %[1]s apps apply <name> [flags]       restart an app so saved env, secret and config changes take effect
  %[1]s apps domains list|add|remove <name> [domain...] [flags]   show or change an app's domains
  %[1]s apps stop <name> [flags]        stop an app's running container
  %[1]s apps start <name> [flags]       start an app previously stopped
  %[1]s apps delete <name> [flags]      remove an app's desired state
  %[1]s apps status <name> [flags]   show an app's current reconcile conditions
  %[1]s apps diagnose <name> [--deploy ID] [--apply-fix N] [flags]   explain a failed deploy or crashloop, optionally apply a fix
  %[1]s apps preflight <name> [--require-env A,B] [flags]   run pre-deploy checks (DNS, ports, disk, image, env)
  %[1]s apps resource-recommendation <name> [flags]   suggest memory/CPU limits from historical usage
  %[1]s apps network <name> [flags]   show the live traffic path: container port, host port, running
  %[1]s apps logs <name> [flags]     search an app's stored log entries, or --follow to stream live
  %[1]s apps metrics <name> --metric NAME [flags]   query an app's metric time series
  %[1]s apps requests <name> [flags]                show request rate, errors and latency from the ingress
  %[1]s apps resource-usage [flags]   rank every app by latest CPU/memory/network usage
  %[1]s apps exec <name> -- <cmd> [args...]   run a command in the app's container, exits with its real exit code
  %[1]s apps exec-access enable|disable|status <name> [flags]   opt an app into (or out of) shell/exec access, on by default
  %[1]s apps log-drain get|set|clear <name> [flags]   configure an external log drain
  %[1]s apps scheduled-tasks <verb> [flags]   manage cron-scheduled commands run inside the app's container
  %[1]s apps build-cache <verb> [flags]   show/set/clear/remove the BuildKit remote cache on a storage destination
  %[1]s apps alerts <verb> [flags]   manage alert rules (threshold, crashloop, cert_expiry)
  %[1]s apps deploy-notify-targets <verb> [flags]   manage which notification channels get a deploy's outcome
  %[1]s apps organizations <verb> [flags]   manage organizations, which group projects
  %[1]s apps projects <verb> [flags]   manage projects, which group apps and databases
  %[1]s apps environments <verb> [flags]   manage a project's environments (staging, production, ...)
  %[1]s apps set-environment <name> <environment-id> [flags]   tag an app with an environment
  %[1]s apps clear-environment <name> [flags]   remove an app's environment tag
  %[1]s apps set-project <name> <project-id> [flags]   move an app into a project
  %[1]s apps clear-project <name> [flags]   remove an app's project assignment
  %[1]s apps set-node <name> <node-id> [--with-volumes] [flags]   move an app to another node, optionally taking its named volumes with it
  %[1]s apps clear-node <name> [--with-volumes] [flags]   move an app back to this control plane's own local node
  %[1]s apps previews <verb> [flags]   manage preview environments per pull request
  %[1]s apps secrets <verb> [flags]   manage an app's encrypted secret values
  %[1]s apps env <verb> [flags]   import a .env file into, or export (secret-free) from, an app's plain env vars
  %[1]s apps git-source <verb> [flags]   connect a repo for auto-deploy-on-push
  %[1]s apps webhook-deliveries <verb> [flags]   inspect and replay recent inbound git webhook requests
  %[1]s apps clone <name> <new-name> [flags]   duplicate an app's desired state under a new name
  %[1]s apps images <name> [flags]   list locally-present image tags under an app's current image repo
  %[1]s apps storage <verb> [flags]   attach/detach a connected bucket as this app's object storage
  %[1]s apps database <verb> [flags]   attach/detach a managed database as this app's connection-env-var source
  %[1]s apps builds trigger <name> --repo-url URL --ref REF [flags]   build and deploy an image from a git source
  %[1]s apps moves <verb> [flags]      inspect "apps set-node --with-volumes" move-with-volumes history
  %[1]s apps vault-env <verb> [flags]   declare/remove an env var resolved live from an external Vault instance
  %[1]s apps preview-env <verb> [flags]   declare/remove a preview-specific env var override, applied only when a preview is created
  %[1]s apps branch-env <verb> [flags]    declare/remove a branch-scoped env var override, applied only when a preview's own branch matches
  %[1]s apps tag <name> <tag> [flags]     attach a tag (by name) to an app, creating it first if new
  %[1]s apps untag <name> <tag> [flags]   detach a tag (by name) from an app
  %[1]s apps egress <verb> [flags]        get/set/clear an app's outbound network allowlist
  %[1]s apps health <verb> [flags]        get/set/clear an app's readiness and liveness probes

Run "%[1]s apps <subcommand> -h" for a subcommand's own flags.
`, prog)
}
