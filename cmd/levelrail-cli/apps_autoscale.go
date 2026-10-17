package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const noAutoscaleMessage = "no autoscale policy is set for this app"

func appsAutoscaleUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps autoscale show <app-name> [flags]      show the policy, the replica count and the last decision
  %[1]s apps autoscale set <app-name> [flags]       create or change the policy
  %[1]s apps autoscale clear <app-name> [flags]     remove the policy; replicas stay where they are
  %[1]s apps autoscale history <app-name> [flags]   list what the autoscaler decided and why

set flags:
  --min N                     fewest replicas (default 1)
  --max N                     most replicas (required for a new policy)
  --metric NAME               cpu_percent, memory_percent or requests_per_replica (required for a new policy)
  --target N                  value to hold the metric at (required for a new policy)
  --scale-up-tolerance PCT    scale up only above target plus this percent (default 10)
  --scale-down-tolerance PCT  scale down only below target minus this percent (default 30)
  --scale-up-cooldown D       wait between scale ups (default 1m)
  --scale-down-cooldown D     wait after any change before scaling down (default 10m)
  --stabilization D           a lower count must hold this long before it is applied (default 5m)
  --max-step-up N             most replicas added at once (default 4)
  --max-step-down N           most replicas removed at once (default 1)

While a policy is set the autoscaler owns the replica count. Manual replica
edits are refused; clear the policy first to set replicas by hand.
`, prog)
}

// runAppsAutoscale dispatches "apps autoscale show|set|clear|history".
func runAppsAutoscale(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsAutoscaleUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsAutoscaleUsage(prog))
		return exitOK
	case "show", "set", "clear", "history":
		return runAppsAutoscaleVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown apps autoscale subcommand %q\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, appsAutoscaleUsage(prog))
	return exitUsage
}

type autoscaleFlags struct {
	min, max, upTol, downTol, stepUp, stepDown *int
	metric                                     *string
	target                                     *float64
	upCool, downCool, stabilize                *time.Duration
}

func bindAutoscaleFlags(fs *flag.FlagSet) autoscaleFlags {
	return autoscaleFlags{
		min:       fs.Int("min", 0, "fewest replicas (set)"),
		max:       fs.Int("max", 0, "most replicas (set)"),
		metric:    fs.String("metric", "", "cpu_percent, memory_percent or requests_per_replica (set)"),
		target:    fs.Float64("target", 0, "value to hold the metric at (set)"),
		upTol:     fs.Int("scale-up-tolerance", 0, "scale up only above target plus this percent (set)"),
		downTol:   fs.Int("scale-down-tolerance", 0, "scale down only below target minus this percent (set)"),
		upCool:    fs.Duration("scale-up-cooldown", 0, "wait between scale ups (set)"),
		downCool:  fs.Duration("scale-down-cooldown", 0, "wait after any change before scaling down (set)"),
		stabilize: fs.Duration("stabilization", 0, "a lower count must hold this long before it is applied (set)"),
		stepUp:    fs.Int("max-step-up", 0, "most replicas added at once (set)"),
		stepDown:  fs.Int("max-step-down", 0, "most replicas removed at once (set)"),
	}
}

func runAppsAutoscaleVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "apps autoscale "+verb, "print the result as JSON to stdout and nothing else", stderr)
	flags := bindAutoscaleFlags(fs)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	app, ok := requireOneArg(fs, stderr, prog, "apps autoscale "+verb, "app name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	switch verb {
	case "clear":
		if err := client.ClearAppAutoscale(ctx, app); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear autoscale: %w", err))
		}
		_, _ = fmt.Fprintf(stdout, "removed the autoscale policy of %s; its replicas stay where they are\n", app)
		return exitOK
	case "set":
		upd, err := buildAutoscaleUpdate(ctx, client, app, fs, flags)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitUsage
		}
		st, err := client.SetAppAutoscale(ctx, app, upd)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set autoscale: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, st, func() { printAppAutoscale(stdout, st) })
	case "history":
		h, err := client.GetAppAutoscaleHistory(ctx, app)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("autoscale history: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, h, func() { printAutoscaleHistory(stdout, h) })
	}
	st, err := client.GetAppAutoscale(ctx, app)
	var apiErr *apiclient.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound && apiErr.Message == noAutoscaleMessage {
		_, _ = fmt.Fprintf(stdout, "%s has no autoscale policy. Set one with: %s apps autoscale set %s --max 5 --metric cpu_percent --target 60\n", app, prog, app)
		return exitOK
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("show autoscale: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, st, func() { printAppAutoscale(stdout, st) })
}

// buildAutoscaleUpdate starts from the app's current policy so a flag changes
// only its own field.
func buildAutoscaleUpdate(ctx context.Context, client *Client, app string, fs *flag.FlagSet, f autoscaleFlags) (apiclient.AppAutoscaleUpdate, error) {
	var upd apiclient.AppAutoscaleUpdate
	cur, err := client.GetAppAutoscale(ctx, app)
	var apiErr *apiclient.APIError
	switch {
	case err == nil:
		upd = apiclient.AppAutoscaleUpdate{
			MinReplicas: cur.MinReplicas, MaxReplicas: cur.MaxReplicas, Metric: cur.Metric, Target: cur.Target,
			ScaleUpTolerancePct: cur.ScaleUpTolerancePct, ScaleDownTolerancePct: cur.ScaleDownTolerancePct,
			ScaleUpCooldownSecs: cur.ScaleUpCooldownSecs, ScaleDownCooldownSecs: cur.ScaleDownCooldownSecs,
			StabilizationSecs: cur.StabilizationSecs, MaxStepUp: cur.MaxStepUp, MaxStepDown: cur.MaxStepDown,
		}
	case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound:
		upd.MinReplicas = 1
	default:
		return upd, fmt.Errorf("read current policy: %w", err)
	}
	given := map[string]bool{}
	fs.Visit(func(fl *flag.Flag) { given[fl.Name] = true })
	setInt := func(name string, dst *int, v int) {
		if given[name] {
			*dst = v
		}
	}
	setInt("min", &upd.MinReplicas, *f.min)
	setInt("max", &upd.MaxReplicas, *f.max)
	setInt("scale-up-tolerance", &upd.ScaleUpTolerancePct, *f.upTol)
	setInt("scale-down-tolerance", &upd.ScaleDownTolerancePct, *f.downTol)
	setInt("max-step-up", &upd.MaxStepUp, *f.stepUp)
	setInt("max-step-down", &upd.MaxStepDown, *f.stepDown)
	setInt("scale-up-cooldown", &upd.ScaleUpCooldownSecs, int(*f.upCool/time.Second))
	setInt("scale-down-cooldown", &upd.ScaleDownCooldownSecs, int(*f.downCool/time.Second))
	setInt("stabilization", &upd.StabilizationSecs, int(*f.stabilize/time.Second))
	if given["metric"] {
		upd.Metric = *f.metric
	}
	if given["target"] {
		upd.Target = *f.target
	}
	if upd.MaxReplicas == 0 || upd.Metric == "" || upd.Target == 0 {
		return upd, errors.New("a new policy needs --max, --metric and --target")
	}
	return upd, nil
}

func printAppAutoscale(out io.Writer, s apiclient.AppAutoscale) {
	_, _ = fmt.Fprintf(out, "%s: %d to %d replicas on %s, target %g\n", s.App, s.MinReplicas, s.MaxReplicas, s.Metric, s.Target)
	if s.EffectiveMax < s.MaxReplicas {
		_, _ = fmt.Fprintf(out, "  limited to  %d by the operator's global maximum\n", s.EffectiveMax)
	}
	_, _ = fmt.Fprintf(out, "  replicas    %d now, %d desired\n", s.Current, s.Desired)
	if s.ValueKnown {
		_, _ = fmt.Fprintf(out, "  metric      %.1f\n", s.Value)
	} else {
		_, _ = fmt.Fprintln(out, "  metric      unknown")
	}
	_, _ = fmt.Fprintf(out, "  band        up above +%d%%, down below -%d%%, step +%d/-%d\n", s.ScaleUpTolerancePct, s.ScaleDownTolerancePct, s.MaxStepUp, s.MaxStepDown)
	_, _ = fmt.Fprintf(out, "  waits       up %s, down %s, stabilize %s\n",
		time.Duration(s.ScaleUpCooldownSecs)*time.Second, time.Duration(s.ScaleDownCooldownSecs)*time.Second, time.Duration(s.StabilizationSecs)*time.Second)
	if s.BlockedBy != "" {
		_, _ = fmt.Fprintf(out, "  blocked by  %s\n", s.BlockedBy)
	}
	_, _ = fmt.Fprintf(out, "  status      %s: %s\n", s.Condition.Reason, s.Condition.Message)
}

func printAutoscaleHistory(out io.Writer, h apiclient.AppAutoscaleHistory) {
	if len(h.Events) == 0 {
		_, _ = fmt.Fprintln(out, "no history yet")
		return
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "WHEN\tEVENT\tREPLICAS\tREASON\tBLOCKED BY\tDETAIL")
	for _, e := range h.Events {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d to %d\t%s\t%s\t%s\n", e.At.Format("2006-01-02 15:04"), e.Kind, e.Current, e.Desired, e.Reason, strings.TrimSpace(e.BlockedBy), e.Detail)
	}
	_ = w.Flush()
}
