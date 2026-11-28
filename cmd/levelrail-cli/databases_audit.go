package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/glincker/levelrail/internal/apiclient"
)

func databasesAuditUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases audit <database> [--principal ID] [--action db:read|db:query|db:write|db:export] [--outcome O] [--since 24h|RFC3339] [--limit N] [--cursor C] [flags]

Lists the explorer statement audit of one database, newest first. Rows carry a
statement fingerprint, never raw values. Needs root (db:admin).
`, prog)
}

func runDatabasesAudit(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var q apiclient.DatabaseAuditQuery
	var since string
	c, code, ok := parseDBAccess(prog, "databases audit", "databases audit <database> [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&q.Principal, "principal", "", "only this user or token id")
		fs.StringVar(&q.Action, "action", "", "only this action (db:read, db:query, db:write, db:export)")
		fs.StringVar(&q.Outcome, "outcome", "", "only this outcome (ok, denied, rejected, timeout, cancelled, error)")
		fs.StringVar(&since, "since", "", "only rows newer than this duration (24h) or RFC3339 time")
		fs.StringVar(&q.Until, "until", "", "only rows older than this RFC3339 time")
		fs.StringVar(&q.Cursor, "cursor", "", "cursor from a previous page")
		fs.IntVar(&q.Limit, "limit", 0, "rows per page (0 uses the server default)")
	})
	if !ok {
		return code
	}
	if q.Limit < 0 {
		return c.fail(errors.New("--limit must not be negative"))
	}
	if since != "" {
		if d, err := time.ParseDuration(since); err == nil && d > 0 {
			q.Since = time.Now().Add(-d).UTC().Format(time.RFC3339)
		} else if _, err := time.Parse(time.RFC3339, since); err == nil {
			q.Since = since
		} else {
			return c.fail(errors.New("--since must be a duration like 24h or an RFC3339 time"))
		}
	}
	page, err := c.client.ListDatabaseAudit(context.Background(), c.rest[0], q)
	if err != nil {
		return c.fail(fmt.Errorf("list audit of database %q: %w", c.rest[0], err))
	}
	return c.result(page, func() {
		if len(page.Items) == 0 {
			_, _ = fmt.Fprintln(stdout, "no audit rows")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "WHEN\tWHO\tACTION\tKIND\tOUTCOME\tROWS\tMS\tFINGERPRINT")
		for _, e := range page.Items {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\n", safeTerminalText(e.At), safeTerminalText(e.PrincipalName), safeTerminalText(e.Action), safeTerminalText(e.OpKind), safeTerminalText(e.Outcome), e.Rows, e.DurationMs, safeTerminalText(e.Fingerprint))
		}
		_ = tw.Flush()
		if page.NextCursor != "" {
			_, _ = fmt.Fprintf(stdout, "more: --cursor %s\n", page.NextCursor)
		}
	})
}
