package main

import (
	"context"
	"fmt"
	"io"
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
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown migrate source %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, migrateUsage(prog))
		return exitUsage
	}
}

func migrateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s migrate coolify --url URL --token TOKEN [flags]   migrate apps from a Coolify instance
  %[1]s migrate dokploy --url URL --token TOKEN [flags]   migrate apps from a Dokploy instance

Run "%[1]s migrate <source> -h" for a source's own flags.
`, prog)
}

// blockingFetchIssue builds a mappedApp reporting sourceName as blocking
// because a required fetch during migration failed. Shared by
// migrate_coolify.go and migrate_dokploy.go's own fetch-and-map
// functions, whose only difference at this point is which fetch call
// failed and how detail was worded.
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
		client := NewClient(resolveAPIURL(f.targetAPIURL, lookupEnv, prog), resolveToken(f.targetToken, lookupEnv, prog))
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

// migratePipelineFlags is the subset of a migrate subcommand's flags that
// runMigrationPipeline needs, kept as its own type so callers can build it
// from either migrateCoolifyFlags or migrateDokployFlags plus their local
// target-instance flag variables.
type migratePipelineFlags struct {
	apply        bool
	outDir       string
	jsonOut      bool
	targetToken  string
	targetAPIURL string
}
