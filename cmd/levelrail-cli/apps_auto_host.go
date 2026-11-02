package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/GLINCKER/levelrail/internal/apiclient"
	"io"
)

func runAppsAutoHost(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var on, off bool
	label := "apps auto-host"
	usage := fmt.Sprintf("Usage:\n  %s %s <app> [--on | --off] [flags]\n\nShow or change whether the app gets an automatic hostname and the certificate that comes with it. Off saves certificate budget; the app is then reachable only on domains you add.\n\nFlags:\n  --on    give the app an automatic hostname (the default)\n  --off   stop the automatic hostname and its certificate\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&on, "on", false, "give the app an automatic hostname")
		fs.BoolVar(&off, "off", false, "stop the automatic hostname and its certificate")
	})
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	if on && off {
		_, _ = fmt.Fprintf(stderr, "%s %s: choose --on or --off, not both\n", prog, label)
		return exitUsage
	}
	ctx := context.Background()
	var st apiclient.AutoHost
	var err error
	if on || off {
		st, err = c.client.SetAutoHost(ctx, app, on)
	} else {
		st, err = c.client.GetAutoHost(ctx, app)
	}
	if err != nil {
		return c.fail(fmt.Errorf("auto-host for %q: %w", app, err))
	}
	return c.render(st, func() {
		if !st.Enabled {
			_, _ = fmt.Fprintf(stdout, "%s: automatic hostname off, no certificate is ordered for it\n", app)
			return
		}
		host := st.Hostname
		if host == "" {
			host = "none can be formed on this server"
		}
		_, _ = fmt.Fprintf(stdout, "%s: automatic hostname on (%s)\n", app, host)
	})
}
