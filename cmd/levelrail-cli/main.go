// Command levelrail-cli is a scriptable HTTP client for the control plane's /api/v1 API.
// Every required input has a flag, so CI or an agent with no TTY never hangs on a prompt.
// With --json, stdout is a single JSON value; diagnostics always go to stderr.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func main() {
	prog := filepath.Base(os.Args[0])
	// Piped stdin or stdout implies CI mode; run() only sees env so tests stay deterministic.
	forceCI = !isTerminal(os.Stdin) || !isTerminal(os.Stdout)
	finishHint := startUpdateHint(prog, os.Args[1:], os.Stderr, os.LookupEnv)
	code := run(prog, os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv)
	finishHint()
	os.Exit(code)
}

// run returns an int exit code instead of calling os.Exit, so tests can drive dispatch directly.
func run(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	rawLookupEnv = lookupEnv
	lookupEnv = envAliasLookup(lookupEnv)
	ciMode = forceCI || ciEnv(rawLookupEnv)
	if colorDisabled(rawLookupEnv, ciMode) {
		_ = os.Setenv(envNoColor, "1")
	}
	args, debug := extractDebugFlag(args)
	args, scopeFlags = extractScopeFlags(args)
	installServerTrust(prog, args, stderr, lookupEnv)
	apiclient.ResetServerPins()
	if debug {
		apiclient.SetDebugTrace(stderr)
	} else {
		// Always set, never left as whatever a previous run() call in
		// this process left behind (a test binary calls run() many
		// times, e.g. completion_test.go's dispatch table).
		apiclient.SetDebugTrace(nil)
	}

	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, simpleUsage(prog))
		return exitUsage
	}

	syncServerBetaFeatures(prog, args, lookupEnv)
	if rejectDisabledExperimental(prog, args, stderr) {
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help":
		_, _ = fmt.Fprint(stdout, simpleUsage(prog))
		return exitOK
	case "help":
		if len(args) > 1 && (args[1] == "advanced" || args[1] == "all") {
			_, _ = fmt.Fprint(stdout, rootUsage(prog))
		} else {
			_, _ = fmt.Fprint(stdout, simpleUsage(prog))
		}
		return exitOK
	case "up":
		return runUp(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	case "dev":
		return runDev(prog, args[1:], stdout, stderr, lookupEnv)
	case "open":
		return runOpen(prog, args[1:], stdout, stderr, lookupEnv)
	case "login":
		return runLogin(prog, args[1:], stdout, stderr, lookupEnv)
	case "backup":
		return runBackupAlias(prog, args[1:], stdout, stderr, lookupEnv)
	case "restore":
		return runRestoreAlias(prog, args[1:], stdout, stderr, lookupEnv)
	case "apps":
		return runApps(prog, args[1:], stdout, stderr, lookupEnv)
	case "git":
		return runGit(prog, args[1:], stdout, stderr, lookupEnv)
	case "previews":
		return runAppsPreviews(prog, args[1:], stdout, stderr, lookupEnv)
	case "deploy":
		return runAliasedDeploy(prog, args[1:], stdout, stderr, lookupEnv)
	case "rollback":
		return runAliasedRollback(prog, args[1:], stdout, stderr, lookupEnv)
	case "databases", "db":
		return runDatabases(prog, args[1:], stdout, stderr, lookupEnv)
	case "functions":
		return runFunctions(prog, args[1:], stdout, stderr, lookupEnv)
	case "jobs":
		return runJobs(prog, args[1:], stdout, stderr, lookupEnv)
	case "models":
		return runModels(prog, args[1:], stdout, stderr, lookupEnv)
	case "auth":
		return runAuth(prog, args[1:], stdout, stderr, lookupEnv)
	case "profile":
		return runProfile(prog, args[1:], stdout, stderr, lookupEnv)
	case "tokens":
		return runTokens(prog, args[1:], stdout, stderr, lookupEnv)
	case "cleanup":
		return runCleanup(prog, args[1:], stdout, stderr, lookupEnv)
	case "mobile":
		return runMobile(prog, args[1:], stdout, stderr, lookupEnv)
	case "plugins":
		return runPlugins(prog, args[1:], stdout, stderr, lookupEnv)
	case "dns":
		return runDNS(prog, args[1:], stdout, stderr, lookupEnv)
	case "domains":
		return runDomains(prog, args[1:], stdout, stderr, lookupEnv)
	case "backups":
		return runBackups(prog, args[1:], stdout, stderr, lookupEnv)
	case "pitr":
		return runPITR(prog, args[1:], stdout, stderr, lookupEnv)
	case "archive":
		return runArchive(prog, args[1:], stdout, stderr, lookupEnv)
	case "control-plane-backups":
		return runControlPlaneBackups(prog, args[1:], stdout, stderr, lookupEnv)
	case "app-volume-backups":
		return runAppVolumeBackups(prog, args[1:], stdout, stderr, lookupEnv)
	case "cloudflare-tunnel":
		return runCloudflareTunnel(prog, args[1:], stdout, stderr, lookupEnv)
	case "vault":
		return runVault(prog, args[1:], stdout, stderr, lookupEnv)
	case "registry":
		return runRegistry(prog, args[1:], stdout, stderr, lookupEnv)
	case "channels":
		return runChannels(prog, args[1:], stdout, stderr, lookupEnv)
	case "push-subscriptions":
		return runPushSubscriptions(prog, args[1:], stdout, stderr, lookupEnv)
	case "alerts":
		return runAlerts(prog, args[1:], stdout, stderr, lookupEnv)
	case "status-page":
		return runStatusPage(prog, args[1:], stdout, stderr, lookupEnv)
	case "env":
		return runEnv(prog, args[1:], stdout, stderr, lookupEnv)
	case "shared-env":
		return runSharedEnv(prog, args[1:], stdout, stderr, lookupEnv)
	case "storage":
		return runStorage(prog, args[1:], stdout, stderr, lookupEnv)
	case "logs":
		return runLogs(prog, args[1:], stdout, stderr, lookupEnv)
	case "metrics":
		return runMetrics(prog, args[1:], stdout, stderr, lookupEnv)
	case "backup-targets":
		return runBackupTargets(prog, args[1:], stdout, stderr, lookupEnv)
	case "registry-credentials":
		return runRegistryCredentials(prog, args[1:], stdout, stderr, lookupEnv)
	case "network-shares":
		return runNetworkShares(prog, args[1:], stdout, stderr, lookupEnv)
	case "private-networks":
		return runPrivateNetworks(prog, args[1:], stdout, stderr, lookupEnv)
	case "firewall":
		return runFirewall(prog, args[1:], stdout, stderr, lookupEnv)
	case "security":
		return runSecurity(prog, args[1:], stdout, stderr, lookupEnv)
	case "flags":
		return runFlags(prog, args[1:], stdout, stderr, lookupEnv)
	case "tags":
		return runTags(prog, args[1:], stdout, stderr, lookupEnv)
	case "dashboards":
		return runDashboards(prog, args[1:], stdout, stderr, lookupEnv)
	case "pipelines":
		return runPipelines(prog, args[1:], stdout, stderr, lookupEnv)
	case "deployments":
		return runDeployments(prog, args[1:], stdout, stderr, lookupEnv)
	case "traffic":
		return runTraffic(prog, args[1:], stdout, stderr, lookupEnv)
	case "images":
		return runImages(prog, args[1:], stdout, stderr, lookupEnv)
	case "builds":
		return runBuilds(prog, args[1:], stdout, stderr, lookupEnv)
	case "cache":
		return runCache(prog, args[1:], stdout, stderr, lookupEnv)
	case "forms":
		return runForms(prog, args[1:], stdout, stderr, lookupEnv)
	case "lb":
		return runLB(prog, args[1:], stdout, stderr, lookupEnv)
	case "preview":
		return runPreview(prog, args[1:], stdout, stderr, lookupEnv)
	case "apply":
		return runApply(prog, args[1:], stdout, stderr, lookupEnv)
	case "diff":
		return runDiff(prog, args[1:], stdout, stderr, lookupEnv)
	case "export":
		return runExport(prog, args[1:], stdout, stderr, lookupEnv)
	case "assets":
		return runAssets(prog, args[1:], stdout, stderr, lookupEnv)
	case "stack":
		return runStack(prog, args[1:], stdout, stderr, lookupEnv)
	case "compliance":
		return runCompliance(prog, args[1:], stdout, stderr, lookupEnv)
	case "otel":
		return runOtel(prog, args[1:], stdout, stderr, lookupEnv)
	case "event-webhooks":
		return runEventWebhooks(prog, args[1:], stdout, stderr, lookupEnv)
	case "cdn":
		return runCDN(prog, args[1:], stdout, stderr, lookupEnv)
	case "maintenance":
		return runMaintenance(prog, args[1:], stdout, stderr, lookupEnv)
	case "policy":
		return runPolicy(prog, args[1:], stdout, stderr, lookupEnv)
	case "nodes":
		return runNodes(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		return runStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "upgrade":
		return runUpgrade(prog, args[1:], stdout, stderr, lookupEnv)
	case "updates":
		return runUpdates(prog, args[1:], stdout, stderr, lookupEnv)
	case "readiness":
		return runReadiness(prog, args[1:], stdout, stderr, lookupEnv)
	case "version":
		return runVersion(prog, args[1:], stdout, stderr, lookupEnv)
	case "update", "self-update":
		return runUpdate(prog, args[1:], stdout, stderr, lookupEnv)
	case "changelog":
		return runChangelog(prog, args[1:], stdout, stderr, lookupEnv)
	case "ai-control":
		return runAIControl(prog, args[1:], stdout, stderr, lookupEnv)
	case "audit-log":
		return runAuditLog(prog, args[1:], stdout, stderr, lookupEnv)
	case "audit-verify":
		return runAuditVerify(prog, args[1:], stdout, stderr, lookupEnv)
	case "audit-purge":
		return runAuditPurge(prog, args[1:], stdout, stderr, lookupEnv)
	case "diagnose":
		return runDiagnose(prog, args[1:], stdout, stderr, lookupEnv)
	case "inbox":
		return runInbox(prog, args[1:], stdout, stderr, lookupEnv)
	case "attention":
		return runAttention(prog, args[1:], stdout, stderr, lookupEnv)
	case "tui":
		return runTUI(prog, args[1:], stdout, stderr, lookupEnv)
	case "init":
		return runInit(prog, args[1:], stdout, stderr, lookupEnv)
	case "fixes":
		return runFixes(prog, args[1:], stdout, stderr, lookupEnv)
	case "bench":
		return runBench(prog, args[1:], stdout, stderr, lookupEnv)
	case "resilience":
		return runResilience(prog, args[1:], stdout, stderr, lookupEnv)
	case "go-live":
		return runGoLiveChecklist(prog, args[1:], stdout, stderr, lookupEnv)
	case "doctor":
		return runDoctor(prog, args[1:], stdout, stderr, lookupEnv)
	case "setup":
		return runSetup(prog, args[1:], stdout, stderr, lookupEnv)
	case "proxy":
		return runProxy(prog, args[1:], stdout, stderr, lookupEnv)
	case "auth-engine":
		return runAuthEngine(prog, args[1:], stdout, stderr, lookupEnv)
	case "api-docs":
		return runAPIDocs(prog, args[1:], stdout, stderr, lookupEnv)
	case "deploy-link":
		return runDeployLink(prog, args[1:], stdout, stderr, lookupEnv)
	case "search":
		return runSearch(prog, args[1:], stdout, stderr, lookupEnv)
	case "janitor":
		return runJanitor(prog, args[1:], stdout, stderr, lookupEnv)
	case "containers":
		return runContainers(prog, args[1:], stdout, stderr, lookupEnv)
	case "system":
		return runSystem(prog, args[1:], stdout, stderr, lookupEnv)
	case "system-prune":
		return runSystemPrune(prog, args[1:], stdout, stderr, lookupEnv)
	case "network":
		return runNetwork(prog, args[1:], stdout, stderr, lookupEnv)
	case "docker-guard":
		return runDockerGuard(prog, args[1:], stdout, stderr, lookupEnv)
	case "advisor":
		return runAdvisor(prog, args[1:], stdout, stderr, lookupEnv)
	case "resources":
		return runResources(prog, args[1:], stdout, stderr, lookupEnv)
	case "costs":
		return runCosts(prog, args[1:], stdout, stderr, lookupEnv)
	case "spend":
		return runSpend(prog, args[1:], stdout, stderr, lookupEnv)
	case "usage":
		return runUsage(prog, args[1:], stdout, stderr, lookupEnv)
	case "ingress-limits":
		return runIngressLimits(prog, args[1:], stdout, stderr, lookupEnv)
	case "budget":
		return runBudget(prog, args[1:], stdout, stderr, lookupEnv)
	case "volumes-orphaned":
		return runVolumesOrphaned(prog, args[1:], stdout, stderr, lookupEnv)
	case "volumes-orphaned-cleanup":
		return runVolumesOrphanedCleanup(prog, args[1:], stdout, stderr, lookupEnv)
	case "roles":
		return runRoles(prog, args[1:], stdout, stderr, lookupEnv)
	case "sso":
		return runSSO(prog, args[1:], stdout, stderr, lookupEnv)
	case "session-policy":
		return runSessionPolicy(prog, args[1:], stdout, stderr, lookupEnv)
	case "teams":
		return runTeams(prog, args[1:], stdout, stderr, lookupEnv)
	case "quota":
		return runQuota(prog, args[1:], stdout, stderr, lookupEnv)
	case "members":
		return runMembers(prog, args[1:], stdout, stderr, lookupEnv)
	case "users":
		return runUsers(prog, args[1:], stdout, stderr, lookupEnv)
	case "invites":
		return runInvites(prog, args[1:], stdout, stderr, lookupEnv)
	case "email":
		return runEmail(prog, args[1:], stdout, stderr, lookupEnv)
	case "iam":
		return runIAM(prog, args[1:], stdout, stderr, lookupEnv)
	case "secrets":
		return runSecrets(prog, args[1:], stdout, stderr, lookupEnv)
	case "migrate":
		return runMigrate(prog, args[1:], stdout, stderr, lookupEnv)
	case "import":
		return runImport(prog, args[1:], stdout, stderr, lookupEnv)
	case "completion":
		return runCompletion(prog, args[1:], stdout, stderr, lookupEnv)
	case "settings":
		return runSettings(prog, args[1:], stdout, stderr, lookupEnv)
	case "git-providers":
		return runGitProviders(prog, args[1:], stdout, stderr, lookupEnv)
	case "github-app":
		return runGitHubApp(prog, args[1:], stdout, stderr, lookupEnv)
	case "compose":
		return runCompose(prog, args[1:], stdout, stderr, lookupEnv)
	case "templates":
		return runTemplates(prog, args[1:], stdout, stderr, lookupEnv)
	case "static-sites":
		return runStaticSites(prog, args[1:], stdout, stderr, lookupEnv)
	case "deploy-approvals":
		return runDeployApprovals(prog, args[1:], stdout, stderr, lookupEnv)
	case "build":
		return runBuild(prog, args[1:], stdout, stderr, lookupEnv)
	case "ai":
		return runAI(prog, args[1:], stdout, stderr, lookupEnv)
	case "environments":
		return runEnvironments(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown command %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, rootUsage(prog))
		return exitUsage
	}
}

func rootUsage(prog string) string {
	return filterExperimentalUsage(fmt.Sprintf(`%[1]s: a scriptable client for the control plane API.

Usage:
  %[1]s up [dir] [--with postgres,redis] [--domain D] [flags]   deploy a directory with one command: infer, create or update, databases, build, domain
  %[1]s login [--device] [flags]     sign in and save a token, same as auth login
  %[1]s open <app> [flags]           print an app's public URLs
  %[1]s dev [status|stop] [flags]    run a private instance on this machine, apps at <app>.localhost
  %[1]s backup <database> [--target ID] [flags]   back a database up now, same as backups trigger
  %[1]s restore <database> --latest --confirm NAME [flags]   restore a database, same as backups restore
  %[1]s apps create [flags]         create an app
  %[1]s apps list [flags]             list apps
  %[1]s apps get <name> [flags]       show one app
  %[1]s apps deploy <name> [flags]   deploy an image to an existing app
  %[1]s apps deploy-compose <name> --file compose.yaml [flags]   deploy a Docker Compose file as an app
  %[1]s import <repo-url|image|-f file|--docker-run "..."> [--deploy] [flags]   preview or deploy anything: repo, image, docker run, compose, Dockerfile
  %[1]s git remote add|show|rotate|branch|revoke <app> [flags]   deploy with git push to your own server (beta)
  %[1]s apps rollback <name> [flags]   redeploy an older image (same endpoint as deploy)
  %[1]s apps restart <name> [flags]     recreate the running container, no image change
  %[1]s apps status <name> [flags]   show an app's current reconcile conditions
  %[1]s apps network <name> [flags]   show the live traffic path: container port, host port, running
  %[1]s apps logs <name> [flags]     search an app's stored log entries
  %[1]s apps exec <name> -- <cmd> [args...]   run a command in the app's container, exits with its real exit code
  %[1]s deploy <name> [flags]        convenience alias for apps deploy
  %[1]s rollback <name> [flags]      convenience alias for apps rollback
  %[1]s databases create [flags]     create a managed database
  %[1]s databases list [flags]         list databases
  %[1]s databases get <name> [flags]   show one database
  %[1]s functions deploy|list|invoke|delete|settings|versions|rollback [flags]   image apps that sleep when idle and wake on the first request
  %[1]s models list|get|deploy|logs|delete|restart|rotate-key|gpus [flags]   AI models on GPU nodes
  %[1]s domains list [flags]           list every app's domains in one call
  %[1]s dns zones|records|check|health-checks ...   DNS zones, records, delegation and propagation at Cloudflare or Route53
  %[1]s preview status|enable|disable|capture|prune <app> [flags]   deploy preview screenshots (opt-in per app)
  %[1]s previews list|delete|extend|settings|approve [flags]   per pull request preview environments
  %[1]s deployments list|summary|watch [flags]   deploys across all apps, filterable, with live stream
  %[1]s environments list|create|update|delete [flags]   instance-wide dev, test, uat, production and custom environments
  %[1]s apps move-env <name> <environment-id> [--confirm]   move an app to another environment
  %[1]s databases move-env <name> <environment-id> [--confirm]   move a database to another environment
  %[1]s backups list|trigger|restore <database> [flags]   database backup history, manual trigger, and restore
  %[1]s pitr enable|disable|status|base-backups|restore <database> [flags]   point-in-time restore (postgres only)
  %[1]s app-volume-backups list|trigger|restore <app> <volume> [flags]   app volume backup history, manual trigger, and restore
  %[1]s cloudflare-tunnel get|set|disconnect [flags]   expose the control plane through a Cloudflare Tunnel
  %[1]s vault get|set|disconnect [flags]               configure resolving app secrets from an external HashiCorp Vault
  %[1]s channels list|create|delete|test [flags]           manage notification channels (Slack, Discord, Telegram, email, Pushover, webhook)
  %[1]s push-subscriptions list|revoke [flags]             list or revoke registered browser push subscriptions
  %[1]s alerts silences|silence|maintenance|history        mute alerts, schedule maintenance windows, review alert history
  %[1]s status-page get|set|preview|components|incidents    manage the opt-in public status page
  %[1]s env pull|push <app> [--file .env] [flags]   sync an app's env vars with a local .env file
  %[1]s shared-env list|set|delete --scope SCOPE --id ID [flags]   manage project/organization/environment-scoped shared env vars, plain or secret
  %[1]s backup-targets list|get|create|update|delete [flags]   manage connected S3-compatible backup destinations
  %[1]s storage providers|list|add|test|delete [flags]   manage S3-compatible storage destinations (AWS S3, R2, B2, MinIO, Wasabi, custom)
  %[1]s logs archive set|status|remove, logs dump|ls|fetch [flags]   archive node-local logs to a storage destination
  %[1]s logs search|failure <app> [flags]   search stored logs by text, level, container and JSON fields; show the failing container's last lines
  %[1]s metrics query|top|investigate [flags]   downsampled series with period comparison, top apps, and the spike investigation view
  %[1]s traffic export sinks list|add|set|rm|test|status [flags]   export edge requests to an analytics vendor (PostHog, OTLP, webhook), no visitor identity by default
  %[1]s traffic <app> [--since 5m|1h|24h|7d] [--watch] [flags]   request rate, errors and p50/p95/p99 latency per app and replica, slow routes, SLO burn
  %[1]s cache status|rules|purge|history <app> [flags]   response cache at the ingress: hit rate, rules, purge by URL, prefix, tag or all
  %[1]s forms list|create|update|snippet|fields|test|delete|submissions|spam|unspam|remove|export <app> [flags]   form capture endpoints for static sites with spam trap, inbox and CSV export
  %[1]s builds list|show|cancel|retry|cache|prune [flags]   build queue and history, cancel and retry, BuildKit cache inventory and prune preview
  %[1]s images list|layers|gc|verify [flags] image inventory per node with size and owners, layer breakdown, retention GC that keeps rollback targets, signature check
  %[1]s registry-credentials list|get|create|update|delete [flags]   manage private container registry pull credentials
  %[1]s network-shares list|get|create|update|delete|test [flags]    manage NFS/CIFS network share mounts
  %[1]s private-networks list|map|create|set|delete|allow|deny|apply [flags]    private networks and app to app allow rules
  %[1]s firewall list|allow|deny|delete [flags]                 manage declarative host firewall rules
  %[1]s security posture|sessions|policy [flags]                security score, sessions and devices, security policy
  %[1]s registry status|enable|disable [flags]                 manage Levelrail's own built-in container registry
  %[1]s flags create|list|get|set|delete [flags]              manage feature flags, read live by a running app via GET /api/v1/flags/evaluate/{key}
  %[1]s pipelines list|validate|save|delete|run|runs|logs|cancel|approve [flags]   CI/CD pipelines: run, watch, approve, cancel
  %[1]s dashboards list|show|create|export|import|delete [flags]   custom dashboards of metric, log, deploy and cost widgets, exported as JSON for version control
  %[1]s tags list|create|delete|apps [flags]                manage tags, always identified by name, arbitrary labels for organizing and filtering apps
  %[1]s lb show|set|clear|status|export|import <app> [flags]  load balancer across an app's replicas: config, live upstreams, terraform/cdk/cloudformation/caddy export
  %[1]s apply -f file|dir|- [--dry-run] [--prune --source S] [flags]   converge to YAML resource files: plan, diff, apply
  %[1]s diff -f dir [flags]                                    drift between the files and live state, exit 2 when they differ
  %[1]s export [--project P] [--app A] [-o dir|-] [flags]      write live state as stable resource files
  %[1]s stack list|show|create|plan|apply|history|show-revision|diff|rollback|pin|unpin|unlock|delete|settings <name> [flags]   stacks: named, versioned sets of resources with revisions and rollback
  %[1]s export destinations list|create|test|status|enable|disable|delete, export audit   send logs, audit events and metrics to your own tools
  %[1]s otel destinations list|add|test|remove                 OTLP/HTTP metric and log export to your own collector
  %[1]s event-webhooks list|create|test|deliveries|replay|enable|disable|rotate-secret|delete   signed outbound event webhooks with a delivery log
  %[1]s policy list|set|check [flags]                           deploy guardrails: registries, probes, limits, mounts, labels, replicas
  %[1]s maintenance list|create|delete [flags]               scheduled maintenance windows with banner, status page and optional deploy freeze
  %[1]s cdn overview [--window 1h]                          fleet traffic, edge cache hit rate and per-node request share
  %[1]s apps tag <name> <tag> [flags]                          attach a tag (by name) to an app
  %[1]s apps untag <name> <tag> [flags]                        detach a tag (by name) from an app
  %[1]s nodes list|get|delete [flags]                        manage nodes
  %[1]s nodes join-token [flags]                             mint a one-time node enrollment token
  %[1]s nodes cordon|uncordon|drain|health|workloads <id> [flags]   node scheduling and maintenance
  %[1]s nodes mesh|rotate-key [id] [flags]                          WireGuard mesh status and key rotation
  %[1]s status [flags]                                        control plane status, including local Docker daemon reachability
  %[1]s upgrade [--no-backup] [flags]                          preflight checks, backup, and the command that upgrades (never upgrades itself)
  %[1]s upgrade --list [--channel stable|beta|all]             last 5 releases with a database schema compatibility verdict
  %[1]s upgrade --rollback-plan <version>                      read-only rollback preview, applied on the host
  %[1]s upgrade retry|rollback <attempt>                       re-run a failed upgrade attempt, or restore its pre-upgrade restore point
  %[1]s upgrade --history [--json] | --ack <id>                recorded upgrades, rollbacks and installs, and acknowledging one
  %[1]s updates restore-points list|restore <id> [--yes]       pre-upgrade restore points and putting one back
  %[1]s version [flags]                                       running control plane version, and whether a newer release is published
  %[1]s update [--check] [--version TAG] [--dry-run] [--yes]  check for a newer CLI release, or replace this binary with a verified one
  %[1]s changelog [--limit N] [flags]                         recent release notes from the control plane's own CHANGELOG.md
  %[1]s ai-control status|set|revoke-agents [flags]           what agents and AI may do: off, observe, operate, admin
  %[1]s network mesh status|enable|disable [flags]            the WireGuard mesh between nodes
  %[1]s docker-guard status|set [flags]                       the Docker API allowlist proxy: off, audit, enforce
  %[1]s audit-log [flags]                                     who changed what, --format csv to export
  %[1]s audit-verify [flags]                                  check the audit log for edited, removed or cut-off entries
  %[1]s audit-purge [flags]                                 delete audit log entries past the retention window now
  %[1]s diagnose <app|database|domain|node> <name> [flags]    why isn't this running: ranked causes, evidence and next actions
  %[1]s attention [flags]                                     everything failing right now: apps, nodes, certificates, doctor checks
  %[1]s tui [--read-only] [flags]                             interactive terminal UI: apps, databases, deploys, logs, restart
  %[1]s advisor findings|summary|apply|dismiss|snooze [flags]  resource advice (beta): limits, idle apps, database tuning
  %[1]s resources list|tag|untag|views [flags]                 every app, database, node, domain, bucket and network in one list, tags, bulk tag with --dry-run, saved views
  %[1]s usage report|settings|budgets [flags]                  cost showback per app, project and team
  %[1]s spend caps list|status|set|rm|dry-run|override        per app and project spend caps with a deploy guard and a pause preview
  %[1]s costs explore|budgets [flags]                         cost explorer with forecast and movers, named budgets with alerts and a 100%% action
  %[1]s ingress-limits show|set|clear APP [flags]             per-app request body cap
  %[1]s budget show|set|clear APP [flags]                      per-app monthly egress and cost cap with throttle or pause
  %[1]s budget [set|delete] project|team ID [flags]            alias for usage budgets
  %[1]s init [--dry-run] [--force] [--yes] [flags]            detect the stack, write app.yaml, AGENTS.md and .mcp.json for AI agents
  %[1]s bench run --url URL [flags]                           load and failover benchmark: open-loop rate, latency percentiles, ingress overhead
  %[1]s bench soak [flags]                                    churn a control plane and fail on RSS, goroutine, FD or WAL growth
  %[1]s bench chaos <list|run|report|disk|free-space> [flags]              failure scenarios measured on local containers, recorded for the resilience report
  %[1]s resilience check [--simulate] [--fix] [flags]         will apps keep serving if the control plane is down (dry run)
  %[1]s doctor [flags]                                        health checks (same as the dashboard Doctor page), --fix applies safe fixes
  %[1]s go-live <app> [domain] [--wait] [--timeout 5m] [flags]  the guided checklist from deploy to https, exits non-zero until live with --wait
  %[1]s setup status|next|skip|undo|reset [flags]             the Get started checklist: what is left before production
  %[1]s proxy [--domain D] [--verify] [flags]                 put the dashboard behind the proxy that owns ports 80 and 443
  %[1]s proxy setup|install|status|apply|verify|disable [flags] let this server manage its routes in that proxy (Traefik)
  %[1]s api-docs [flags]                                      every registered HTTP route, the same data as Settings > API explorer
  %[1]s deploy-link --image REF --name N --port P [flags]     print a "Deploy with" link and README button for an image, git repo or template
  %[1]s containers [flags]                                    every container on this node, managed by %[1]s or not
  %[1]s archive ls|create|restore|verify|download|delete|policy [flags]   archive cold audit, deploy and pipeline rows into bundles and bring them back
  %[1]s control-plane-backups list|create|download|verify|delete [flags]   snapshot, verify and export the control plane's own database
  %[1]s system address [--domain NAME] [flags]              show this server's public IPs and the DNS records to create
  %[1]s search <query...> [--kind K] [flags]                  find apps, databases, nodes, templates, settings and actions; search saved list|add|rm keeps queries
  %[1]s janitor <status|run|policy> [flags]                   automatic cleanup of unused images, build cache, volumes and old deploy attempts
  %[1]s system-prune [flags]                                remove stopped containers, dangling images, and unused volumes/build cache, fleet-wide
  %[1]s volumes-orphaned [flags]                              list this instance's named volumes no app or database references any more
  %[1]s volumes-orphaned-cleanup --names name1,name2 [flags]   remove exactly the named orphaned volumes, after re-confirming each is still orphaned
  %[1]s users list|create|set-abilities|delete|roles [flags]   manage users and their abilities, directly or via a curated role
  %[1]s roles list|create|update|delete [flags]                stored roles, including the guest role and custom roles
  %[1]s users role set <user> <role> [flags]                   assign a stored role to a user
  %[1]s users grants get|set <user> [--environment ID ...]     the environments a guest user can see
  %[1]s invites create|list|revoke [flags]                     invite a teammate by email, list or revoke pending invites
  %[1]s teams list|create|show|rename|delete|use|roles|transfer|events|members|invites   teams, their members, roles and invitations
  %[1]s quota show|set|clear <team|project> <id> [flags]       per-team and per-project limits on apps, databases, memory, CPU and previews
  %[1]s members list|set-role|remove|leave <team> ...          shortcut for "teams members"
  %[1]s --team ID [--project ID] <command> ...                 narrow any command to one team or project (also APP_TEAM, APP_PROJECT, "teams use")
  %[1]s iam policies create|list|get|update|delete|attach|detach|attachments [flags]   resource-scoped Allow/Deny policies, additive on top of --abilities
  %[1]s iam templates list|apply <id> [--param KEY=VALUE] [--attach-user ID|--attach-token ID]   ready-made policies (read-only, guest-one-environment, deployer-nonprod, ...)
  %[1]s secrets generate-master-key --out PATH   write a new master key file (local, no API call)
  %[1]s secrets rotate-master-key --new-key-file PATH [flags]   rotate the envelope-encryption master key
  %[1]s secrets binding-status|rebind [flags]   count and bind secret values not yet bound to their slot
  %[1]s auth login [flags]             authenticate and persist a new API token
  %[1]s auth login --profile NAME [flags]   authenticate and save it under a named profile instead of overwriting "default"
  %[1]s auth whoami [flags]           show who the current token authenticates as
  %[1]s profile list [flags]           list configured credentials profiles and their API URLs
  %[1]s tokens create|list|revoke [flags]   manage API tokens (requires a live session, see "%[1]s tokens -h")
  %[1]s mobile pair|devices [flags]   pair a phone with the mobile app and manage paired phones
  %[1]s plugins list|show|inspect|install|upgrade|approve|enable|disable|restart|uninstall|run|trust [flags]   signed plugins (needs the plugins experimental feature)
  %[1]s migrate coolify --url URL --token TOKEN [flags]   migrate apps from a Coolify instance
  %[1]s import platform coolify|dokploy|caprover|heroku|vercel|render --url URL [flags]   import apps from another platform, see "%[1]s import platform -h"
  %[1]s import config [dir] [--out FILE] [--strict] [--json]   offline: propose an app.yaml from Procfile, app.json and vercel.json, see "%[1]s import config -h"
  %[1]s import takeover plan|run|status|rollback [flags]   move this server's ports 80 and 443 from the old proxy to the built-in ingress, see "%[1]s import takeover -h"
  %[1]s completion bash|zsh|fish                          print a shell completion script, see "%[1]s completion -h"
  %[1]s settings oauth|email|ingress|ai-assistant get|set [flags]   configure OAuth sign-in, outbound email, ingress/ACME, and the BYOK AI assistant
  %[1]s git-providers [flags]                             connection status and capabilities for every git provider in one call
  %[1]s github-app status|disconnect|repos|branches|use-as-source [flags]   check/forget a GitHub App connection, browse and use its repos
  %[1]s templates list|get|deploy|install [flags]         browse, deploy and install from the curated service catalog
  %[1]s compose deploy|status|logs|redeploy|rollback|down  run a Docker Compose file as one stack
  %[1]s static-sites list|delete|routes [flags]            list build.type: static apps, remove an orphaned site, show and test routing rules
  %[1]s deploy-approvals list|get|approve|reject [flags]   two-person approval gate on a deploy/promote into a protected environment
  %[1]s build detect|branches --repo-url URL [flags]       check what framework a public repo would build as, or list its branches
  %[1]s backups restores <database> [flags]                database restore attempt history
  %[1]s ai chat "<message>" [--session ID] [flags]          talk to the in-app AI assistant, streamed
  %[1]s ai sessions list|get|delete|resolve [flags]         manage chat sessions and pending tool-call confirmations
  %[1]s pitr restores <database> [flags]                   point-in-time restore attempt history
  %[1]s app-volume-backups restores <app> <volume> [flags]   volume restore attempt history

Auth and target:
  --debug                 trace every outgoing request's method, URL, and
                          response status/timing to stderr (never stdout);
                          Authorization and any token are always redacted
  --token, %[2]s          API token
  --api-url, %[3]s      control plane base URL (default %[4]s)
  --profile, %[5]s        named credentials profile to use (default "%[6]s")
  or a credentials file at ~/.config/%[1]s/credentials, INI-style, with a
  [default] section (and any named [profile] sections alongside it), each
  holding its own %[2]s=... and %[3]s=... lines. An operator managing more
  than one control plane saves each under its own name via
  "%[1]s auth login --profile NAME" and switches between them with
  --profile/%[5]s, without re-authenticating or overwriting anything.
  An explicit --token/--api-url flag always wins over the resolved
  profile, the same "flag beats everything" precedence every other
  command in this CLI already follows.

Run "%[1]s apps -h", "%[1]s databases -h", or "%[1]s <command> <subcommand> -h" for more.
`, prog, envAPIToken, envAPIURL, defaultAPIURL, envProfile, defaultProfile))
}
