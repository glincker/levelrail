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

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/costbudget"
	"github.com/GLINCKER/levelrail/internal/spendcap"
)

const spendScopeApp, spendScopeProject = "app", "project"

func spendUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s spend caps list
  %[1]s spend caps status app|project NAME
  %[1]s spend caps set app|project NAME [--cost N] [--egress 500GiB] [--warn 80,100] [--mode alert|block_deploys] [--action none|throttle|pause]
  %[1]s spend caps rm app|project NAME
  %[1]s spend caps dry-run app|project NAME
  %[1]s spend caps override app NAME --reason TEXT

A cap alerts at each warn level once a month. In block_deploys mode, new
deploys are refused at 100%% until the month rolls over, the cap is raised,
or a root token overrides the guard (audited, with a reason). dry-run only
previews what a pause would stop and save; it changes nothing. --egress is
for app caps only. Setting block_deploys needs a root token.
`, prog)
}

// runSpend dispatches "spend caps <verb>".
func runSpend(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, spendUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, spendUsage(prog))
		return exitOK
	case "caps":
		return runSpendCapsVerb(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown spend subcommand %q\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, spendUsage(prog))
	return exitUsage
}

func runSpendCapsVerb(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, spendUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "list", "status", "set", "rm", "dry-run", "override":
		return runSpendCaps(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown spend caps subcommand %q\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, spendUsage(prog))
	return exitUsage
}

// findProjectCap returns the project scoped budget the CLI and dashboard
// manage for a project, matched by id or by the "spend cap: NAME" budget name.
func findProjectCap(budgets []apiclient.CostBudgetStatus, project string) (apiclient.CostBudgetStatus, bool) {
	for _, b := range budgets {
		if b.Scope == costbudget.ScopeProject && (b.ScopeValue == project || strings.EqualFold(b.Name, "spend cap: "+project)) {
			return b, true
		}
	}
	return apiclient.CostBudgetStatus{}, false
}

func runSpendCaps(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "spend caps "+verb, "print the result as JSON to stdout and nothing else", stderr)
	cost := fs.Float64("cost", 0, "monthly cost ceiling in the configured currency (set)")
	egress := fs.String("egress", "", "monthly egress limit such as 500GiB, 0 removes it (set, app only)")
	warn := fs.String("warn", "", "warn levels in percent, comma separated (set)")
	mode := fs.String("mode", "", "alert or block_deploys (set)")
	action := fs.String("action", "", "what happens at 100%: none, throttle or pause (set)")
	reason := fs.String("reason", "", "why deploys should go through (override)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	rest := fs.Args()
	if verb == "list" {
		return spendCapsList(ctx, client, stdout, stderr, jsonOut, of)
	}
	if len(rest) != 2 || (rest[0] != spendScopeApp && rest[0] != spendScopeProject) || (verb == "override" && rest[0] != spendScopeApp) {
		_, _ = fmt.Fprint(stderr, spendUsage(prog))
		return exitUsage
	}
	scope, name := rest[0], rest[1]
	fail := func(what string, err error) int {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", what, err))
	}

	switch verb {
	case "override":
		if strings.TrimSpace(*reason) == "" {
			_, _ = fmt.Fprintln(stderr, "an override needs --reason")
			return exitUsage
		}
		g, err := client.OverrideSpendGuard(ctx, name, *reason)
		if err != nil {
			return fail("override", err)
		}
		return writeScheduledTaskResult(stdout, stderr, of, g, func() {
			if g.Override != nil {
				_, _ = fmt.Fprintf(stdout, "deploys of %s are allowed until %s\n", name, g.Override.ExpiresAt.Format("2006-01-02 15:04 MST"))
			}
		})
	case "dry-run":
		var res apiclient.SpendDryRun
		var err error
		if scope == spendScopeApp {
			res, err = client.DryRunAppBudget(ctx, name)
		} else {
			var b apiclient.CostBudgetStatus
			if b, err = lookupProjectCap(ctx, client, name); err == nil {
				res, err = client.DryRunCostBudget(ctx, b.ID)
			}
		}
		if err != nil {
			return fail("dry-run", err)
		}
		return writeScheduledTaskResult(stdout, stderr, of, res, func() { printSpendDryRun(stdout, res) })
	case "status":
		return spendCapStatus(ctx, client, scope, name, stdout, stderr, jsonOut, of)
	case "rm":
		if scope == spendScopeApp {
			if err := client.ClearAppBudget(ctx, name); err != nil {
				return fail("remove cap", err)
			}
		} else {
			b, err := lookupProjectCap(ctx, client, name)
			if err == nil {
				err = client.DeleteCostBudget(ctx, b.ID)
			}
			if err != nil {
				return fail("remove cap", err)
			}
		}
		_, _ = fmt.Fprintf(stdout, "removed the spend cap of %s %s\n", scope, name)
		return exitOK
	}

	thresholds, err := optionalThresholds(*warn)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: --warn: %v\n", prog, err)
		return exitUsage
	}
	in := spendSetInput{cost: *cost, egress: *egress, thresholds: thresholds, mode: *mode, action: *action, visited: map[string]bool{}}
	fs.Visit(func(f *flag.Flag) { in.visited[f.Name] = true })
	if scope == spendScopeApp {
		return spendCapSetApp(ctx, client, name, in, stdout, stderr, jsonOut, of)
	}
	return spendCapSetProject(ctx, client, name, in, stdout, stderr, jsonOut, of)
}

func optionalThresholds(s string) ([]int, error) {
	if s == "" {
		return nil, nil
	}
	return parseThresholds(s)
}

func lookupProjectCap(ctx context.Context, client *Client, project string) (apiclient.CostBudgetStatus, error) {
	res, err := client.ListCostBudgets(ctx)
	if err != nil {
		return apiclient.CostBudgetStatus{}, err
	}
	b, ok := findProjectCap(res.Budgets, project)
	if !ok {
		return b, fmt.Errorf("project %q has no spend cap", project)
	}
	return b, nil
}

// spendSetInput carries the set flags; visited says which were passed, so a
// flag changes only its own field.
type spendSetInput struct {
	cost       float64
	egress     string
	thresholds []int
	mode       string
	action     string
	visited    map[string]bool
}

func spendCapSetApp(ctx context.Context, client *Client, app string, in spendSetInput, stdout, stderr io.Writer, jsonOut bool, of outputFlags) int {
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
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read current cap: %w", err))
	}
	if in.visited["cost"] {
		upd.MonthlyCostCeiling = in.cost
	}
	if in.visited["egress"] {
		upd.MonthlyEgressBytes = 0
		if in.egress != "0" {
			if upd.MonthlyEgressBytes, err = spendcap.ParseBytes(in.egress); err != nil {
				_, _ = fmt.Fprintln(stderr, err)
				return exitUsage
			}
		}
	}
	if in.visited["warn"] {
		upd.Thresholds = in.thresholds
	}
	if in.visited["action"] {
		upd.Action = in.action
	}
	if in.visited["mode"] {
		upd.GuardMode = in.mode
	}
	b, err := client.SetAppBudget(ctx, app, upd)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set cap: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, b, func() { printAppBudget(stdout, b) })
}

func spendCapSetProject(ctx context.Context, client *Client, project string, in spendSetInput, stdout, stderr io.Writer, jsonOut bool, of outputFlags) int {
	if in.visited["egress"] {
		_, _ = fmt.Fprintln(stderr, "--egress applies to app caps only")
		return exitUsage
	}
	body := map[string]any{}
	if in.visited["cost"] {
		body["monthly"] = in.cost
	}
	if in.visited["warn"] {
		body["thresholds"] = in.thresholds
	}
	if in.visited["action"] {
		body["action"] = in.action
	}
	if in.visited["mode"] {
		body["guard_mode"] = in.mode
	}
	res, err := client.ListCostBudgets(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set cap: %w", err))
	}
	var saved costbudget.Budget
	if cur, found := findProjectCap(res.Budgets, project); found {
		if len(body) == 0 {
			_, _ = fmt.Fprintf(stderr, "nothing to change, pass --cost, --warn, --mode or --action\n")
			return exitUsage
		}
		saved, err = client.UpdateCostBudget(ctx, cur.ID, body)
	} else {
		if in.cost <= 0 {
			_, _ = fmt.Fprintf(stderr, "a new project cap needs --cost\n")
			return exitUsage
		}
		body["name"], body["scope"], body["scope_value"] = "spend cap: "+project, costbudget.ScopeProject, project
		saved, err = client.CreateCostBudget(ctx, body)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set cap: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, saved, func() {
		_, _ = fmt.Fprintf(stdout, "project %s: %.2f a month, guard %s, action %s\n", project, saved.Monthly, saved.GuardMode, saved.Action)
	})
}

func spendCapStatus(ctx context.Context, client *Client, scope, name string, stdout, stderr io.Writer, jsonOut bool, of outputFlags) int {
	if scope == spendScopeApp {
		b, err := client.GetAppBudget(ctx, name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("cap status: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, b, func() {
			printAppBudget(stdout, b)
			printSpendForecast(stdout, b)
		})
	}
	b, err := lookupProjectCap(ctx, client, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("cap status: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, b, func() {
		_, _ = fmt.Fprintf(stdout, "project %s: %.2f of %.2f %s (%.0f%%), projected %.2f at month end (%s)\n  guard %s, at 100%% %s, level %s\n",
			name, b.Spent, b.Monthly, b.Currency, b.Pct, b.Forecast.Mid, b.Forecast.Confidence, b.GuardMode, b.Action, levelOrOK(b.Level))
	})
}

func levelOrOK(s string) string {
	if s == "" {
		return "ok"
	}
	return s
}

func printSpendForecast(out io.Writer, b apiclient.AppBudget) {
	if b.Forecast != nil {
		_, _ = fmt.Fprintf(out, "  projected   %.2f cost at month end (%.0f%% of the limit, %s confidence)\n", b.Forecast.Cost.Mid, b.Forecast.Pct, b.Forecast.Cost.Confidence)
	}
	if b.Guard != nil && b.Guard.Blocked {
		line := "  deploys     blocked"
		if b.Guard.Override != nil {
			line = fmt.Sprintf("  deploys     allowed by override until %s (%s)", b.Guard.Override.ExpiresAt.Format("15:04 MST"), b.Guard.Override.Reason)
		}
		_, _ = fmt.Fprintln(out, line)
	}
}

func spendCapsList(ctx context.Context, client *Client, stdout, stderr io.Writer, jsonOut bool, of outputFlags) int {
	apps, err := client.ListAppBudgets(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list caps: %w", err))
	}
	budgets, err := client.ListCostBudgets(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list caps: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]any{"apps": apps.Caps, "budgets": budgets.Budgets}, func() {
		if len(apps.Caps) == 0 && len(budgets.Budgets) == 0 {
			_, _ = fmt.Fprintln(stdout, "no spend caps set")
			return
		}
		w := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "SCOPE\tNAME\tUSED\tPROJECTED\tGUARD\tSTATE")
		for _, c := range apps.Caps {
			proj := "-"
			if c.Forecast != nil {
				proj = fmt.Sprintf("%.0f%%", c.Forecast.Pct)
			}
			_, _ = fmt.Fprintf(w, "app\t%s\t%.0f%%\t%s\t%s\t%s\n", c.App, c.Pct, proj, c.GuardMode, c.Condition.Reason)
		}
		for _, b := range budgets.Budgets {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%.0f%%\t%.0f%%\t%s\t%s\n", b.Scope, b.ScopeValue, b.Pct, b.ForecastPct, b.GuardMode, levelOrOK(b.Level))
		}
		_ = w.Flush()
	})
}

func printSpendDryRun(out io.Writer, r apiclient.SpendDryRun) {
	_, _ = fmt.Fprintf(out, "%s\n", r.Note)
	if len(r.Apps) == 0 {
		_, _ = fmt.Fprintln(out, "no apps in scope")
		return
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "APP\tREPLICAS\tPER DAY\tEST. SAVING")
	for _, a := range r.Apps {
		_, _ = fmt.Fprintf(w, "%s\t%d\t%.2f\t%.2f\n", a.Name, a.Replicas, a.DailyRunRate, a.EstimatedSaving)
	}
	_ = w.Flush()
	_, _ = fmt.Fprintf(out, "%d replicas, about %.2f %s saved over the remaining %.0f days\n", r.TotalReplicas, r.EstimatedSaving, r.Currency, r.RemainingDays)
}
