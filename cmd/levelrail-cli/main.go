// Command levelrail-cli is a thin, scriptable HTTP client for the
// control plane's versioned API (internal/api, mounted at /api/v1), the
// first command-line surface this project has (cmd/levelrail is the
// control plane server, cmd/levelrail-agent is the node agent, neither
// is meant to be typed at by an operator).
//
// It is designed the way this platform's own API is meant to be:
// "AI-ready" from the start. Concretely, per the
// competitive research direction this command was built against
// (flyctl launch, railway up, vercel):
//
//   - Every required input has a flag. Supplying all of them skips every
//     prompt, so a missing required flag is reported as an immediate,
//     actionable error rather than a hang on stdin: safe to drive from
//     CI or an agent with no TTY at all. "apps create --interactive" is
//     the one deliberate exception, a step-by-step wizard for a human
//     at a real terminal (see apps_create_interactive.go).
//   - --json switches every command's stdout to a single parseable JSON
//     value (the result, or {"error": "..."} on failure) and nothing
//     else; diagnostics always go to stderr, in both modes.
//   - Exit codes are real and distinct (see output.go's exitOK/
//     exitUsage/exitValidation/exitNetwork/exitAPIError), not just 0/1,
//     so a caller can branch on *why* a call failed.
//
// No product name is hardcoded anywhere in this command: "prog" (the
// string every usage message is built from) always comes from
// filepath.Base(os.Args[0]), matching this project's standing rule that
// a CLI's command name comes from os.Args[0], not a literal, and
// APP_API_TOKEN/APP_API_URL follow
// this project's existing APP_* env-var-prefix convention (see
// internal/brand's own envPrefix), not a name-specific one.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	prog := filepath.Base(os.Args[0])
	os.Exit(run(prog, os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv))
}

// run is main's testable core: every command function below returns an
// int exit code rather than calling os.Exit directly (flag.ContinueOnError
// throughout, never flag.ExitOnError), so the whole CLI's dispatch and
// flag-parsing logic can be exercised by table-driven tests without
// forking a subprocess.
func run(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, rootUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, rootUsage(prog))
		return exitOK
	case "apps":
		return runApps(prog, args[1:], stdout, stderr, lookupEnv)
	case "databases":
		return runDatabases(prog, args[1:], stdout, stderr, lookupEnv)
	case "auth":
		return runAuth(prog, args[1:], stdout, stderr, lookupEnv)
	case "tokens":
		return runTokens(prog, args[1:], stdout, stderr, lookupEnv)
	case "domains":
		return runDomains(prog, args[1:], stdout, stderr, lookupEnv)
	case "backups":
		return runBackups(prog, args[1:], stdout, stderr, lookupEnv)
	case "cloudflare-tunnel":
		return runCloudflareTunnel(prog, args[1:], stdout, stderr, lookupEnv)
	case "channels":
		return runChannels(prog, args[1:], stdout, stderr, lookupEnv)
	case "backup-targets":
		return runBackupTargets(prog, args[1:], stdout, stderr, lookupEnv)
	case "registry-credentials":
		return runRegistryCredentials(prog, args[1:], stdout, stderr, lookupEnv)
	case "nodes":
		return runNodes(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		return runStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "users":
		return runUsers(prog, args[1:], stdout, stderr, lookupEnv)
	case "migrate":
		return runMigrate(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown command %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, rootUsage(prog))
		return exitUsage
	}
}

func rootUsage(prog string) string {
	return fmt.Sprintf(`%[1]s: a scriptable client for the control plane API.

Usage:
  %[1]s apps create [flags]         create an app
  %[1]s apps list [flags]             list apps
  %[1]s apps get <name> [flags]       show one app
  %[1]s apps deploy <name> [flags]   deploy an image to an existing app
  %[1]s apps deploy-compose <name> --file compose.yaml [flags]   deploy a Docker Compose file as an app
  %[1]s apps rollback <name> [flags]   redeploy an older image (same endpoint as deploy)
  %[1]s apps restart <name> [flags]     recreate the running container, no image change
  %[1]s apps status <name> [flags]   show an app's current reconcile conditions
  %[1]s apps network <name> [flags]   show the live traffic path: container port, host port, running
  %[1]s apps logs <name> [flags]     search an app's stored log entries
  %[1]s apps exec <name> -- <cmd> [args...]   run a command in the app's container, exits with its real exit code
  %[1]s databases create [flags]     create a managed database
  %[1]s databases list [flags]         list databases
  %[1]s databases get <name> [flags]   show one database
  %[1]s domains list [flags]           list every app's domains in one call
  %[1]s backups list|trigger|restore <database> [flags]   database backup history, manual trigger, and restore
  %[1]s cloudflare-tunnel get|set|disconnect [flags]   expose the control plane through a Cloudflare Tunnel
  %[1]s channels list|create|delete|test [flags]           manage notification channels (Slack, Discord, Telegram, email, Pushover, webhook)
  %[1]s backup-targets list|get|create|update|delete [flags]   manage connected S3-compatible backup destinations
  %[1]s registry-credentials list|get|create|update|delete [flags]   manage private container registry pull credentials
  %[1]s nodes list|get|delete [flags]                        manage nodes
  %[1]s nodes join-token [flags]                             mint a one-time node enrollment token
  %[1]s nodes cordon|uncordon|drain|health|workloads <id> [flags]   node scheduling and maintenance
  %[1]s status [flags]                                        control plane status, including local Docker daemon reachability
  %[1]s users list|create|set-abilities|delete|roles [flags]   manage users and their abilities, directly or via a curated role
  %[1]s auth login [flags]             authenticate and persist a new API token
  %[1]s auth whoami [flags]           show who the current token authenticates as
  %[1]s tokens create|list|revoke [flags]   manage API tokens (requires a live session, see "%[1]s tokens -h")
  %[1]s migrate coolify --url URL --token TOKEN [flags]   migrate apps from a Coolify instance

Auth and target:
  --token, %[2]s          API token
  --api-url, %[3]s      control plane base URL (default %[4]s)
  or a credentials file at ~/.config/%[1]s/credentials with
  %[2]s=... and %[3]s=... lines.

Run "%[1]s apps -h", "%[1]s databases -h", or "%[1]s <command> <subcommand> -h" for more.
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
