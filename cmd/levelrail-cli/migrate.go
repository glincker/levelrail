package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// runMigrate dispatches "migrate <source> [flags]".
func runMigrate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, migrateUsage(prog))
		return exitUsage
	}

	if hasResumeFlag(args) {
		return runMigrateResume(prog, args, os.Stdin, stdout, stderr, lookupEnv)
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, migrateUsage(prog))
		return exitOK
	case "coolify":
		return runMigrateCoolify(prog, args[1:], stdout, stderr, lookupEnv)
	case "dokploy":
		return runMigrateDokploy(prog, args[1:], stdout, stderr, lookupEnv)
	case "caprover":
		return runMigrateCaprover(prog, args[1:], stdout, stderr, lookupEnv)
	case "verify":
		return runMigrateVerify(prog, args[1:], stdout, stderr, lookupEnv)
	case "server":
		return runMigrateServer(prog, args[1:], os.Stdin, stdout, stderr, lookupEnv)
	case "db-copy":
		return runMigrateDbCopy(prog, args[1:], os.Stdin, stdout, stderr, lookupEnv)
	case "db-status":
		return runMigrateDbStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "volumes":
		return runMigrateVolumes(prog, args[1:], stdout, stderr, lookupEnv)
	case "cutover":
		return runMigrateCutover(prog, args[1:], stdout, stderr, lookupEnv)
	case "plan":
		return runMigratePlan(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		return runMigrateStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "step":
		return runMigrateStep(prog, args[1:], stdout, stderr, lookupEnv)
	case "agent":
		return runMigrateAgent(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown migrate source %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, migrateUsage(prog))
		return exitUsage
	}
}

func migrateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s migrate coolify --url URL --token TOKEN [flags]    migrate apps from a Coolify instance
  %[1]s migrate dokploy --url URL --token TOKEN [flags]    migrate apps from a Dokploy instance
  %[1]s migrate caprover --url URL --token TOKEN [flags]   migrate apps from a CapRover instance
  %[1]s migrate verify <coolify|dokploy|caprover> --url URL --token TOKEN
                                                           parity report against the source before cutover (exit 1 on a blocking difference)

  %[1]s migrate [coolify|dokploy|caprover|server] --resume JOB [--retry] [--follow] [--retire [--probe] [--ack STEP]]
                                                           resume a migration job: status, retry failures, safe-to-delete check

Data and cutover steps, after "import platform":
`+migrateDataUsage(prog)+`
Run "%[1]s migrate <source> -h" for a source's own flags.
`, prog)
}

// blockingFetchIssue reports sourceName as blocking because a required migration fetch failed.
func blockingFetchIssue(sourceName, detail string) mappedApp {
	return mappedApp{
		SourceName: sourceName,
		Blocking:   true,
		Issues:     []migrationIssue{{Field: "fetch", Severity: issueBlocking, Detail: detail}},
	}
}

// runMigrationPipeline runs the tail every "migrate <source>" subcommand
// shares once report.Apps is populated: apply to a target Levelrail
// instance or write files, then print the report as JSON or human text.
func runMigrationPipeline(ctx context.Context, report *migrationReport, f migratePipelineFlags, prog string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	// Compose services are staged as files in both modes and kept out of the app.yaml and create paths.
	var composeApps []mappedApp
	plain := *report
	plain.Apps = nil
	for _, a := range report.Apps {
		if a.Compose == nil {
			plain.Apps = append(plain.Apps, a)
			continue
		}
		if !a.Blocking {
			stageComposeApp(&a, f.stageDir)
		}
		composeApps = append(composeApps, a)
	}
	defer func() { report.Apps = append(plain.Apps, composeApps...) }()

	if f.apply {
		profile := resolveProfile(f.targetProfile, lookupEnv)
		client := NewClient(resolveAPIURL(f.targetAPIURL, lookupEnv, prog, profile), resolveToken(f.targetToken, lookupEnv, prog, profile))
		applyMigration(ctx, client, &plain)
	} else if err := writeMigrationFiles(f.outDir, &plain); err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}

	if f.jsonOut {
		full := plain
		full.Apps = append(append([]mappedApp(nil), plain.Apps...), composeApps...)
		if err := writeJSONValue(stdout, full); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printMigrationReportHuman(stdout, plain, prog, f.outDir)
	printStagedServices(stdout, composeApps)
	return exitOK
}

// printStagedServices lists staged one-click services with the deploy command and what needs a manual fix.
func printStagedServices(out io.Writer, apps []mappedApp) {
	if len(apps) == 0 {
		return
	}
	_, _ = fmt.Fprintf(out, "\n%d one-click service(s) translated to compose apps:\n", len(apps))
	for _, a := range apps {
		_, _ = fmt.Fprintf(out, "  - %s (from %q)\n", a.ServiceName, a.SourceName)
		switch {
		case a.Compose == nil || a.Blocking:
			_, _ = fmt.Fprintln(out, "      not staged")
		case a.CreateError != "":
			_, _ = fmt.Fprintf(out, "      staging FAILED: %s\n", a.CreateError)
		default:
			_, _ = fmt.Fprintf(out, "      staged: %s (holds generated secrets, do not commit)\n      deploy: %s\n", a.Compose.StagedPath, a.Compose.Deploy)
		}
		if a.Compose != nil && len(a.Compose.Unsupported) > 0 {
			_, _ = fmt.Fprintf(out, "      unsupported, needs review: %s\n", strings.Join(a.Compose.Unsupported, ", "))
		}
		for _, iss := range a.Issues {
			_, _ = fmt.Fprintf(out, "      [%s] %s: %s\n", iss.Severity, iss.Field, iss.Detail)
		}
	}
}

type migratePipelineFlags struct {
	apply         bool
	outDir        string
	stageDir      string
	jsonOut       bool
	targetToken   string
	targetAPIURL  string
	targetProfile string
}
