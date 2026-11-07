// Command levelrail-stress boots its own control plane on a scratch data dir,
// loads it with a seeded estate and a fake agent fleet, and judges the run
// against the budgets in a stress profile. Dev tooling only.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/stressprofile"
)

type options struct {
	profile  string
	report   string
	cpBin    string
	repoRoot string
	duration time.Duration
	keep     bool
}

func main() { os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr)) }

func realMain(args []string, stdout, stderr io.Writer) int {
	var o options
	fs := flag.NewFlagSet("levelrail-stress", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.profile, "profile", "", "profile YAML (default: $"+stressprofile.EnvProfile+")")
	fs.StringVar(&o.report, "report", "stress-report.json", "path for the JSON report")
	fs.StringVar(&o.cpBin, "cp-bin", "", "control plane binary (default: build from -repo)")
	fs.StringVar(&o.repoRoot, "repo", ".", "repo root used to build the control plane")
	fs.DurationVar(&o.duration, "duration", 0, "override the profile duration")
	fs.BoolVar(&o.keep, "keep", false, "keep the scratch dir")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	prof, err := stressprofile.Load(o.profile)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "levelrail-stress:", err)
		return 2
	}
	if o.duration > 0 {
		prof.Duration = o.duration
		if err := prof.Validate(); err != nil {
			_, _ = fmt.Fprintln(stderr, "levelrail-stress:", err)
			return 2
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rep, err := execute(ctx, prof, o, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "levelrail-stress:", err)
		if rep == nil {
			return 1
		}
	}
	if data, merr := json.MarshalIndent(rep, "", "  "); merr == nil {
		if werr := os.WriteFile(o.report, data, 0o600); werr != nil {
			_, _ = fmt.Fprintln(stderr, "levelrail-stress: write report:", werr)
		}
	}
	rep.summary(stdout)
	if err != nil || !rep.Pass {
		return 1
	}
	return 0
}
