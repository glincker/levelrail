package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/clitree"
)

// runOpen implements "open <app>": print the app's public addresses, primary first.
func runOpen(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "open", "print the addresses as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, openUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "open", "app name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	app, err := client.GetApp(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get app %q: %w", name, err))
	}
	urls := []string{}
	for _, d := range app.Domains {
		urls = append(urls, "https://"+d)
	}
	if app.FallbackURL != "" {
		urls = append(urls, app.FallbackURL)
	}
	if len(urls) == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("app %q has no public address yet; add one with \"%s apps domains add %s DOMAIN\"", name, prog, name))
	}
	return writeScheduledTaskResult(stdout, stderr, of, urls, func() { _, _ = fmt.Fprintln(stdout, strings.Join(urls, "\n")) })
}

func openUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s open <app> [flags]

Prints the app's public addresses, custom domains first, then the automatic URL.

Flags:
  --token string      API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json              print the addresses as JSON to stdout, nothing else
  --output string     output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string      JMESPath expression to filter the result before printing
  -h, --help          show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runLogin is "auth login" at the top level.
func runLogin(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runAuthLogin(prog, args, stdout, stderr, lookupEnv, os.Stdin)
}

// runBackupAlias is "backup <db>", the same call as "backups trigger <db>".
func runBackupAlias(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runBackupsTrigger(prog, args, stdout, stderr, lookupEnv)
}

// runRestoreAlias is "restore <db>", the same call as "backups restore <db>".
func runRestoreAlias(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runBackupsRestore(prog, args, stdout, stderr, lookupEnv, os.Stdin)
}

// simpleVerbHelp is the one-line summary shown for each short-help verb.
var simpleVerbHelp = map[string]string{
	"up":       "up [dir] [--with postgres,redis] [--domain D]   deploy this directory: create or update the app, databases, build, domain",
	"deploy":   "deploy [dir] | <app> --image IMAGE | --latest   upload this directory and build it, or deploy an image or the newest commit",
	"rollback": "rollback <app> [--image IMAGE]   go back to the previous successful deploy",
	"logs":     "logs <app> [flags]   an app's recent logs (admin forms: logs archive|dump|ls|fetch|search)",
	"env":      "env set|pull|push <app> ...   change or sync an app's environment variables",
	"domains":  "domains list | apps domains add <app> <domain>   domains and their routing",
	"db":       "db create|list|get ...   managed databases (alias of databases)",
	"backup":   "backup <db> [--target ID]   back a database up now",
	"restore":  "restore <db> --latest --confirm <db>   restore a database from its newest backup",
	"apps":     "apps list|get|create|scale|restart|status|logs ...   every app command",
	"status":   "status   control plane health at a glance",
	"open":     "open <app>   print the app's public URLs",
	"login":    "login [--device]   sign in and save a token",
}

// simpleUsage is the short help: the everyday verbs only.
func simpleUsage(prog string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: deploy and run apps on your servers.\n\nUsage:\n", prog)
	for _, v := range clitree.SimpleVerbs() {
		fmt.Fprintf(&b, "  %s %s\n", prog, simpleVerbHelp[v])
	}
	fmt.Fprintf(&b, `
Quick start, in a project directory with a git remote:
  %[1]s login --device
  %[1]s up --with postgres
  %[1]s open <app>

Every command takes -h for its flags. "%[1]s help advanced" lists the other
commands: nodes, networking, teams, policies, benchmarks, migration and more.
`, prog)
	return b.String()
}
