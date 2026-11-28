package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/glincker/levelrail/internal/apiclient"
)

func databasesMongoUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases mongo collections <name> [--database D] [--width N] [flags]
  %[1]s databases mongo find <name> --database D --collection C [--filter JSON] [--sort JSON] [--projection JSON] [--limit N] [--after ID] [--width N] [flags]

Read-only. The server checks every request against its allow-list; filters
with code operators, system databases and system collections are refused.
--after takes the next_after value of the previous page (JSON, or a plain string).
`, prog)
}

func runDatabasesMongo(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesMongoUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesMongoUsage(prog))
		return exitOK
	case "collections":
		return runMongoCollections(prog, rest, stdout, stderr, lookupEnv)
	case "find":
		return runMongoFind(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases mongo subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesMongoUsage(prog))
		return exitUsage
	}
}

type mongoCollectionRow struct {
	Kind        string `json:"kind"`
	DB          string `json:"db"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Count       int64  `json:"count"`
	Size        int64  `json:"size"`
	StorageSize int64  `json:"storageSize"`
	SizeOnDisk  int64  `json:"sizeOnDisk"`
	Indexes     []any  `json:"indexes"`
	Error       string `json:"error"`
}

func runMongoCollections(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var database string
	c, code, ok := parseDBAccess(prog, "databases mongo collections", "databases mongo collections <name> [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&database, "database", "", "limit the listing to one MongoDB database")
	})
	if !ok {
		return code
	}
	res, err := c.client.ListMongoCollections(context.Background(), c.rest[0], database)
	if err != nil {
		return c.fail(fmt.Errorf("list mongo collections of database %q: %w", c.rest[0], err))
	}
	return c.result(res, func() {
		if len(res.Collections) == 0 {
			_, _ = fmt.Fprintln(stdout, "no collections")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "DATABASE\tCOLLECTION\tTYPE\tDOCS\tSIZE\tINDEXES")
		for _, raw := range res.Collections {
			var r mongoCollectionRow
			if json.Unmarshal(raw, &r) != nil {
				continue
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\n", safeTerminalText(r.DB), safeTerminalText(r.Name), safeTerminalText(r.Type), r.Count, r.Size, len(r.Indexes))
		}
		_ = tw.Flush()
		if res.Truncated {
			_, _ = fmt.Fprintln(stdout, "(list truncated at the server limit; pass --database to narrow it)")
		}
	})
}

// rawJSONFlag turns a flag value into a RawMessage; syntax only, the server guard judges content.
func rawJSONFlag(name, v string) (json.RawMessage, error) {
	if v == "" {
		return nil, nil
	}
	if !json.Valid([]byte(v)) {
		return nil, fmt.Errorf("--%s is not valid JSON", name)
	}
	return json.RawMessage(v), nil
}

func runMongoFind(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var database, collection, filter, sortS, proj, after string
	var limit, width int
	c, code, ok := parseDBAccess(prog, "databases mongo find", "databases mongo find <name> --database D --collection C [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&database, "database", "", "MongoDB database (required)")
		fs.StringVar(&collection, "collection", "", "collection (required)")
		fs.StringVar(&filter, "filter", "", "filter as JSON, default all documents")
		fs.StringVar(&sortS, "sort", "", "sort as JSON, default by _id")
		fs.StringVar(&proj, "projection", "", "projection as JSON")
		fs.IntVar(&limit, "limit", 0, "documents per page (0 uses the server default)")
		fs.StringVar(&after, "after", "", "_id of the last document of the previous page")
		fs.IntVar(&width, "width", 120, "truncate each document to N characters in the table")
	})
	if !ok {
		return code
	}
	if database == "" || collection == "" {
		return c.fail(errors.New("--database and --collection are required"))
	}
	if limit < 0 {
		return c.fail(errors.New("--limit must not be negative"))
	}
	req := apiclient.MongoFindRequest{Database: database, Collection: collection, Limit: limit}
	var err error
	if req.Filter, err = rawJSONFlag("filter", filter); err != nil {
		return c.fail(err)
	}
	if req.Sort, err = rawJSONFlag("sort", sortS); err != nil {
		return c.fail(err)
	}
	if req.Projection, err = rawJSONFlag("projection", proj); err != nil {
		return c.fail(err)
	}
	if after != "" {
		if json.Valid([]byte(after)) {
			req.After = json.RawMessage(after)
		} else {
			b, _ := json.Marshal(after)
			req.After = b
		}
	}
	res, err := c.client.MongoFind(context.Background(), c.rest[0], req)
	if err != nil {
		return c.fail(fmt.Errorf("mongo find on database %q: %w", c.rest[0], err))
	}
	return c.result(res, func() {
		if len(res.Documents) == 0 {
			_, _ = fmt.Fprintln(stdout, "no documents")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "#\tDOCUMENT")
		for i, d := range res.Documents {
			_, _ = fmt.Fprintf(tw, "%d\t%s\n", i+1, oneLine(string(d), width))
		}
		_ = tw.Flush()
		if res.Truncated && len(res.NextAfter) > 0 {
			_, _ = fmt.Fprintf(stdout, "more documents: --after '%s'\n", safeTerminalText(string(res.NextAfter)))
		}
	})
}
