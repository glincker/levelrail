package main

import (
	"context"
	"fmt"
	"io"
)

func runDoctorReachability(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "doctor reachability"
	usage := fmt.Sprintf("Usage:\n  %s %s [flags]\n\nWhich ports this server listens on, and whether the internet can reach them.\nThe outside check is off until the runtime setting doctor.reachability_dialback_url is set;\nenabling it sends the port list to that URL, which learns this server's public IP.\n\nFlags:\n%s", prog, label, commonFlagsHelp)
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
			_, _ = fmt.Fprintf(stdout, "%-9s %5d  %-13s outside: %s\n", p.Name, p.Port, state, p.External)
			if p.Advice != "" {
				_, _ = fmt.Fprintf(stdout, "          %s\n", p.Advice)
			}
		}
		if r.Firewall != "" {
			_, _ = fmt.Fprintf(stdout, "local firewall: %s\n", r.Firewall)
		}
		_, _ = fmt.Fprintln(stdout, r.Message)
	})
}
