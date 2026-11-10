package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsBuildSettingsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps build-settings show <name>
  %[1]s apps build-settings set <name> [--builder TYPE] [--dockerfile PATH] [--target STAGE] [--no-cache=true|false] [--provider NAME]

Settings persist and apply to every later build of the app; saving them
does not start a deploy. Builder and Dockerfile path need a git source.
--target picks a multi-stage Dockerfile stage, --no-cache skips the layer
cache on Dockerfile builds, --provider pins the Railpack provider (a build
that detects another one fails). Pass an empty value to clear a field.
%[2]s`, prog, commonFlagsHelp)
}

func runAppsBuildSettings(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsBuildSettingsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsBuildSettingsUsage(prog))
		return exitOK
	case "show":
		return runBuildSettingsShow(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runBuildSettingsSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps build-settings subcommand %q\n\n%s", prog, args[0], appsBuildSettingsUsage(prog))
		return exitUsage
	}
}

func runBuildSettingsShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "apps build-settings show", appsBuildSettingsUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	if len(c.fs.Args()) != 1 {
		return c.invalid("expected exactly one app name")
	}
	s, err := c.client.GetAppBuildSettings(context.Background(), c.fs.Arg(0))
	if err != nil {
		return c.fail(fmt.Errorf("get build settings: %w", err))
	}
	return c.render(s, func() { printBuildSettings(stdout, s) })
}

func runBuildSettingsSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var (
		builder, dockerfile, target, provider string
		noCache                               bool
	)
	c, code, ok := parseCLICall(prog, "apps build-settings set", appsBuildSettingsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&builder, "builder", "", "build type: dockerfile, railpack or static")
		fs.StringVar(&dockerfile, "dockerfile", "", "Dockerfile path relative to the repository root")
		fs.StringVar(&target, "target", "", "multi-stage Dockerfile target stage")
		fs.BoolVar(&noCache, "no-cache", false, "skip the BuildKit layer cache on Dockerfile builds")
		fs.StringVar(&provider, "provider", "", "Railpack provider the build must detect")
	})
	if !ok {
		return code
	}
	if len(c.fs.Args()) != 1 {
		return c.invalid("expected exactly one app name")
	}
	var req apiclient.SetAppBuildSettingsRequest
	c.fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "builder":
			req.Builder = &builder
		case "dockerfile":
			req.DockerfilePath = &dockerfile
		case "target":
			req.Target = &target
		case "no-cache":
			req.NoCache = &noCache
		case "provider":
			req.Provider = &provider
		}
	})
	if req == (apiclient.SetAppBuildSettingsRequest{}) {
		return c.invalid("set at least one of --builder, --dockerfile, --target, --no-cache, --provider")
	}
	s, err := c.client.SetAppBuildSettings(context.Background(), c.fs.Arg(0), req)
	if err != nil {
		return c.fail(fmt.Errorf("set build settings: %w", err))
	}
	return c.render(s, func() {
		_, _ = fmt.Fprintln(stdout, "build settings saved; the next build uses them")
		printBuildSettings(stdout, s)
	})
}

func printBuildSettings(w io.Writer, s apiclient.AppBuildSettings) {
	show := func(v string) string {
		if v == "" {
			return "(default)"
		}
		return v
	}
	_, _ = fmt.Fprintf(w, "builder:     %s\ndockerfile:  %s\ntarget:      %s\nno cache:    %t\nprovider:    %s\n",
		show(s.Builder), show(s.DockerfilePath), show(s.Target), s.NoCache, show(s.Provider))
}
