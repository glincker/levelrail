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

func databasesDynamoUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases dynamo tables <name> [--after TABLE] [flags]
  %[1]s databases dynamo query <name> --table T --key-name K --key-value JSON [--index I] [--sort JSON] [--limit N] [--start-key JSON] [flags]
  %[1]s databases dynamo scan <name> --table T --confirm [--limit N] [--start-key JSON] [flags]

Read-only, DynamoDB JSON in and out (for example --key-value '{"S":"user-1"}').
A scan reads every item it passes, so it needs --confirm. Writes and table
changes are not available.
`, prog)
}

func runDatabasesDynamo(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesDynamoUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesDynamoUsage(prog))
		return exitOK
	case "tables":
		return runDynamoTables(prog, rest, stdout, stderr, lookupEnv)
	case "query":
		return runDynamoRead(prog, "query", rest, stdout, stderr, lookupEnv)
	case "scan":
		return runDynamoRead(prog, "scan", rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases dynamo subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesDynamoUsage(prog))
		return exitUsage
	}
}

type dynamoTableRow struct {
	TableName string `json:"TableName"`
	Status    string `json:"TableStatus"`
	Items     int64  `json:"ItemCount"`
	Size      int64  `json:"TableSizeBytes"`
	Keys      []struct {
		Name string `json:"AttributeName"`
		Type string `json:"KeyType"`
	} `json:"KeySchema"`
}

func runDynamoTables(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var after string
	c, code, ok := parseDBAccess(prog, "databases dynamo tables", "databases dynamo tables <name> [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&after, "after", "", "list tables after this name")
	})
	if !ok {
		return code
	}
	res, err := c.client.ListDynamoTables(context.Background(), c.rest[0], after)
	if err != nil {
		return c.fail(fmt.Errorf("list dynamodb tables of database %q: %w", c.rest[0], err))
	}
	return c.result(res, func() {
		if len(res.Names) == 0 {
			_, _ = fmt.Fprintln(stdout, "no tables")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "TABLE\tSTATUS\tITEMS (estimate)\tSIZE BYTES\tKEY")
		for _, raw := range res.Tables {
			var r dynamoTableRow
			if json.Unmarshal(raw, &r) != nil {
				continue
			}
			key := ""
			for _, k := range r.Keys {
				key += safeTerminalText(k.Name) + "(" + k.Type + ") "
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\n", safeTerminalText(r.TableName), safeTerminalText(r.Status), r.Items, r.Size, key)
		}
		_ = tw.Flush()
		if res.Truncated {
			_, _ = fmt.Fprintln(stdout, "(list truncated at the server limit; use --after to continue)")
		}
	})
}

func runDynamoRead(prog, op string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var table, index, keyName, keyValue, sortS, startKey string
	var limit, width int
	var confirm bool
	label := "databases dynamo " + op
	c, code, ok := parseDBAccess(prog, label, label+" <name> --table T [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&table, "table", "", "table name (required)")
		fs.StringVar(&index, "index", "", "secondary index name")
		fs.StringVar(&keyName, "key-name", "", "partition key attribute name (query)")
		fs.StringVar(&keyValue, "key-value", "", "partition key value as DynamoDB JSON (query)")
		fs.StringVar(&sortS, "sort", "", `sort key condition as JSON, for example {"name":"sk","op":"begins_with","value":{"S":"a"}}`)
		fs.IntVar(&limit, "limit", 0, "items per page (0 uses the server default)")
		fs.StringVar(&startKey, "start-key", "", "last_evaluated_key of the previous page")
		fs.BoolVar(&confirm, "confirm", false, "confirm a scan, which reads every item it passes")
		fs.IntVar(&width, "width", 120, "truncate each item to N characters in the table")
	})
	if !ok {
		return code
	}
	if table == "" || limit < 0 {
		return c.fail(errors.New("--table is required and --limit must not be negative"))
	}
	req := apiclient.DynamoReadRequest{Table: table, Index: index, KeyName: keyName, Limit: limit, Confirm: confirm}
	var err error
	if req.KeyValue, err = rawJSONFlag("key-value", keyValue); err != nil {
		return c.fail(err)
	}
	if req.Sort, err = rawJSONFlag("sort", sortS); err != nil {
		return c.fail(err)
	}
	if req.StartKey, err = rawJSONFlag("start-key", startKey); err != nil {
		return c.fail(err)
	}
	res, err := c.client.DynamoRead(context.Background(), c.rest[0], op, req)
	if err != nil {
		return c.fail(fmt.Errorf("dynamodb %s on database %q: %w", op, c.rest[0], err))
	}
	return c.result(res, func() {
		for _, it := range res.Items {
			s := safeTerminalText(string(it))
			if width > 0 && len(s) > width {
				s = s[:width] + "..."
			}
			_, _ = fmt.Fprintln(stdout, s)
		}
		_, _ = fmt.Fprintf(stdout, "%d items, %d scanned\n", res.Count, res.ScannedCount)
		if len(res.LastKey) > 0 {
			_, _ = fmt.Fprintf(stdout, "more: --start-key '%s'\n", safeTerminalText(string(res.LastKey)))
		}
	})
}
