package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const nullCell = "NULL"

// runDatabasesQuery implements "databases query <name> --sql ...": POST
// /api/v1/databases/{name}/query (read-only by default), /query/write
// with --write, or /explain with --explain.
func runDatabasesQuery(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases query", "print the result as JSON to stdout and nothing else", stderr)
	var sql, file, confirm string
	var write, explain, analyze bool
	fs.StringVar(&sql, "sql", "", "statement to run (or use --file, or pipe it on stdin with --file -)")
	fs.StringVar(&file, "file", "", "read the statement from a file, or - for stdin")
	fs.BoolVar(&write, "write", false, "allow a write statement (admin only, needs --confirm)")
	fs.StringVar(&confirm, "confirm", "", "the database name, required with --write")
	fs.BoolVar(&explain, "explain", false, "show the plan instead of running the statement (Postgres, MySQL, MariaDB)")
	fs.BoolVar(&analyze, "analyze", false, "with --explain, execute the statement to get real timings")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesQueryUsage(prog)) }

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "databases query", "database name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	text, err := resolveQuerySQL(sql, file)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if write && explain {
		return reportError(stdout, stderr, jsonOut, newValidationError("--write and --explain cannot be combined"))
	}
	if write && confirm != name {
		return reportError(stdout, stderr, jsonOut, newValidationError("--write requires --confirm %s", name))
	}

	req := apiclient.DatabaseQueryRequest{SQL: text, Analyze: analyze}
	var result apiclient.DatabaseQueryResult
	switch {
	case explain:
		result, err = client.ExplainDatabaseQuery(context.Background(), name, req)
	default:
		req.Confirm = confirm
		result, err = client.QueryDatabase(context.Background(), name, req, write)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("query database %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printQueryResultHuman(stdout, result) })
}

func resolveQuerySQL(sql, file string) (string, error) {
	switch {
	case sql != "" && file != "":
		return "", newValidationError("use either --sql or --file, not both")
	case sql != "":
		return sql, nil
	case file == "":
		return "", newValidationError("a statement is required: pass --sql or --file")
	case file == "-":
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read statement from stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(file) //nolint:gosec // operator-supplied path to their own statement file
	if err != nil {
		return "", fmt.Errorf("read statement file: %w", err)
	}
	return string(b), nil
}

func printQueryResultHuman(out io.Writer, r apiclient.DatabaseQueryResult) {
	if len(r.Columns) == 0 {
		_, _ = fmt.Fprintf(out, "ok (%d ms)\n", r.DurationMs)
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, strings.Join(r.Columns, "\t"))
	for _, row := range r.Rows {
		cells := make([]string, len(row))
		for i, c := range row {
			if c == nil {
				cells[i] = nullCell
				continue
			}
			cells[i] = strings.NewReplacer("\n", "\\n", "\t", "\\t").Replace(*c)
		}
		_, _ = fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	_ = tw.Flush()
	suffix := ""
	if r.Truncated {
		suffix = ", truncated at the server row limit"
	}
	_, _ = fmt.Fprintf(out, "(%d rows, %d ms%s)\n", r.RowCount, r.DurationMs, suffix)
}

// runDatabasesSchema implements "databases schema <name>": GET
// /api/v1/databases/{name}/schema.
func runDatabasesSchema(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases schema", "print the schema as JSON to stdout and nothing else", stderr)
	var columns bool
	fs.BoolVar(&columns, "columns", false, "also list each table's columns and indexes")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases schema <name> [--columns] [flags]\n\nLists a Postgres, MySQL, or MariaDB database's schemas and tables with\nrow estimates and sizes. Reads need the read:sensitive ability.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "databases schema", "database name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	schema, err := client.GetDatabaseSchema(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get schema for database %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, schema, func() { printSchemaHuman(stdout, schema, columns) })
}

func printSchemaHuman(out io.Writer, s apiclient.DatabaseSchemaResource, columns bool) {
	if len(s.Schemas) == 0 {
		_, _ = fmt.Fprintln(out, "no user tables")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SCHEMA\tNAME\tKIND\tROWS (EST)\tSIZE")
	for _, sc := range s.Schemas {
		for _, t := range sc.Tables {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", sc.Name, t.Name, t.Kind, t.RowEstimate, humanBytes(t.SizeBytes))
		}
	}
	_ = tw.Flush()
	if !columns {
		return
	}
	for _, sc := range s.Schemas {
		for _, t := range sc.Tables {
			_, _ = fmt.Fprintf(out, "\n%s.%s\n", sc.Name, t.Name)
			ctw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			for _, c := range t.Columns {
				flags := ""
				if c.PrimaryKey {
					flags = "PK"
				} else if !c.Nullable {
					flags = "NOT NULL"
				}
				_, _ = fmt.Fprintf(ctw, "  %s\t%s\t%s\n", c.Name, c.Type, flags)
			}
			_ = ctw.Flush()
			for _, ix := range t.Indexes {
				_, _ = fmt.Fprintf(out, "  index %s %s\n", ix.Name, ix.Definition)
			}
		}
	}
}

func databasesQueryUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases query <name> --sql "select ..." [flags]
  %[1]s databases query <name> --file query.sql [flags]

Runs one statement against a Postgres, MySQL, or MariaDB database through
the control plane. Read-only by default: the server runs it in a read-only
transaction with a statement timeout and row limit, and rejects
multi-statement input. Needs the read:sensitive ability.

Flags:
  --sql string       statement to run
  --file string      read the statement from a file, or - for stdin
  --explain          show the plan instead of running the statement
  --analyze          with --explain, execute the statement for real timings
  --write            allow a write statement (admin only)
  --confirm string   the database name, required with --write
  --token, --api-url, --profile, --json, --output, --query  (as every command)
  -h, --help         show this help
`, prog)
}
