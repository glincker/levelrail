package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

type migrateCoolifyFlags struct {
	coolifyURL          string
	coolifyToken        string
	outDir              string
	apply               bool
	includeSecretValues bool
	jsonOut             bool
	dryRun, verify      bool
	insecureTLS         bool
	allowPrivate        bool
	only                string
}

// runMigrateCoolify implements "migrate coolify": writes app.yaml files and a report, or creates apps on a target with --apply.
// Coolify's token uses --token; the target uses --target-token and --target-api-url, since this command talks to two APIs.
func runMigrateCoolify(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" migrate coolify", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var f migrateCoolifyFlags
	var targetTokenFlag, targetAPIURLFlag, targetProfileFlag string
	fs.StringVar(&f.coolifyURL, "url", "", "Coolify instance base URL, e.g. https://coolify.example.com (required)")
	fs.StringVar(&f.coolifyToken, "token", "", "Coolify API bearer token (required)")
	fs.StringVar(&f.outDir, "out-dir", "./migrated", "directory to write generated app.yaml files and secrets into (file mode only, i.e. when --apply is not given)")
	fs.BoolVar(&f.apply, "apply", false, "create apps directly on the target Levelrail instance (POST /api/v1/apps) instead of writing files")
	fs.BoolVar(&f.includeSecretValues, "include-secret-values", false, "fetch real env var values from Coolify (requires a Coolify token with the read:sensitive ability); never written into a generated app.yaml, only into a companion secrets file or applied via PUT .../secrets/{key}")
	fs.StringVar(&targetTokenFlag, "target-token", "", "Levelrail API token, --apply only (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&targetAPIURLFlag, "target-api-url", "", "Levelrail control plane base URL, --apply only (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.StringVar(&targetProfileFlag, "target-profile", "", "named credentials profile to read the target Levelrail instance from, --apply only (overrides "+envProfile+", default \""+defaultProfile+"\")")
	fs.BoolVar(&f.dryRun, "dry-run", false, "read Coolify and print the exact plan, per-resource outcomes and the cutover checklist; creates and writes nothing")
	fs.BoolVar(&f.verify, "verify", false, "like --dry-run, but asked of the target control plane so apps already imported show as imported")
	fs.StringVar(&f.only, "only", "", "comma-separated app names or Coolify ids, --dry-run and --verify only")
	fs.BoolVar(&f.insecureTLS, "insecure-tls", false, "skip TLS verification of Coolify, --dry-run and --verify only")
	fs.BoolVar(&f.allowPrivate, "allow-private", false, "allow a private-network Coolify address (--verify also needs APP_IMPORT_ALLOW_PRIVATE_NETWORKS=true on the control plane)")
	fs.BoolVar(&f.jsonOut, "json", false, "print the migration report as JSON to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, migrateCoolifyUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}

	if f.coolifyURL == "" || f.coolifyToken == "" {
		return reportError(stdout, stderr, f.jsonOut, newValidationError("--url and --token (Coolify's own) are both required"))
	}

	ctx := context.Background()
	if f.dryRun || f.verify {
		if f.apply {
			return reportError(stdout, stderr, f.jsonOut, newValidationError("--dry-run and --verify create nothing: drop --apply"))
		}
		return runCoolifyExit(ctx, prog, coolifyExitFlags{url: f.coolifyURL, token: f.coolifyToken, verify: f.verify, insecureTLS: f.insecureTLS,
			allowPrivate: f.allowPrivate, allowLoopback: envIsTrue(lookupEnv, platformimport.AllowLoopbackEnv), jsonOut: f.jsonOut, only: f.only},
			func() *Client {
				return apiClientFromFlags(prog, targetAPIURLFlag, targetTokenFlag, targetProfileFlag, lookupEnv)
			}, stdout, stderr)
	}
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
		targetToken: targetTokenFlag, targetAPIURL: targetAPIURLFlag, targetProfile: targetProfileFlag,
	}, prog, stdout, stderr, lookupEnv)
}

// fetchAndMapApplication maps one Coolify application. A fetch failure is a blocking issue for that app only, not the whole command.
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
  --dry-run                       print the exact plan, per-resource outcomes and cutover checklist; writes nothing
  --verify                         the same report from the target, so apps already imported show as imported
  --only string                   limit --dry-run and --verify to these app names or Coolify ids
  --insecure-tls                  skip TLS verification of Coolify (--dry-run and --verify)
  --allow-private                 allow a private-network Coolify address
  --include-secret-values     fetch real env var values from Coolify (requires a Coolify token with the read:sensitive ability)
  --target-token string        Levelrail API token, --apply only (default: %[2]s env var, then the credentials file)
  --target-api-url string    Levelrail control plane base URL, --apply only (default: %[3]s env var, then %[4]s)
  --json                             print the migration report as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help

--dry-run and --verify cover what this command's file and --apply modes do
not: databases, volumes, scheduled tasks, webhooks, servers and a DNS
switch order. Stage them with "import apps --from coolify".

Git repo/branch identity is not persisted Levelrail state today: the
report prints the exact "apps create --repo ... --ref ..." follow-up
command for each migrated app that had one.
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
