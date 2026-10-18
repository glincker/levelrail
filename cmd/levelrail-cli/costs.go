package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/usagecost"
)

func costsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s costs explore [--by app|project|tag|node|kind] [--granularity daily|monthly] [--period mtd|30d|last_month] [--months N]
  %[1]s costs budgets list
  %[1]s costs budgets create --name N --scope project|tag|app --value V --monthly AMOUNT [--thresholds 50,80,100] [--action none|throttle|pause] [--throttle-rps N]
  %[1]s costs budgets set BUDGET-ID [any create flag]
  %[1]s costs budgets delete BUDGET-ID

Budgets alert at each threshold once a month through your budget_threshold
alert rule. The optional action applies at 100%% to the apps in scope and is
lifted when the budget is raised or the month rolls over.
`, prog)
}

// runCosts dispatches "costs <verb>": the cost explorer and named budgets.
func runCosts(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, costsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, costsUsage(prog))
		return exitOK
	case "explore":
		return runCostsExplore(prog, args[1:], stdout, stderr, lookupEnv)
	case "budgets":
		return runCostsBudgets(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown costs subcommand %q\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, costsUsage(prog))
	return exitUsage
}

func runCostsExplore(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "costs explore", "print the exploration as JSON to stdout and nothing else", stderr)
	by := fs.String("by", usagecost.GroupApp, "group by app, project, tag, node or kind")
	gran := fs.String("granularity", "daily", "daily or monthly buckets")
	period := fs.String("period", "mtd", "mtd, 30d or last_month (daily)")
	months := fs.Int("months", 0, "months to cover (monthly, 1 to 12, default 6)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	q := apiclient.CostExploreQuery{GroupBy: *by, Granularity: *gran, Months: *months}
	if *gran == "daily" {
		q.Period = *period
	}
	res, err := client.ExploreCosts(context.Background(), q)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("explore costs: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printCostExploration(stdout, res) })
}

func printCostExploration(out io.Writer, res apiclient.CostExploration) {
	if !res.Configured {
		_, _ = fmt.Fprintln(out, "no prices are configured yet: costs below are zero. Set them with `usage settings set`.")
	}
	_, _ = fmt.Fprintf(out, "Period: %s to %s (%s, grouped by %s)\n", usagecost.DayKey(res.Period.From), usagecost.DayKey(res.Period.To.AddDate(0, 0, -1)), res.Granularity, res.GroupBy)
	_, _ = fmt.Fprintf(out, "Total: %s (%s vs previous period)\n", formatMoney(res.Total, res.Currency), formatDeltaPct(res.DeltaPct))
	if f := res.Forecast; f != nil {
		_, _ = fmt.Fprintf(out, "Forecast month end: %s (range %.2f to %.2f, %s confidence)\n", formatMoney(f.Mid, res.Currency), f.Low, f.High, f.Confidence)
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "\n%s\tCOST\tPREVIOUS\tCHANGE\n", strings.ToUpper(res.GroupBy))
	for _, s := range res.Series {
		label := s.Key
		if n := res.Labels[res.GroupBy+":"+s.Key]; n != "" {
			label = n
		}
		_, _ = fmt.Fprintf(w, "%s\t%.2f\t%.2f\t%s\n", label, s.Total, s.Previous, formatDeltaPct(s.DeltaPct))
	}
	_ = w.Flush()
	if len(res.Movers) > 0 {
		_, _ = fmt.Fprintln(out, "\nTop movers (largest increase):")
		for _, m := range res.Movers {
			_, _ = fmt.Fprintf(out, "  %s  +%.2f (%s)\n", m.Key, m.Delta, formatDeltaPct(m.DeltaPct))
		}
	}
}

func runCostsBudgets(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	verb := "list"
	if len(args) > 0 && (args[0] == "list" || args[0] == "create" || args[0] == "set" || args[0] == "delete") {
		verb, args = args[0], args[1:]
	}
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "costs budgets "+verb, "print the result as JSON to stdout and nothing else", stderr)
	name := fs.String("name", "", "budget name")
	scope := fs.String("scope", "", "project, tag or app")
	value := fs.String("value", "", "project, tag name or app the budget covers")
	monthly := fs.Float64("monthly", 0, "monthly amount")
	thresholds := fs.String("thresholds", "", "alert levels in percent, such as 50,80,100")
	action := fs.String("action", "", "at 100%: none, throttle or pause")
	rps := fs.Int("throttle-rps", 0, "requests per second per client when the action is throttle")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	rest := fs.Args()
	body := map[string]any{}
	for k, v := range map[string]any{"name": *name, "scope": *scope, "scope_value": *value, "action": *action} {
		if s, _ := v.(string); s != "" {
			body[k] = s
		}
	}
	if *monthly > 0 {
		body["monthly"] = *monthly
	}
	if *rps > 0 {
		body["throttle_rps"] = *rps
	}
	if *thresholds != "" {
		t, err := parseThresholds(*thresholds)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: --thresholds: %v\n", prog, err)
			return exitUsage
		}
		body["thresholds"] = t
	}
	switch verb {
	case "create":
		if *name == "" || *scope == "" || *value == "" || *monthly <= 0 {
			_, _ = fmt.Fprintf(stderr, "usage: %s costs budgets create --name N --scope project|tag|app --value V --monthly AMOUNT\n", prog)
			return exitUsage
		}
		b, err := client.CreateCostBudget(ctx, body)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("create budget: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, b, func() { _, _ = fmt.Fprintf(stdout, "created budget %s (%s)\n", b.ID, b.Name) })
	case "set":
		if len(rest) != 1 || len(body) == 0 {
			_, _ = fmt.Fprintf(stderr, "usage: %s costs budgets set BUDGET-ID [--name N] [--monthly AMOUNT] [--thresholds 50,80,100] [--action pause]\n", prog)
			return exitUsage
		}
		b, err := client.UpdateCostBudget(ctx, rest[0], body)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("update budget: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, b, func() { _, _ = fmt.Fprintf(stdout, "updated budget %s (%s)\n", b.ID, b.Name) })
	case "delete":
		if len(rest) != 1 {
			_, _ = fmt.Fprintf(stderr, "usage: %s costs budgets delete BUDGET-ID\n", prog)
			return exitUsage
		}
		if err := client.DeleteCostBudget(ctx, rest[0]); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete budget: %w", err))
		}
		if jsonOut {
			_, _ = fmt.Fprintln(stdout, `{"deleted":true}`)
			return exitOK
		}
		_, _ = fmt.Fprintf(stdout, "deleted budget %s\n", rest[0])
		return exitOK
	}
	res, err := client.ListCostBudgets(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list budgets: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printCostBudgets(stdout, res) })
}

func printCostBudgets(out io.Writer, res apiclient.CostBudgets) {
	if len(res.Budgets) == 0 {
		_, _ = fmt.Fprintln(out, "no cost budgets set")
		return
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tSCOPE\tBUDGET\tSPENT\tFORECAST\tACTION\tSTATE")
	for _, b := range res.Budgets {
		state := b.Level
		if state == "" {
			state = "ok"
		}
		if b.Enforced != "" {
			state += " (" + b.Enforced + ")"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s:%s\t%.2f\t%.2f (%.0f%%)\t%.2f\t%s\t%s\n", b.ID, b.Name, b.Scope, b.ScopeValue, b.Monthly, b.Spent, b.Pct, b.Forecast.Mid, b.Action, state)
	}
	_ = w.Flush()
}
