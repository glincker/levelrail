package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

func runDatabasesLink(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases link", "print the resulting link as JSON to stdout and nothing else", stderr)
	var envVar, field string
	fs.StringVar(&envVar, "env-var", "", "env var name to inject (default depends on the engine, DATABASE_URL for most)")
	fs.StringVar(&field, "field", "", "database field to resolve (default \"url\")")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases link <name> <app> [flags]\n\nConnects <app> to the database: its connection env var is injected the\nnext time the app's container is created, and follows credential rotation\nthen. An app can link many databases. The app shows it as a pending change; apps apply restarts it now.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	pos, ok := requireArgs(fs, stderr, prog, "databases link", "<name> <app>", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.ConnectDatabaseApp(context.Background(), pos[0], pos[1], envVar, field)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("link database %q to app %q: %w", pos[0], pos[1], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "linked %s to %s as %s (%s)\n", pos[1], pos[0], result.EnvVar, result.Field)
		_, _ = fmt.Fprintf(stdout, "pending: %s picks it up on its next restart; apply now with %s apps apply %s\n", pos[1], prog, pos[1])
	})
}

func runDatabasesLinks(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases links", "print the links as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases links <name> [flags]\n\nLists the apps connected to the database.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "databases links", "database name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	links, err := client.ListDatabaseLinks(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list links for database %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, links, func() {
		if len(links) == 0 {
			_, _ = fmt.Fprintf(stdout, "no apps connected to %q\n", name)
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "APP\tENV VAR\tFIELD")
		for _, l := range links {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", l.App, l.EnvVar, l.Field)
		}
		_ = tw.Flush()
	})
}
