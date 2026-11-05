package main

import (
	"context"
	"fmt"
	"io"
)

func runDoctorReachability(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "doctor reachability"
	cat := doctorStrings()
	usage := cat.render("reach_usage", "prog", prog, "label", label, "flags", commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	r, err := c.client.GetReachability(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("reachability: %w", err))
	}
	return c.render(r, func() {
		for _, p := range r.Ports {
			state := "listening"
			if !p.Listening {
				state = "not listening"
			}
			_, _ = fmt.Fprintf(stdout, "%-9s %5d  %-13s %s %s\n", p.Name, p.Port, state, cat.render("reach_outside"), p.External)
			if p.Advice != "" {
				_, _ = fmt.Fprintf(stdout, "          %s\n", p.Advice)
			}
		}
		if r.Firewall != "" {
			_, _ = fmt.Fprintf(stdout, "%s %s\n", cat.render("reach_firewall"), r.Firewall)
		}
		_, _ = fmt.Fprintln(stdout, r.Message)
	})
}
