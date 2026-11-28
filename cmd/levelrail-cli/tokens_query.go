package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// tokenQueryFlags are the list filters; none set keeps the plain array response.
type tokenQueryFlags struct {
	status, name, q, ability, kind, sort, order *string
	limit                                       *int
	groups                                      bool
	groupsP                                     *bool
}

func bindTokenQueryFlags(fs *flag.FlagSet) *tokenQueryFlags {
	f := &tokenQueryFlags{
		status:  fs.String("status", "", "active, expiring, expired or revoked"),
		name:    fs.String("name", "", "exact token name"),
		q:       fs.String("q", "", "name contains this text"),
		ability: fs.String("ability", "", "holds this ability"),
		kind:    fs.String("kind", "", "emergency, agent, system or user"),
		sort:    fs.String("sort", "", "name, created, last_used or expires"),
		order:   fs.String("order", "", "asc or desc"),
		limit:   fs.Int("limit", 0, "most rows to print"),
		groupsP: fs.Bool("groups", false, "one row per name with counts"),
	}
	return f
}

// encode builds the query string, empty when no filter flag was given.
func (f *tokenQueryFlags) encode() (string, error) {
	f.groups = *f.groupsP
	if *f.limit < 0 {
		return "", fmt.Errorf("--limit must be 1 or more")
	}
	v := url.Values{}
	for k, p := range map[string]*string{"status": f.status, "name": f.name, "q": f.q, "ability": f.ability, "kind": f.kind, "sort": f.sort, "order": f.order} {
		if *p != "" {
			v.Set(k, *p)
		}
	}
	if *f.limit > 0 {
		v.Set("limit", strconv.Itoa(*f.limit))
	}
	if f.groups {
		v.Set("groups", "name")
	}
	return v.Encode(), nil
}

type tokenPageResponse struct {
	Items          []tokenResource `json:"items"`
	Groups         []tokenGroupRow `json:"groups"`
	Total          int             `json:"total"`
	CountsByStatus map[string]int  `json:"counts_by_status"`
	NextCursor     string          `json:"next_cursor,omitempty"`
}

type tokenGroupRow struct {
	Name             string     `json:"name"`
	Count            int        `json:"count"`
	Active           int        `json:"active"`
	Expiring         int        `json:"expiring"`
	Expired          int        `json:"expired"`
	OldestCreated    time.Time  `json:"oldest_created"`
	NewestLastUsedAt *time.Time `json:"newest_last_used,omitempty"`
}

func printTokenPage(stdout, stderr io.Writer, of outputFlags, groups bool, fetch func() (json.RawMessage, error)) int {
	raw, err := fetch()
	if err != nil {
		return reportError(stdout, stderr, of.Format == "json", fmt.Errorf("list tokens: %w", err))
	}
	var page tokenPageResponse
	if err := json.Unmarshal(raw, &page); err != nil {
		return reportError(stdout, stderr, of.Format == "json", fmt.Errorf("list tokens: decode: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, page, func() {
		if groups {
			printTokenGroups(stdout, page)
			return
		}
		printTokenRows(stdout, page)
	})
}

func tokenDay(t *time.Time) string {
	if t == nil {
		return "never"
	}
	return t.Local().Format("2006-01-02")
}

func printTokenRows(out io.Writer, page tokenPageResponse) {
	if len(page.Items) == 0 {
		_, _ = fmt.Fprintln(out, "no tokens match")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tSTATUS\tNAME\tABILITIES\tCREATED\tLAST USED\tEXPIRES")
	for _, t := range page.Items {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", t.ID, t.Status, t.Name, strings.Join(t.Abilities, ","), t.CreatedAt.Local().Format("2006-01-02"), tokenDay(t.LastUsedAt), tokenDay(t.ExpiresAt))
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(out, "%d of %d tokens%s\n", len(page.Items), page.Total, tokenCountsLine(page.CountsByStatus))
}

func printTokenGroups(out io.Writer, page tokenPageResponse) {
	if len(page.Groups) == 0 {
		_, _ = fmt.Fprintln(out, "no tokens match")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tCOUNT\tACTIVE\tEXPIRING\tEXPIRED\tOLDEST\tLAST USED")
	for _, g := range page.Groups {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%s\t%s\n", g.Name, g.Count, g.Active, g.Expiring, g.Expired, g.OldestCreated.Local().Format("2006-01-02"), tokenDay(g.NewestLastUsedAt))
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(out, "%d tokens%s\n", page.Total, tokenCountsLine(page.CountsByStatus))
}

func tokenCountsLine(c map[string]int) string {
	if len(c) == 0 {
		return ""
	}
	return fmt.Sprintf(" (active %d, expiring %d, expired %d, revoked %d)", c["active"], c["expiring"], c["expired"], c["revoked"])
}
