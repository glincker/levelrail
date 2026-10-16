package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

func runAppsURLs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps urls"
	usage := fmt.Sprintf("Usage:\n  %s %s <app> [flags]\n\nWhere an app can be reached right now: container and host port, local and LAN addresses, domains and the internal DNS name.\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, func(*flag.FlagSet) {})
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	res, err := c.client.GetAppURLs(context.Background(), app)
	if err != nil {
		return c.fail(fmt.Errorf("get urls for %q: %w", app, err))
	}
	return c.render(res, func() {
		state := "stopped"
		if res.Running {
			state = "running"
		}
		_, _ = fmt.Fprintf(stdout, "%s: %s, container port %d, host port %d, bind %s\n", res.App, state, res.ContainerPort, res.HostPort, res.BindAddress)
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "KIND\tADDRESS\tSTATE")
		for _, a := range res.Addresses {
			addr := a.URL
			if addr == "" && a.Host != "" {
				addr = fmt.Sprintf("%s:%d", a.Host, a.Port)
			}
			if addr == "" {
				addr = "-"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", a.Kind, addr, a.ReachableHint)
		}
		_ = tw.Flush()
	})
}
