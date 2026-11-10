package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// safeTerminalText replaces control and format characters (escape sequences,
// bidi overrides) so engine-supplied names and stored SQL cannot steer a terminal.
func safeTerminalText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return '?'
		}
		return r
	}, s)
}

// oneLine collapses whitespace to single spaces, sanitizes, and cuts to width runes.
func oneLine(s string, width int) string {
	s = safeTerminalText(strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s)), " "))
	rs := []rune(s)
	if width > 3 && len(rs) > width {
		return string(rs[:width-3]) + "..."
	}
	return s
}

func runDatabasesEngineStats(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseDBAccess(prog, "databases engine-stats", "databases engine-stats <name> [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	st, err := c.client.GetDatabaseEngineStats(context.Background(), c.rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("engine stats of database %q: %w", c.rest[0], err))
	}
	return c.result(st, func() { printEngineStats(stdout, st) })
}

func printEngineStats(out io.Writer, st apiclient.DatabaseEngineStats) {
	_, _ = fmt.Fprintf(out, "engine: %s    total: %d\n", safeTerminalText(st.Engine), st.Total)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	if len(st.Objects) > 0 {
		_, _ = fmt.Fprintln(tw, "\nOBJECT\tCOUNT")
		for _, o := range st.Objects {
			_, _ = fmt.Fprintf(tw, "%s\t%d\n", safeTerminalText(o.Name), o.Count)
		}
	}
	if len(st.Metrics) > 0 {
		keys := make([]string, 0, len(st.Metrics))
		for k := range st.Metrics {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		_, _ = fmt.Fprintln(tw, "\nMETRIC\tVALUE")
		for _, k := range keys {
			_, _ = fmt.Fprintf(tw, "%s\t%d\n", safeTerminalText(k), st.Metrics[k])
		}
	}
	_ = tw.Flush()
}
