package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/spendcap"
)

// runBudget serves `budget show|clear APP` and `budget set APP`; the older
// project and team budget verbs fall through to runUsageBudgets.
func runBudget(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 {
		switch args[0] {
		case "show", "clear":
			return runAppBudget(prog, args[0], args[1:], stdout, stderr, lookupEnv)
		}
		if args[0] == "set" && isAppBudgetSet(args[1:]) {
			return runAppBudget(prog, "set", args[1:], stdout, stderr, lookupEnv)
		}
	}
	return runUsageBudgets(prog, args, stdout, stderr, lookupEnv)
}

func isAppBudgetSet(args []string) bool {
	if len(args) > 0 && (args[0] == "project" || args[0] == "team") {
		return false
	}
	for _, a := range args {
		if a == "--monthly" || a == "-monthly" || strings.HasPrefix(a, "--monthly=") || strings.HasPrefix(a, "-monthly=") {
			return false
		}
	}
	return true
}

func runAppBudget(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "budget "+verb, "print the result as JSON to stdout and nothing else", stderr)
	egress := fs.String("egress", "", "monthly egress limit such as 500GiB or 2TB (set)")
	cost := fs.Float64("cost", 0, "monthly cost ceiling in the configured currency (set)")
	action := fs.String("action", "", "what happens at 100%: none, throttle or pause (set)")
	thresholds := fs.String("thresholds", "", "alert levels in percent, comma separated, default 50,80,100 (set)")
	rps := fs.Int("throttle-rps", 0, "requests per second per client while throttled, default 5 (set)")
	history := fs.Bool("history", false, "also list what happened this and earlier months (show)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "usage: %s budget %s APP%s\n", prog, verb, map[string]string{"set": " [--egress 500GiB] [--cost N] [--action none|throttle|pause] [--thresholds 50,80,100] [--throttle-rps N]"}[verb])
		return exitUsage
	}
	app := rest[0]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	switch verb {
	case "clear":
		if err := client.ClearAppBudget(ctx, app); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear budget: %w", err))
		}
		_, _ = fmt.Fprintf(stdout, "removed the budget of %s\n", app)
		return exitOK
	case "set":
		upd, err := buildBudgetUpdate(ctx, client, app, fs, *egress, *cost, *action, *thresholds, *rps)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitUsage
		}
		b, err := client.SetAppBudget(ctx, app, upd)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set budget: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, b, func() { printAppBudget(stdout, b) })
	}
	b, err := client.GetAppBudget(ctx, app)
	var apiErr *apiclient.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound && apiErr.Message == "no budget is set for this app" {
		_, _ = fmt.Fprintf(stdout, "%s has no budget. Set one with: %s budget set %s --egress 500GiB --action throttle\n", app, prog, app)
		return exitOK
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("show budget: %w", err))
	}
	if !*history {
		return writeScheduledTaskResult(stdout, stderr, of, b, func() { printAppBudget(stdout, b) })
	}
	h, err := client.GetAppBudgetHistory(ctx, app)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("budget history: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]any{"budget": b, "history": h.Events}, func() {
		printAppBudget(stdout, b)
		printBudgetHistory(stdout, h)
	})
}

// buildBudgetUpdate starts from the app's current budget so a flag changes
// only its own field.
func buildBudgetUpdate(ctx context.Context, client *Client, app string, fs *flag.FlagSet, egress string, cost float64, action, thresholds string, rps int) (apiclient.AppBudgetUpdate, error) {
	var upd apiclient.AppBudgetUpdate
	cur, err := client.GetAppBudget(ctx, app)
	var apiErr *apiclient.APIError
	switch {
	case err == nil:
		upd = apiclient.AppBudgetUpdate{
			MonthlyEgressBytes: cur.MonthlyEgressBytes, MonthlyCostCeiling: cur.MonthlyCostCeiling,
			Thresholds: cur.Thresholds, Action: cur.Action, ThrottleRPS: cur.ThrottleRPS,
		}
	case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound:
	default:
		return upd, fmt.Errorf("read current budget: %w", err)
	}
	var perr error
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "egress":
			if egress == "0" {
				upd.MonthlyEgressBytes = 0
				return
			}
			upd.MonthlyEgressBytes, perr = spendcap.ParseBytes(egress)
		case "cost":
			upd.MonthlyCostCeiling = cost
		case "action":
			upd.Action = action
		case "thresholds":
			upd.Thresholds, perr = parseThresholds(thresholds)
		case "throttle-rps":
			upd.ThrottleRPS = rps
		}
	})
	return upd, perr
}

func parseThresholds(s string) ([]int, error) {
	var out []int
	for _, p := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("invalid threshold %q, want whole percents such as 50,80,100", p)
		}
		out = append(out, n)
	}
	return out, nil
}

func formatByteSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 4; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTP"[exp])
}

func printAppBudget(out io.Writer, b apiclient.AppBudget) {
	_, _ = fmt.Fprintf(out, "%s: %s, resets %s\n", b.App, b.Period, b.ResetsAt.Format("2006-01-02"))
	if b.MonthlyEgressBytes > 0 {
		_, _ = fmt.Fprintf(out, "  egress      %s of %s (%.0f%%)\n", formatByteSize(b.EgressBytes), formatByteSize(b.MonthlyEgressBytes), b.EgressPct)
	}
	if b.MonthlyCostCeiling > 0 {
		note := ""
		if !b.CostPriced {
			note = "  (no prices configured, so cost reads zero)"
		}
		_, _ = fmt.Fprintf(out, "  cost        %.2f of %.2f (%.0f%%)%s\n", b.Cost, b.MonthlyCostCeiling, b.CostPct, note)
	}
	action := b.Action
	if b.Action == spendcap.ActionThrottle {
		action = fmt.Sprintf("throttle at %d requests per second per client", b.ThrottleRPS)
	}
	pcts := make([]string, 0, len(b.Thresholds))
	for _, t := range b.Thresholds {
		pcts = append(pcts, strconv.Itoa(t))
	}
	_, _ = fmt.Fprintf(out, "  at 100%%     %s\n  alerts at   %s%%\n  in force    %s\n  status      %s: %s\n", action, strings.Join(pcts, ", "), b.Enforced, b.Condition.Reason, b.Condition.Message)
}

func printBudgetHistory(out io.Writer, h apiclient.AppBudgetHistory) {
	if len(h.Events) == 0 {
		_, _ = fmt.Fprintln(out, "\nno history yet")
		return
	}
	_, _ = fmt.Fprintln(out)
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "WHEN\tMONTH\tEVENT\tDETAIL")
	for _, e := range h.Events {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.At.Format("2006-01-02 15:04"), e.Period, e.Kind, e.Detail)
	}
	_ = w.Flush()
}
