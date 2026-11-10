package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// savedQueryMaxBytes caps a saved statement on the client so an oversize file
// is refused before any request; the server enforces its own, tighter limit.
const savedQueryMaxBytes = 64 << 10

func databasesQueryHistoryUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases query-history list <database> [--limit N] [--width N] [flags]  your recent statements
  %[1]s databases query-history clear <database> --yes [flags]                 delete your history for this database

Stored statements can contain literals. Tables show one truncated line per
statement; --json prints them in full.
`, prog)
}

func databasesSavedQueriesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases saved-queries list <database> [--width N] [flags]
  %[1]s databases saved-queries add <database> --name N (--sql "..." | --sql-file F) [flags]
  %[1]s databases saved-queries delete <database> <id> --yes [flags]

Saved queries belong to the calling user. A statement may be at most 64 KiB.
`, prog)
}

func runDatabasesQueryHistory(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesQueryHistoryUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesQueryHistoryUsage(prog))
		return exitOK
	case "list":
		return runQueryHistoryList(prog, rest, stdout, stderr, lookupEnv)
	case "clear":
		return runQueryHistoryClear(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases query-history subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesQueryHistoryUsage(prog))
		return exitUsage
	}
}

func runQueryHistoryList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var limit, width int
	c, code, ok := parseDBAccess(prog, "databases query-history list", "databases query-history list <database> [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.IntVar(&limit, "limit", 0, "most recent N entries (0 uses the server default)")
		fs.IntVar(&width, "width", 80, "truncate each statement to N characters in the table")
	})
	if !ok {
		return code
	}
	if limit < 0 {
		return c.fail(errors.New("--limit must not be negative"))
	}
	rows, err := c.client.ListDatabaseQueryHistory(context.Background(), c.rest[0], limit)
	if err != nil {
		return c.fail(fmt.Errorf("list query history of database %q: %w", c.rest[0], err))
	}
	return c.result(rows, func() {
		if len(rows) == 0 {
			_, _ = fmt.Fprintln(stdout, "no query history")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "WHEN\tMODE\tOK\tROWS\tMS\tSQL")
		for _, h := range rows {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%v\t%d\t%d\t%s\n", safeTerminalText(h.CreatedAt), safeTerminalText(h.Mode), h.OK, h.RowCount, h.DurationMs, oneLine(h.SQL, width))
		}
		_ = tw.Flush()
	})
}

func runQueryHistoryClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var yes bool
	c, code, ok := parseDBAccess(prog, "databases query-history clear", "databases query-history clear <database> --yes [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&yes, "yes", false, "delete the history; without it nothing is sent")
	})
	if !ok {
		return code
	}
	name := c.rest[0]
	if !yes {
		_, _ = fmt.Fprintf(stdout, "would delete your whole query history for database %q; add --yes to proceed\n", safeTerminalText(name))
		return exitOK
	}
	if err := c.client.ClearDatabaseQueryHistory(context.Background(), name); err != nil {
		return c.fail(fmt.Errorf("clear query history of database %q: %w", name, err))
	}
	return c.result(map[string]any{"cleared": true}, func() { _, _ = fmt.Fprintln(stdout, "query history cleared") })
}

func runDatabasesSavedQueries(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesSavedQueriesUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesSavedQueriesUsage(prog))
		return exitOK
	case "list":
		return runSavedQueriesList(prog, rest, stdout, stderr, lookupEnv)
	case "add":
		return runSavedQueriesAdd(prog, rest, stdout, stderr, lookupEnv)
	case "delete":
		return runSavedQueriesDelete(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases saved-queries subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesSavedQueriesUsage(prog))
		return exitUsage
	}
}

func runSavedQueriesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var width int
	c, code, ok := parseDBAccess(prog, "databases saved-queries list", "databases saved-queries list <database> [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.IntVar(&width, "width", 80, "truncate each statement to N characters in the table")
	})
	if !ok {
		return code
	}
	rows, err := c.client.ListDatabaseSavedQueries(context.Background(), c.rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("list saved queries of database %q: %w", c.rest[0], err))
	}
	return c.result(rows, func() {
		if len(rows) == 0 {
			_, _ = fmt.Fprintln(stdout, "no saved queries")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tNAME\tSQL")
		for _, q := range rows {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", safeTerminalText(q.ID), oneLine(q.Name, 40), oneLine(q.SQL, width))
		}
		_ = tw.Flush()
	})
}

func runSavedQueriesAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var title, sql, file string
	c, code, ok := parseDBAccess(prog, "databases saved-queries add", "databases saved-queries add <database> --name N (--sql \"...\" | --sql-file F) [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&title, "name", "", "name for the saved statement (1 to 120 characters)")
		fs.StringVar(&sql, "sql", "", "the statement text")
		fs.StringVar(&file, "sql-file", "", "read the statement from a file (up to 64 KiB)")
	})
	if !ok {
		return code
	}
	text, err := resolveSavedSQL(sql, file)
	if err != nil {
		return c.fail(err)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return c.fail(errors.New("--name is required"))
	}
	q, err := c.client.SaveDatabaseQuery(context.Background(), c.rest[0], title, text)
	if err != nil {
		return c.fail(fmt.Errorf("save query on database %q: %w", c.rest[0], err))
	}
	return c.result(q, func() { _, _ = fmt.Fprintf(stdout, "saved %s (id %s)\n", oneLine(q.Name, 60), safeTerminalText(q.ID)) })
}

func resolveSavedSQL(sql, file string) (string, error) {
	if (sql == "") == (file == "") {
		return "", errors.New("pass exactly one of --sql or --sql-file")
	}
	if file != "" {
		f, err := os.Open(file) //nolint:gosec // operator-supplied path to their own statement file
		if err != nil {
			return "", fmt.Errorf("read --sql-file: %w", err)
		}
		defer func() { _ = f.Close() }()
		b, err := io.ReadAll(io.LimitReader(f, savedQueryMaxBytes+1))
		if err != nil {
			return "", fmt.Errorf("read --sql-file: %w", err)
		}
		sql = string(b)
	}
	if len(sql) > savedQueryMaxBytes {
		return "", fmt.Errorf("the statement is larger than %d KiB", savedQueryMaxBytes>>10)
	}
	if strings.TrimSpace(sql) == "" {
		return "", errors.New("the statement is empty")
	}
	return sql, nil
}

func runSavedQueriesDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var yes bool
	c, code, ok := parseDBAccess(prog, "databases saved-queries delete", "databases saved-queries delete <database> <id> --yes [flags]", args, 2, "a database name and a saved query id", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&yes, "yes", false, "delete the saved query; without it nothing is sent")
	})
	if !ok {
		return code
	}
	name, id := c.rest[0], c.rest[1]
	if !yes {
		_, _ = fmt.Fprintf(stdout, "would delete saved query %q from database %q; add --yes to proceed\n", safeTerminalText(id), safeTerminalText(name))
		return exitOK
	}
	if err := c.client.DeleteDatabaseSavedQuery(context.Background(), name, id); err != nil {
		return c.fail(fmt.Errorf("delete saved query %q on database %q: %w", id, name, err))
	}
	return c.result(map[string]any{"deleted": true}, func() { _, _ = fmt.Fprintln(stdout, "saved query deleted") })
}
