package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"text/tabwriter"
)

// databaseTagFlagsHelp is shared by every database tag verb.
func databaseTagFlagsHelp() string {
	return fmt.Sprintf(`Flags:
  --token string          API token (default: %[1]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}

// runDatabasesTag implements "databases tag <db> <tag>"; the server creates the tag on first use.
func runDatabasesTag(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases tag", "print the attached tag as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases tag <database> <tag> [flags]\n\nAttaches a tag (by name) to a database, creating the tag first if needed.\n\n%s", prog, databaseTagFlagsHelp())
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "databases tag", "exactly two arguments: database name and tag", 2)
	if !ok {
		return exitUsage
	}
	dbName, tagName := rest[0], rest[1]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	attached, err := client.AttachDatabaseTag(context.Background(), dbName, tagName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("tag database %q with %q: %w", dbName, tagName, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, attached, func() {
		_, _ = fmt.Fprintf(stdout, "database %q tagged %q\n", dbName, attached.Name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runDatabasesUntag implements "databases untag <db> <tag>", resolving the tag name to its ID.
func runDatabasesUntag(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases untag", "print {\"detached\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases untag <database> <tag> [flags]\n\nRemoves a tag (by name) from a database.\n\n%s", prog, databaseTagFlagsHelp())
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "databases untag", "exactly two arguments: database name and tag name", 2)
	if !ok {
		return exitUsage
	}
	dbName, tagName := rest[0], rest[1]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	tagID, err := resolveDatabaseTagID(context.Background(), client, dbName, tagName)
	if err == nil {
		err = client.DetachDatabaseTag(context.Background(), dbName, tagID)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("untag database %q from %q: %w", dbName, tagName, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"detached": true}, func() {
		_, _ = fmt.Fprintf(stdout, "database %q untagged %q\n", dbName, tagName)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runDatabasesTags implements "databases tags <db>".
func runDatabasesTags(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases tags", "print tags as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases tags <database> [flags]\n\nLists the tags on one database.\n\n%s", prog, databaseTagFlagsHelp())
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	dbName, ok := requireOneArg(fs, stderr, prog, "databases tags", "database name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	tags, err := client.ListDatabaseTags(context.Background(), dbName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list tags for database %q: %w", dbName, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, tags, func() {
		if len(tags) == 0 {
			_, _ = fmt.Fprintln(stdout, "no tags")
			return
		}
		for _, t := range tags {
			_, _ = fmt.Fprintln(stdout, t.Name)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runTagsDatabases implements "tags databases <tag>": databases the caller can read that carry the tag.
func runTagsDatabases(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "tags databases", "print databases as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s tags databases <name> [flags]\n\nLists every database you can read that carries a tag, identified by name.\n\n%s", prog, databaseTagFlagsHelp())
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "tags databases", "tag name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	id, err := resolveTagIDByName(context.Background(), client, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list databases for tag %q: %w", name, err))
	}
	dbs, err := client.ListDatabasesByTag(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list databases for tag %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, dbs, func() {
		if len(dbs) == 0 {
			_, _ = fmt.Fprintln(stdout, "no databases tagged")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "NAME")
		for _, d := range dbs {
			_, _ = fmt.Fprintln(tw, d.Name)
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// resolveDatabaseTagID reads the database's own tags, so a team-only member never needs the global tag list.
func resolveDatabaseTagID(ctx context.Context, client *Client, dbName, tagName string) (string, error) {
	tags, err := client.ListDatabaseTags(ctx, dbName)
	if err != nil {
		return "", err
	}
	for _, t := range tags {
		if t.Name == tagName {
			return t.ID, nil
		}
	}
	return "", &apiError{StatusCode: http.StatusNotFound, Message: fmt.Sprintf("tag %q is not on database %q", tagName, dbName)}
}
