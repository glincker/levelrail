package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

type migrateDokployFlags struct {
	dokployURL          string
	dokployToken        string
	outDir              string
	apply               bool
	includeSecretValues bool
	jsonOut             bool
}

// runMigrateDokploy implements "migrate dokploy", mirroring
// runMigrateCoolify (migrate_coolify.go) exactly: same flag surface,
// same file-vs-apply modes, same report shape.
func runMigrateDokploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" migrate dokploy", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var f migrateDokployFlags
	var targetTokenFlag, targetAPIURLFlag string
	fs.StringVar(&f.dokployURL, "url", "", "Dokploy instance base URL, e.g. https://dokploy.example.com (required)")
	fs.StringVar(&f.dokployToken, "token", "", "Dokploy API key (required)")
	fs.StringVar(&f.outDir, "out-dir", "./migrated", "directory to write generated app.yaml files and secrets into (file mode only, i.e. when --apply is not given)")
	fs.BoolVar(&f.apply, "apply", false, "create apps directly on the target Levelrail instance (POST /api/v1/apps) instead of writing files")
	fs.BoolVar(&f.includeSecretValues, "include-secret-values", false, "write the real env var values already returned by Dokploy into a companion secrets file, or apply them via PUT .../secrets/{key}; never written into a generated app.yaml")
	fs.StringVar(&targetTokenFlag, "target-token", "", "Levelrail API token, --apply only (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&targetAPIURLFlag, "target-api-url", "", "Levelrail control plane base URL, --apply only (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&f.jsonOut, "json", false, "print the migration report as JSON to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, migrateDokployUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	if f.dokployURL == "" || f.dokployToken == "" {
		return reportError(stdout, stderr, f.jsonOut, newValidationError("--url and --token (Dokploy's own) are both required"))
	}

	ctx := context.Background()
	dokploy := NewDokployClient(f.dokployURL, f.dokployToken)

	apps, err := dokploy.ListApplications(ctx)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("list dokploy applications: %w", err))
	}

	report := migrationReport{DokployURL: f.dokployURL, Applied: f.apply}
	for _, summary := range apps {
		report.Apps = append(report.Apps, fetchAndMapDokployApplication(ctx, dokploy, summary, f.includeSecretValues))
	}

	return runMigrationPipeline(ctx, &report, migratePipelineFlags{
		apply: f.apply, outDir: f.outDir, jsonOut: f.jsonOut,
		targetToken: targetTokenFlag, targetAPIURL: targetAPIURLFlag,
	}, prog, stdout, stderr, lookupEnv)
}

// fetchAndMapDokployApplication loads one Dokploy application's full
// detail and domains and maps it. A fetch failure for this one app is
// reported as a blocking issue on that app alone, matching
// fetchAndMapApplication's own reasoning in migrate_coolify.go.
func fetchAndMapDokployApplication(ctx context.Context, dokploy *DokployClient, summary dokployApplicationSummary, includeSecretValues bool) mappedApp {
	full, err := dokploy.GetApplication(ctx, summary.ApplicationID)
	if err != nil {
		return blockingFetchIssue(summary.Name, fmt.Sprintf("could not fetch full application detail: %v", err))
	}
	domains, err := dokploy.ListDomains(ctx, summary.ApplicationID)
	if err != nil {
		return blockingFetchIssue(summary.Name, fmt.Sprintf("could not fetch domains: %v", err))
	}
	return mapDokployApplication(full, domains, includeSecretValues)
}

func migrateDokployUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s migrate dokploy --url URL --token TOKEN [flags]

One-way migration from a live Dokploy instance to Levelrail. Default
mode writes app.yaml files plus a migration report; --apply creates apps
directly on a target Levelrail instance instead of writing files.

Flags:
  --url string                     Dokploy instance base URL, e.g. https://dokploy.example.com (required)
  --token string                   Dokploy API key (required)
  --out-dir string                directory for generated files, default ./migrated (file mode only)
  --apply                            create apps on the target Levelrail instance instead of writing files
  --include-secret-values     write real env var values already returned by Dokploy into a secrets file or apply them
  --target-token string        Levelrail API token, --apply only (default: %[2]s env var, then the credentials file)
  --target-api-url string    Levelrail control plane base URL, --apply only (default: %[3]s env var, then %[4]s)
  --json                             print the migration report as JSON to stdout, nothing else
  -h, --help                       show this help

Dokploy's docker (prebuilt image) and drop (file upload) source types,
and its nixpacks/heroku_buildpacks/paketo_buildpacks build types, have no
Levelrail equivalent and are reported for manual review, not migrated.

Git repo/branch identity is not persisted Levelrail state today: the
report prints the exact "apps create --repo ... --ref ..." follow-up
command for each migrated app that had one.
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
