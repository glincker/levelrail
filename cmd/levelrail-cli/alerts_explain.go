package main

import (
	"context"
	"fmt"
	"io"
)

func explainCommand() apiCmd {
	return apiCmd{
		label: "alerts explain",
		usage: alertsUsage,
		args:  1,
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			ex, err := c.ExplainDeploy(ctx, pos[0])
			return ex, func(w io.Writer) {
				_, _ = fmt.Fprintln(w, ex.Headline)
				if ex.Cause != "" {
					_, _ = fmt.Fprintf(w, "cause: %s\n", ex.Cause)
				}
				if ex.Hint != "" {
					_, _ = fmt.Fprintf(w, "next:  %s\n", ex.Hint)
				}
				for _, l := range ex.LastLogs {
					_, _ = fmt.Fprintf(w, "  | %s\n", l)
				}
			}, err
		},
	}
}
