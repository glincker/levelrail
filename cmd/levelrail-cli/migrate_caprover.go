package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

type migrateCaproverFlags struct {
	caproverURL         string
	caproverToken       string
	outDir              string
	apply               bool
	includeSecretValues bool
	jsonOut             bool
}

// runMigrateCaprover implements "migrate caprover", mirroring
// runMigrateDokploy (migrate_dokploy.go): same flag surface, same
// file-vs-apply modes, same report shape. --token is CapRover's own login
// password: unlike Coolify and Dokploy's static tokens, CapRover
// authenticates via a login exchange (CaproverClient.Login), not a bearer
// token passed straight through.
func runMigrateCaprover(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" migrate caprover", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var f migrateCaproverFlags
	var targetTokenFlag, targetAPIURLFlag string
	fs.StringVar(&f.caproverURL, "url", "", "CapRover instance base URL, e.g. https://captain.example.com (required)")
	fs.StringVar(&f.caproverToken, "token", "", "CapRover login password, exchanged for a session token automatically (required)")
	fs.StringVar(&f.outDir, "out-dir", "./migrated", "directory to write generated app.yaml files and secrets into (file mode only, i.e. when --apply is not given)")
	fs.BoolVar(&f.apply, "apply", false, "create apps directly on the target Levelrail instance (POST /api/v1/apps) instead of writing files")
	fs.BoolVar(&f.includeSecretValues, "include-secret-values", false, "write the real env var values already returned by CapRover into a companion secrets file, or apply them via PUT .../secrets/{key}; never written into a generated app.yaml")
	fs.StringVar(&targetTokenFlag, "target-token", "", "Levelrail API token, --apply only (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&targetAPIURLFlag, "target-api-url", "", "Levelrail control plane base URL, --apply only (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&f.jsonOut, "json", false, "print the migration report as JSON to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, migrateCaproverUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	if f.caproverURL == "" || f.caproverToken == "" {
		return reportError(stdout, stderr, f.jsonOut, newValidationError("--url and --token (CapRover's own login password) are both required"))
	}

	ctx := context.Background()
	caprover := NewCaproverClient(f.caproverURL, f.caproverToken)

	if err := caprover.Login(ctx); err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("log in to caprover: %w", err))
	}

	apps, rootDomain, err := caprover.ListApplications(ctx)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("list caprover applications: %w", err))
	}

	report := migrationReport{CaproverURL: f.caproverURL, Applied: f.apply}
	for _, app := range apps {
		report.Apps = append(report.Apps, mapCaproverApplication(app, rootDomain, f.includeSecretValues))
	}

	return runMigrationPipeline(ctx, &report, migratePipelineFlags{
		apply: f.apply, outDir: f.outDir, jsonOut: f.jsonOut,
		targetToken: targetTokenFlag, targetAPIURL: targetAPIURLFlag,
	}, prog, stdout, stderr, lookupEnv)
}

func migrateCaproverUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s migrate caprover --url URL --token TOKEN [flags]

One-way migration from a live CapRover instance to Levelrail. Default
mode writes app.yaml files plus a migration report; --apply creates apps
directly on a target Levelrail instance instead of writing files.

Flags:
  --url string                     CapRover instance base URL, e.g. https://captain.example.com (required)
  --token string                   CapRover login password, exchanged for a session token automatically (required)
  --out-dir string                directory for generated files, default ./migrated (file mode only)
  --apply                            create apps on the target Levelrail instance instead of writing files
  --include-secret-values     write real env var values already returned by CapRover into a secrets file or apply them
  --target-token string        Levelrail API token, --apply only (default: %[2]s env var, then the credentials file)
  --target-api-url string    Levelrail control plane base URL, --apply only (default: %[3]s env var, then %[4]s)
  --json                             print the migration report as JSON to stdout, nothing else
  -h, --help                       show this help

CapRover's captain-definition build config (Dockerfile path, template, or
inline Dockerfile lines) is not exposed by its API, so every app's
build.type is assumed to be dockerfile and flagged for manual review.
CapRover does not expose git repo/branch identity for a deployed app at
all, so no follow-up "apps create --repo ..." command is printed here,
unlike migrate coolify/dokploy.
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
