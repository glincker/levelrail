package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsBuildCacheUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps build-cache show <name>|--global          show the setting, last build outcome and bucket usage
  %[1]s apps build-cache set <name>|--global --target ID [--mode min|max] [--disable]
  %[1]s apps build-cache clear <name>                  delete the app's cached layers from the bucket
  %[1]s apps build-cache remove <name>|--global        stop using a remote cache (bucket contents stay)

Builds import and export BuildKit layers to the storage destination under
build-cache/<app>/. The global setting applies to every app without its own.
A cache problem never fails a build; it is recorded as a warning instead.
"clear" is bounded per call; run it again while it reports more to delete.
%[2]s`, prog, commonFlagsHelp)
}

func runAppsBuildCache(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsBuildCacheUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsBuildCacheUsage(prog))
		return exitOK
	case "show":
		return runBuildCacheShow(prog, rest, stdout, stderr, lookupEnv)
	case "set":
		return runBuildCacheSet(prog, rest, stdout, stderr, lookupEnv)
	case "clear":
		return runBuildCacheClear(prog, rest, stdout, stderr, lookupEnv)
	case "remove":
		return runBuildCacheRemove(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps build-cache subcommand %q\n\n%s", prog, args[0], appsBuildCacheUsage(prog))
		return exitUsage
	}
}

// buildCacheScope reads the app name (or --global) after flag parsing.
func buildCacheScope(c cliCall, global bool, needApp bool) (string, bool) {
	args := c.fs.Args()
	switch {
	case global && len(args) == 0 && !needApp:
		return "", true
	case !global && len(args) == 1:
		return args[0], true
	default:
		what := "exactly one app name or --global"
		if needApp {
			what = "exactly one app name"
		}
		_ = c.invalid("expected %s", what)
		return "", false
	}
}

func runBuildCacheShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var global bool
	c, code, ok := parseCLICall(prog, "apps build-cache show", appsBuildCacheUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&global, "global", false, "show the default that applies to every app")
	})
	if !ok {
		return code
	}
	app, ok := buildCacheScope(c, global, false)
	if !ok {
		return exitValidation
	}
	settings, err := c.client.ListBuildCache(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list build cache settings: %w", err))
	}
	var found *apiclient.BuildCacheSetting
	for i := range settings {
		if settings[i].AppName == app {
			found = &settings[i]
		}
	}
	if found == nil {
		return c.render(map[string]any{"configured": false, "scope": scopeName(app)}, func() {
			_, _ = fmt.Fprintf(stdout, "no build cache setting for %s\n", scopeName(app))
		})
	}
	var stats *apiclient.BuildCacheStats
	if app != "" && found.Enabled {
		if s, err := c.client.GetBuildCacheStats(context.Background(), app); err == nil {
			stats = &s
		}
	}
	return c.render(map[string]any{"setting": found, "bucket": stats}, func() { printBuildCache(stdout, app, *found, stats) })
}

func printBuildCache(w io.Writer, app string, s apiclient.BuildCacheSetting, stats *apiclient.BuildCacheStats) {
	_, _ = fmt.Fprintf(w, "scope:        %s\ntarget:       %s\nmode:         %s\nenabled:      %t\n", scopeName(app), s.TargetID, s.Mode, s.Enabled)
	if s.KeyPrefix != "" {
		_, _ = fmt.Fprintf(w, "key prefix:   %s\n", s.KeyPrefix)
	}
	if s.LastBuildAt != "" {
		_, _ = fmt.Fprintf(w, "last build:   %s (%s)\n", s.LastBuildAt, s.LastResult)
	}
	if s.LastWarning != "" {
		_, _ = fmt.Fprintf(w, "warning:      %s\n", s.LastWarning)
	}
	if s.LastClearedAt != "" {
		_, _ = fmt.Fprintf(w, "last cleared: %s\n", s.LastClearedAt)
	}
	if stats != nil {
		more := ""
		if stats.Truncated {
			more = " or more"
		}
		_, _ = fmt.Fprintf(w, "bucket:       %d%s objects, %d bytes\n", stats.Objects, more, stats.Bytes)
		if !stats.LastModified.IsZero() {
			_, _ = fmt.Fprintf(w, "last export:  %s\n", stats.LastModified.UTC().Format("2006-01-02T15:04:05Z"))
		}
	}
}

func runBuildCacheSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var (
		global  bool
		disable bool
		req     apiclient.BuildCacheRequest
	)
	c, code, ok := parseCLICall(prog, "apps build-cache set", appsBuildCacheUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&global, "global", false, "set the default for every app without its own setting")
		fs.StringVar(&req.TargetID, "target", "", "storage destination id (required)")
		fs.StringVar(&req.Mode, "mode", "max", "cache export mode: min or max")
		fs.BoolVar(&disable, "disable", false, "keep the setting but stop using the cache")
	})
	if !ok {
		return code
	}
	app, ok := buildCacheScope(c, global, false)
	if !ok {
		return exitValidation
	}
	if req.TargetID == "" {
		return c.invalid("--target is required")
	}
	enabled := !disable
	req.AppName, req.Enabled = app, &enabled
	s, err := c.client.SetBuildCache(context.Background(), req)
	if err != nil {
		return c.fail(fmt.Errorf("set build cache: %w", err))
	}
	return c.render(s, func() {
		_, _ = fmt.Fprintf(stdout, "build cache for %s set (target %s, mode %s, enabled %t)\n", scopeName(app), s.TargetID, s.Mode, s.Enabled)
	})
}

func runBuildCacheClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "apps build-cache clear", appsBuildCacheUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	app, ok := buildCacheScope(c, false, true)
	if !ok {
		return exitValidation
	}
	res, err := c.client.ClearBuildCache(context.Background(), app)
	if err != nil {
		return c.fail(fmt.Errorf("clear build cache: %w", err))
	}
	return c.render(res, func() {
		_, _ = fmt.Fprintf(stdout, "deleted %d objects from the build cache of %s\n", res.Deleted, app)
		if res.More {
			_, _ = fmt.Fprintln(stdout, "more objects remain, run the command again")
		}
	})
}

func runBuildCacheRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var global bool
	c, code, ok := parseCLICall(prog, "apps build-cache remove", appsBuildCacheUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&global, "global", false, "remove the default")
	})
	if !ok {
		return code
	}
	app, ok := buildCacheScope(c, global, false)
	if !ok {
		return exitValidation
	}
	if err := c.client.DeleteBuildCache(context.Background(), app); err != nil {
		return c.fail(fmt.Errorf("remove build cache setting: %w", err))
	}
	return c.render(map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "build cache setting for %s removed\n", scopeName(app))
	})
}
