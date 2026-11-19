package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const pagesSubmissionsUsage = "Usage:\n  %[1]s pages submissions <site> [--limit N] [flags]\n  %[1]s pages submissions rm <site> <form> <id> [flags]\n"

// runPagesSubmissions lists or removes the form posts a Pages site collected.
func runPagesSubmissions(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	remove := len(args) > 0 && args[0] == "rm"
	if remove {
		args = args[1:]
	}
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages submissions", "print the submissions as JSON to stdout and nothing else", stderr)
	limit := fs.Int("limit", 50, "most recent submissions to show")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	pos := fs.Args()
	if remove {
		id, err := strconv.ParseInt(lastOr(pos, 2), 10, 64)
		if len(pos) != 3 || err != nil {
			_, _ = fmt.Fprintf(stderr, pagesSubmissionsUsage, prog)
			return exitUsage
		}
		if err := client.DeleteStaticSiteSubmission(context.Background(), pos[0], pos[1], id); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete submission %d: %w", id, err))
		}
		_, _ = fmt.Fprintf(stdout, "removed submission %d\n", id)
		return exitOK
	}
	if len(pos) != 1 {
		_, _ = fmt.Fprintf(stderr, pagesSubmissionsUsage, prog)
		return exitUsage
	}
	rows, err := client.ListStaticSiteSubmissions(context.Background(), pos[0], *limit)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list submissions of %q: %w", pos[0], err))
	}
	return renderOrFail(stdout, stderr, of, rows, func() { printSiteSubmissions(stdout, rows) })
}

func lastOr(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

func printSiteSubmissions(out io.Writer, rows []apiclient.SiteSubmission) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "no submissions yet")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tFORM\tWHEN\tSPAM\tFIELDS")
	for _, r := range rows {
		keys := make([]string, 0, len(r.Fields))
		for k := range r.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			v := r.Fields[k]
			parts = append(parts, cleanCell(k)+"="+cleanCell(v))
		}
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%t\t%s\n", r.ID, r.Form, r.CreatedAt.Format("2006-01-02 15:04"), r.Spam, strings.Join(parts, " "))
	}
	_ = tw.Flush()
}

// cleanCell drops control characters (terminal escapes) from visitor input and
// truncates by rune so multi-byte text is never cut mid-character.
func cleanCell(v string) string {
	v = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, v)
	if rs := []rune(v); len(rs) > 40 {
		v = string(rs[:40]) + "..."
	}
	return v
}
