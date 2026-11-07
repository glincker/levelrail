package main

import (
	"context"
	"fmt"
	"io"
)

func appsNextImageCacheUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps next-image-cache set <name> persistent|none

Sets whether a Next.js app keeps its optimised images on a platform volume.
"persistent" mounts a per app volume at the Next image cache path so deploys
and restarts on the same node start warm; "none" keeps them in the container.
The change reaches containers on the next deploy from source.
%[2]s`, prog, commonFlagsHelp)
}

func runAppsNextImageCache(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsNextImageCacheUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsNextImageCacheUsage(prog))
		return exitOK
	case "set":
		return runNextImageCacheSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps next-image-cache subcommand %q\n\n%s", prog, args[0], appsNextImageCacheUsage(prog))
		return exitUsage
	}
}

func runNextImageCacheSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "apps next-image-cache set", appsNextImageCacheUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	rest := c.fs.Args()
	if len(rest) != 2 {
		return c.invalid("expected an app name and a mode: persistent or none")
	}
	name, mode := rest[0], rest[1]
	if mode != "persistent" && mode != "none" {
		return c.invalid("mode must be persistent or none, got %q", mode)
	}
	ctx := context.Background()
	app, err := c.client.GetApp(ctx, name)
	if err != nil {
		return c.fail(fmt.Errorf("get app %q: %w", name, err))
	}
	app.NextImageCache = mode
	updated, err := c.client.UpdateApp(ctx, name, app)
	if err != nil {
		return c.fail(fmt.Errorf("set next.image_cache for app %q: %w", name, err))
	}
	return c.render(updated, func() {
		_, _ = fmt.Fprintf(stdout, "app %q next.image_cache set to %s; redeploy from source to apply\n", name, mode)
	})
}
