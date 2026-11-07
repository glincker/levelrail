package main

import (
	"context"
	"fmt"
	"io"
)

func appsNextCacheUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps next-cache set <name> redis|none

Sets where a Next.js app keeps its page and data cache. "redis" shares one cache
across replicas through the app's linked Redis database (REDIS_URL); "none" uses
each replica's own in-process cache. The change reaches containers on the next
restart or deploy.
%[2]s`, prog, commonFlagsHelp)
}

func runAppsNextCache(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsNextCacheUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsNextCacheUsage(prog))
		return exitOK
	case "set":
		return runNextCacheSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps next-cache subcommand %q\n\n%s", prog, args[0], appsNextCacheUsage(prog))
		return exitUsage
	}
}

func runNextCacheSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "apps next-cache set", appsNextCacheUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	rest := c.fs.Args()
	if len(rest) != 2 {
		return c.invalid("expected an app name and a mode: redis or none")
	}
	name, mode := rest[0], rest[1]
	if mode != "redis" && mode != "none" {
		return c.invalid("mode must be redis or none, got %q", mode)
	}
	ctx := context.Background()
	app, err := c.client.GetApp(ctx, name)
	if err != nil {
		return c.fail(fmt.Errorf("get app %q: %w", name, err))
	}
	app.NextCache = mode
	updated, err := c.client.UpdateApp(ctx, name, app)
	if err != nil {
		return c.fail(fmt.Errorf("set next.cache for app %q: %w", name, err))
	}
	return c.render(updated, func() {
		_, _ = fmt.Fprintf(stdout, "app %q next.cache set to %s; restart or redeploy to apply\n", name, mode)
	})
}
