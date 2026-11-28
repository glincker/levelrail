package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/glincker/levelrail/internal/apiclient"
)

func runDatabasesExplorer(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] != "capabilities" {
		_, _ = fmt.Fprintf(stderr, "usage: %s databases explorer capabilities <name> [--json]\n", prog)
		return 2
	}
	c, code, ok := parseDBAccess(prog, "databases explorer capabilities", "databases explorer capabilities <name> [flags]", args[1:], 1, "a database name", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	caps, err := c.client.GetDatabaseExplorerCapabilities(context.Background(), c.rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("explorer capabilities of database %q: %w", c.rest[0], err))
	}
	return c.result(caps, func() { printExplorerCapabilities(stdout, caps) })
}

func printExplorerCapabilities(out io.Writer, caps apiclient.DatabaseExplorerCapabilities) {
	_, _ = fmt.Fprintf(out, "engine: %s    kind: %s    planned: %t\n", safeTerminalText(caps.Engine), safeTerminalText(caps.Kind), caps.Planned)
	keys := make([]string, 0, len(caps.Capabilities))
	for k := range caps.Capabilities {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "\nCAPABILITY\tSTATUS\tREASON")
	for _, k := range keys {
		v := caps.Capabilities[k]
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", safeTerminalText(k), safeTerminalText(v.Status), safeTerminalText(v.Reason))
	}
	_ = tw.Flush()
}
