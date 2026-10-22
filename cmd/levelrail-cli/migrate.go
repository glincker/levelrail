package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

// runMigrate dispatches "migrate <source> [flags]".
func runMigrate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, migrateUsage(prog))
		return exitUsage
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
	if f.apply {
		profile := resolveProfile(f.targetProfile, lookupEnv)
		client := NewClient(resolveAPIURL(f.targetAPIURL, lookupEnv, prog, profile), resolveToken(f.targetToken, lookupEnv, prog, profile))
		applyMigration(ctx, client, report)
	} else if err := writeMigrationFiles(f.outDir, report); err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}

	if f.jsonOut {
		if err := writeJSONValue(stdout, *report); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printMigrationReportHuman(stdout, *report, prog, f.outDir)
	return exitOK
}

type migratePipelineFlags struct {
	apply         bool
	outDir        string
	jsonOut       bool
	targetToken   string
	targetAPIURL  string
	targetProfile string
}
