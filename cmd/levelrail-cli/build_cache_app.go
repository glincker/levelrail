package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

func appsCacheUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps cache show <name>    size, record count and last use of the app's build cache on the builder
  %[1]s apps cache clear <name>   delete only this app's cache mounts (not the shared cache, not other apps)

Next.js and package caches speed up repeat builds. Clear them when a stale
cache breaks a build. Clearing is refused (409) while a build of the app runs.
For the remote layer cache on a storage destination use "apps build-cache".
%[2]s`, prog, commonFlagsHelp)
}

func runAppsCache(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsCacheUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsCacheUsage(prog))
		return exitOK
	case "show", "clear":
		return runAppsCacheVerb(prog, args[0], rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps cache subcommand %q\n\n%s", prog, args[0], appsCacheUsage(prog))
		return exitUsage
	}
}

func runAppsCacheVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "apps cache "+verb, appsCacheUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	if len(c.fs.Args()) != 1 {
		return c.invalid("expected exactly one app name")
	}
	app := c.fs.Args()[0]
	ctx := context.Background()
	if verb == "clear" {
		res, err := c.client.ClearAppBuildCache(ctx, app)
		if err != nil {
			return c.fail(fmt.Errorf("clear build cache: %w", err))
		}
		return c.render(res, func() {
			_, _ = fmt.Fprintf(stdout, "cleared %s: freed %s, %d records removed\n", app, humanBytes(res.FreedBytes), res.RemovedCount)
		})
	}
	res, err := c.client.GetAppBuildCache(ctx, app)
	if err != nil {
		return c.fail(fmt.Errorf("get build cache: %w", err))
	}
	return c.render(res, func() {
		if res.RecordCount == 0 {
			_, _ = fmt.Fprintf(stdout, "no build cache for %s yet, the first build fills it\n", app)
			return
		}
		used := "never"
		if res.LastUsedAt != nil {
			used = res.LastUsedAt.Format(time.RFC3339)
		}
		_, _ = fmt.Fprintf(stdout, "%s: %s in %d records, last used %s\n", app, humanBytes(res.SizeBytes), res.RecordCount, used)
	})
}
