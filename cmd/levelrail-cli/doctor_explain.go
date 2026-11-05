package main

import (
	"context"
	"fmt"
	"io"
)

func runDoctorExplain(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "doctor explain"
	cat := doctorStrings()
	usage := cat.render("explain_usage", "prog", prog, "label", label, "flags", commonFlagsHelp)
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
		_, _ = fmt.Fprintf(stdout, "%s\n\n%s %s\n%s %s\n", e.Plain, cat.render("explain_why"), e.Why, cat.render("explain_next"), e.Next)
		if e.FixID != "" {
			_, _ = fmt.Fprintf(stdout, "%s %s (%s)\n", cat.render("explain_fix"), e.FixID, e.Kind)
		}
	})
}
