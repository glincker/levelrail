package main

import (
	"context"
	"fmt"
	"io"
)

func runDoctorExplain(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "doctor explain"
	usage := fmt.Sprintf("Usage:\n  %s %s <check-id> [flags]\n\nExplains one doctor check in plain words: what is wrong, why it matters and the\nnext step. The check id is the check_id field of the doctor report. The text is a\nfixed table, no network lookup is involved.\n\nFlags:\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	rest := c.fs.Args()
	if len(rest) != 1 {
		return c.invalid("usage: %s %s <check-id>", prog, label)
	}
	e, err := c.client.GetDoctorExplanation(context.Background(), rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("explain: %w", err))
	}
	return c.render(e, func() {
		_, _ = fmt.Fprintf(stdout, "%s\n\nWhy it matters: %s\nNext step: %s\n", e.Plain, e.Why, e.Next)
		if e.FixID != "" {
			_, _ = fmt.Fprintf(stdout, "Fix: %s (%s)\n", e.FixID, e.Kind)
		}
	})
}
