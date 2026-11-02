package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsCDNUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps cdn get <app> [flags]        show the proxy setting, what it resolves to and the SSL mode
  %[1]s apps cdn set <app> [flags]        change the proxy setting for one app
  %[1]s apps cdn resync <app> [flags]     move existing DNS records to the app's current setting

set flags:
  --proxied on|off|inherit      on or off overrides the global setting, inherit clears the override (required)
  --no-resync                   save the setting only, leave existing DNS records as they are

resync flags:
  --dry-run                     list the records that would change and change nothing

A re-sync only touches records Levelrail created for this app, and only when
their proxy flag differs from the setting. A record someone else changed is
left alone. Each record reports its own result, so one failure never hides
the rest. To preview before changing, set with --no-resync, then run resync --dry-run.
%[2]s`, prog, commonFlagsHelp)
}

// runAppsCDN dispatches "apps cdn get|set|resync".
func runAppsCDN(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsCDNUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsCDNUsage(prog))
		return exitOK
	case "get", "set", "resync":
		return runAppsCDNVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown apps cdn subcommand %q\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, appsCDNUsage(prog))
	return exitUsage
}

func runAppsCDNVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps cdn " + verb
	var proxied *string
	var noResync, dryRun *bool
	c, code, ok := parseCLICall(prog, label, appsCDNUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		proxied = fs.String("proxied", "", "on, off or inherit")
		noResync = fs.Bool("no-resync", false, "save the setting without touching existing DNS records")
		dryRun = fs.Bool("dry-run", false, "show what a re-sync would change")
	})
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	ctx := context.Background()
	switch verb {
	case "get":
		cur, err := c.client.GetAppCDN(ctx, app)
		if err != nil {
			return c.fail(fmt.Errorf("read CDN setting for %q: %w", app, err))
		}
		return c.render(cur, func() { printAppCDN(c.stdout, cur) })
	case "resync":
		res, err := c.client.ResyncAppCDN(ctx, app, *dryRun)
		if err != nil {
			return c.fail(fmt.Errorf("re-sync DNS records for %q: %w", app, err))
		}
		return c.finishResync(res)
	}
	val, err := parseProxiedFlag(*proxied)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s %s: %v\n", prog, label, err)
		return exitUsage
	}
	return runAppsCDNSet(ctx, c, app, val, *noResync)
}

func runAppsCDNSet(ctx context.Context, c cliCall, app string, val *bool, noResync bool) int {
	before, err := c.client.GetAppCDN(ctx, app)
	if err != nil {
		return c.fail(fmt.Errorf("read CDN setting for %q: %w", app, err))
	}
	after, err := c.client.SetAppCDN(ctx, app, val)
	if err != nil {
		return c.fail(fmt.Errorf("set CDN setting for %q: %w", app, err))
	}
	if noResync || before.Effective == after.Effective {
		return c.render(after, func() {
			printAppCDN(c.stdout, after)
			if !noResync {
				_, _ = fmt.Fprintln(c.stdout, "the effective setting did not change, so no DNS records were touched")
			}
		})
	}
	res, err := c.client.ResyncAppCDN(ctx, app, false)
	if err != nil {
		return c.fail(fmt.Errorf("setting saved, but the DNS re-sync failed: %w", err))
	}
	return c.finishResync(res)
}

// finishResync prints a re-sync and exits non-zero when any record failed.
func (c cliCall) finishResync(res apiclient.AppCDNResync) int {
	code := c.render(res, func() { printAppCDNResync(c.stdout, res) })
	if code == exitOK && res.Failed > 0 {
		return exitCheckFailed
	}
	return code
}

func parseProxiedFlag(v string) (*bool, error) {
	switch strings.ToLower(v) {
	case "on", "true":
		t := true
		return &t, nil
	case "off", "false":
		f := false
		return &f, nil
	case "inherit":
		return nil, nil
	}
	return nil, fmt.Errorf("--proxied must be on, off or inherit")
}

func printAppCDN(w io.Writer, c apiclient.AppCDN) {
	setting := "inherit (follows the global setting)"
	if c.Proxied != nil {
		setting = cdnOnOff(*c.Proxied)
	}
	_, _ = fmt.Fprintf(w, "app: %s\nproxy override: %s\nglobal: %s\neffective: %s\n", c.App, setting, cdnOnOff(c.GlobalProxied), cdnOnOff(c.Effective))
	if c.SSLMode != "" {
		_, _ = fmt.Fprintf(w, "ssl mode: %s\n", c.SSLMode)
	}
	if c.SSLWarning != "" {
		_, _ = fmt.Fprintf(w, "warning: %s\n", c.SSLWarning)
	}
}

func cdnOnOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func printAppCDNResync(w io.Writer, r apiclient.AppCDNResync) {
	mode := ""
	if r.DryRun {
		mode = " (dry run, nothing changed)"
	}
	_, _ = fmt.Fprintf(w, "proxy is %s for %s%s\n", cdnOnOff(r.Effective), r.App, mode)
	if len(r.Records) == 0 {
		_, _ = fmt.Fprintln(w, "no DNS records created by Levelrail for this app")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DOMAIN\tTYPE\tRESULT\tNOTE")
	for _, rec := range r.Records {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", rec.Domain, rec.Type, rec.Result, rec.Message)
	}
	_ = tw.Flush()
	if !r.DryRun {
		_, _ = fmt.Fprintf(w, "%d updated, %d failed\n", r.Updated, r.Failed)
	}
}
