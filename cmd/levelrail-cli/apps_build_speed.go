package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const buildSpeedFixAuto = "auto"

func appsBuildSpeedUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps build-speed <name> [--fix]   time saved by build cache and suggestions to speed builds up

--fix applies the suggestions the control plane can apply itself (re-enabling a disabled build cache).
%[2]s`, prog, commonFlagsHelp)
}

func runAppsBuildSpeed(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var fix bool
	c, code, ok := parseCLICall(prog, "apps build-speed", appsBuildSpeedUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&fix, "fix", false, "apply the fixes the control plane can apply itself")
	})
	if !ok {
		return code
	}
	if len(c.fs.Args()) != 1 {
		return c.invalid("expected exactly one app name")
	}
	app := c.fs.Args()[0]
	ctx := context.Background()
	s, err := c.client.GetBuildSpeed(ctx, app)
	if err != nil {
		return c.fail(fmt.Errorf("get build speed: %w", err))
	}
	if fix {
		enable := true
		for _, f := range s.Findings {
			if f.FixKind != buildSpeedFixAuto || f.TargetID == "" {
				continue
			}
			if _, err := c.client.SetBuildCache(ctx, apiclient.BuildCacheRequest{AppName: app, TargetID: f.TargetID, Enabled: &enable}); err != nil {
				return c.fail(fmt.Errorf("apply %s: %w", f.Code, err))
			}
		}
		if s, err = c.client.GetBuildSpeed(ctx, app); err != nil {
			return c.fail(fmt.Errorf("get build speed: %w", err))
		}
	}
	return c.render(s, func() { printBuildSpeed(stdout, app, s) })
}

func printBuildSpeed(w io.Writer, app string, s apiclient.BuildSpeed) {
	if s.Builds == 0 {
		_, _ = fmt.Fprintf(w, "no builds recorded for %s yet\n", app)
		return
	}
	if s.WarmKnown {
		_, _ = fmt.Fprintf(w, "cold build p50 %s, cached build p50 %s, about %s saved by cache\n",
			fmtSeconds(s.ColdP50Seconds), fmtSeconds(s.WarmP50Seconds), fmtSeconds(s.TimeSavedSeconds))
	}
	if len(s.Findings) == 0 {
		_, _ = fmt.Fprintln(w, "no speedups to suggest")
		return
	}
	for _, f := range s.Findings {
		_, _ = fmt.Fprintf(w, "%-8s %-20s fix: %s\n", f.Severity, f.Code, f.FixKind)
	}
}
