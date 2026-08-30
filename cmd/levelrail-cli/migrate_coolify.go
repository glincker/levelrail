package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

type migrateCoolifyFlags struct {
	coolifyURL          string
	coolifyToken        string
	outDir              string
	apply               bool
	includeSecretValues bool
	jsonOut             bool
}

// runMigrateCoolify implements "migrate coolify": fetch every
// application from a live Coolify instance, map each to a Levelrail
// service, and either write app.yaml files plus a migration report
// (default) or create the apps directly on a target Levelrail instance
// (--apply).
//
// --token here is Coolify's own API token, matching the founder-approved
// command signature. The target Levelrail instance's own token/API URL
// (--apply only) use distinctly-named flags (--target-token,
// --target-api-url) rather than reusing --token/--api-url: this command
// is the one place in this CLI that talks to two different APIs in one
// invocation, so the usual --token/--api-url meaning ("this CLI's own
// control plane") would be ambiguous here. Both still resolve through
// the exact same resolveToken/resolveAPIURL precedence chain
// (APP_API_TOKEN/APP_API_URL, then the credentials file) every other
// command in this CLI already uses.
func runMigrateCoolify(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" migrate coolify", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var f migrateCoolifyFlags
	var targetTokenFlag, targetAPIURLFlag string
	fs.StringVar(&f.coolifyURL, "url", "", "Coolify instance base URL, e.g. https://coolify.example.com (required)")
	fs.StringVar(&f.coolifyToken, "token", "", "Coolify API bearer token (required)")
	fs.StringVar(&f.outDir, "out-dir", "./migrated", "directory to write generated app.yaml files and secrets into (file mode only, i.e. when --apply is not given)")
	fs.BoolVar(&f.apply, "apply", false, "create apps directly on the target Levelrail instance (POST /api/v1/apps) instead of writing files")
	fs.BoolVar(&f.includeSecretValues, "include-secret-values", false, "fetch real env var values from Coolify (requires a Coolify token with the read:sensitive ability); never written into a generated app.yaml, only into a companion secrets file or applied via PUT .../secrets/{key}")
	fs.StringVar(&targetTokenFlag, "target-token", "", "Levelrail API token, --apply only (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&targetAPIURLFlag, "target-api-url", "", "Levelrail control plane base URL, --apply only (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&f.jsonOut, "json", false, "print the migration report as JSON to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, migrateCoolifyUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	if f.coolifyURL == "" || f.coolifyToken == "" {
		return reportError(stdout, stderr, f.jsonOut, newValidationError("--url and --token (Coolify's own) are both required"))
	}

	ctx := context.Background()
	coolify := NewCoolifyClient(f.coolifyURL, f.coolifyToken)

	apps, err := coolify.ListApplications(ctx)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("list coolify applications: %w", err))
	}

	report := migrationReport{CoolifyURL: f.coolifyURL, Applied: f.apply}
	for _, summary := range apps {
		report.Apps = append(report.Apps, fetchAndMapApplication(ctx, coolify, summary, f.includeSecretValues))
	}

	return runMigrationPipeline(ctx, &report, migratePipelineFlags{
		apply: f.apply, outDir: f.outDir, jsonOut: f.jsonOut,
		targetToken: targetTokenFlag, targetAPIURL: targetAPIURLFlag,
	}, prog, stdout, stderr, lookupEnv)
}

// fetchAndMapApplication loads one Coolify application's full detail and
// env vars and maps it. A fetch failure for this one app is reported as
// a blocking issue on that app alone, not a whole-command failure: one
// application Coolify can't return detail for (deleted mid-migration,
// permission gap) shouldn't stop every other app from migrating.
func fetchAndMapApplication(ctx context.Context, coolify *CoolifyClient, summary coolifyApplication, includeSecretValues bool) mappedApp {
	full, err := coolify.GetApplication(ctx, summary.UUID)
	if err != nil {
		return blockingFetchIssue(summary.Name, fmt.Sprintf("could not fetch full application detail: %v", err))
	}
	envs, err := coolify.ListEnvs(ctx, summary.UUID)
	if err != nil {
		return blockingFetchIssue(summary.Name, fmt.Sprintf("could not fetch environment variables: %v", err))
	}
	return mapCoolifyApplication(full, envs, includeSecretValues)
}

func migrateCoolifyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s migrate coolify --url URL --token TOKEN [flags]

One-way migration from a live Coolify instance to Levelrail. Default
mode writes app.yaml files plus a migration report; --apply creates apps
directly on a target Levelrail instance instead of writing files.

Flags:
  --url string                     Coolify instance base URL, e.g. https://coolify.example.com (required)
  --token string                   Coolify API bearer token (required)
  --out-dir string                directory for generated files, default ./migrated (file mode only)
  --apply                            create apps on the target Levelrail instance instead of writing files
  --include-secret-values     fetch real env var values from Coolify (requires a Coolify token with the read:sensitive ability)
  --target-token string        Levelrail API token, --apply only (default: %[2]s env var, then the credentials file)
  --target-api-url string    Levelrail control plane base URL, --apply only (default: %[3]s env var, then %[4]s)
  --json                             print the migration report as JSON to stdout, nothing else
  -h, --help                       show this help

Git repo/branch identity is not persisted Levelrail state today: the
report prints the exact "apps create --repo ... --ref ..." follow-up
command for each migrated app that had one.
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
