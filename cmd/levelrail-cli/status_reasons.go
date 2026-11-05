package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func statusReasonsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s status-reasons

Lists every reason a deploy can fail its gate or be rolled back, with what it
means and the next step.
`, prog)
}

func runStatusReasons(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cmd := apiCmd{
		label: "status-reasons",
		usage: statusReasonsUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListStatusReasons(ctx)
			return list, func(w io.Writer) { printStatusReasons(w, list) }, err
		},
	}
	return runAPICmd(prog, cmd, args, stdout, stderr, lookupEnv)
}

func printStatusReasons(w io.Writer, list []apiclient.StatusReason) {
	for _, r := range list {
		_, _ = fmt.Fprintf(w, "%s (%s)\n  %s\n  next: %s\n", r.Code, r.Kind, r.Meaning, r.NextStep)
	}
}
