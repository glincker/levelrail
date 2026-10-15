package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func databasesVersionsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases versions <engine> [--refresh]
      Versions with channel (stable, lts, rc, beta, eol), end-of-life date and source.
      --refresh looks the engine up live in its registry and endoflife.date now.
  %[1]s databases catalog status
      Where the version catalog comes from (published, direct, live, embedded), its age and freshness.
  %[1]s databases catalog refresh
      Fetch the configured catalog source now.
  %[1]s databases catalog set --remote on|off
      Switch remote catalog refresh on or off (APP_DB_CATALOG_REMOTE=off always wins).
`, prog)
}

// runDatabasesVersions implements "databases versions <engine> [--refresh]".
func runDatabasesVersions(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases versions", "print the result as JSON to stdout and nothing else", stderr)
	refresh := fs.Bool("refresh", false, "look the engine up live now")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesVersionsUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	engine, ok := requireOneArg(fs, stderr, prog, "databases versions", "engine")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var (
		res apiclient.DBVersions
		err error
	)
	if *refresh {
		res, err = client.RefreshDatabaseVersions(context.Background(), engine)
	} else {
		res, err = client.DatabaseVersions(context.Background(), engine)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("versions of %q: %w", engine, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printDBVersions(stdout, res) })
}

func printDBVersions(out io.Writer, r apiclient.DBVersions) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "VERSION\tCHANNEL\tEOL\tRECOMMENDED")
	for _, v := range r.Versions {
		rec := ""
		if v.Recommended {
			rec = "yes"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", v.Version, v.Channel, dash(v.EOL), dash(rec))
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(out, "\nsource: %s, freshness: %s, fetched %s, generated %s\n", r.Origin, r.Freshness, dash(r.FetchedAt), dash(r.GeneratedAt))
	if r.Source.EOLURL != "" {
		_, _ = fmt.Fprintf(out, "lifecycle: %s\n", r.Source.EOLURL)
	}
	if r.Message != "" {
		_, _ = fmt.Fprintln(out, r.Message)
	}
}

// runDatabasesCatalog implements "databases catalog status|refresh|set".
func runDatabasesCatalog(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesVersionsUsage(prog))
		return exitUsage
	}
	sub, rest := args[0], args[1:]
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases catalog "+sub, "print the result as JSON to stdout and nothing else", stderr)
	remote := fs.String("remote", "", "on or off (set only)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesVersionsUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, rest, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var (
		st  apiclient.DBCatalogStatus
		err error
	)
	switch sub {
	case "status":
		st, err = client.DatabaseCatalogStatus(context.Background())
	case "refresh":
		st, err = client.RefreshDatabaseCatalog(context.Background())
	case "set":
		if *remote != "on" && *remote != "off" {
			_, _ = fmt.Fprintln(stderr, "databases catalog set: --remote on|off is required")
			return exitUsage
		}
		st, err = client.SetDatabaseCatalogRemote(context.Background(), *remote == "on")
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases catalog subcommand %q\n", prog, sub)
		return exitUsage
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("database catalog %s: %w", sub, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, st, func() { printDBCatalogStatus(stdout, st) })
}

func printDBCatalogStatus(out io.Writer, s apiclient.DBCatalogStatus) {
	_, _ = fmt.Fprintf(out, "source:       %s (mode %s)\n", s.Source, s.Mode)
	_, _ = fmt.Fprintf(out, "url:          %s\n", s.URL)
	_, _ = fmt.Fprintf(out, "generated_at: %s\n", dash(s.GeneratedAt))
	_, _ = fmt.Fprintf(out, "fetched_at:   %s\n", dash(s.FetchedAt))
	_, _ = fmt.Fprintf(out, "freshness:    %s\n", s.Freshness)
	if s.DisabledReason != "" {
		_, _ = fmt.Fprintf(out, "remote:       off (%s)\n", s.DisabledReason)
	}
	if s.LastError != "" {
		_, _ = fmt.Fprintf(out, "last error:   %s\n", s.LastError)
	}
}
