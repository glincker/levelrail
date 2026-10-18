package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/usagecost"
)

// runUsage dispatches "usage <verb>": cost showback.
func runUsage(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usageCostUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, usageCostUsage(prog))
		return exitOK
	case "report":
		return runUsageReport(prog, args[1:], stdout, stderr, lookupEnv)
	case "settings":
		return runUsageSettings(prog, args[1:], stdout, stderr, lookupEnv)
	case "budgets":
		return runUsageBudgets(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown usage subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, usageCostUsage(prog))
		return exitUsage
	}
}

func usageCostUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s usage report [--period mtd|30d|last_month|custom] [--from D --to D] [--by project|environment|team|resource|tag] [--csv]
  %[1]s usage settings                                   show prices and node bills
  %[1]s usage settings set [flags]                       change prices (instance admin)
  %[1]s usage budgets                                    list budgets and where they stand
  %[1]s usage budgets set project|team ID --monthly N [--warn-pct 80]
  %[1]s usage budgets delete BUDGET-ID

Cost is allocated from your own prices by measured usage. It is a showback
figure for sharing a bill fairly, not an invoice.
`, prog)
}

func formatMoney(v float64, currency string) string {
	return fmt.Sprintf("%.2f %s", v, currency)
}

func formatDeltaPct(p *float64) string {
	if p == nil {
		return "-"
	}
	return fmt.Sprintf("%+.0f%%", *p)
}

func usageGroupLabel(labels map[string]string, dim string, g usagecost.Group) string {
	switch dim {
	case "resource":
		return g.Kind + ":" + g.Name
	case "tag":
		return g.Key
	case "project", "environment", "team":
		if n := labels[dim+":"+g.Key]; n != "" {
			return n
		}
	}
	return g.Key
}

func runUsageReport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "usage report", "print the report as JSON to stdout and nothing else", stderr)
	period := fs.String("period", "mtd", "mtd, 30d, last_month or custom")
	from := fs.String("from", "", "first day (YYYY-MM-DD) for --period custom")
	to := fs.String("to", "", "last day (YYYY-MM-DD, inclusive) for --period custom")
	by := fs.String("by", "project", "breakdown: project, environment, team, resource or tag")
	csv := fs.Bool("csv", false, "print the resource breakdown as CSV")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	q := apiclient.UsageReportQuery{Period: *period, From: *from, To: *to}
	if *csv {
		data, err := client.DownloadUsageReportCSV(context.Background(), q)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("download usage report: %w", err))
		}
		_, _ = stdout.Write(data)
		return exitOK
	}
	rep, err := client.GetUsageReport(context.Background(), q)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get usage report: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rep, func() { printUsageReport(stdout, rep, *by) })
}

func printUsageReport(out io.Writer, rep apiclient.UsageReport, by string) {
	if !rep.Configured {
		_, _ = fmt.Fprintln(out, "no prices are configured yet: costs below are zero. Set them with `usage settings set`.")
	}
	_, _ = fmt.Fprintf(out, "Period: %s to %s (%s)\n", usagecost.DayKey(rep.Period.From), usagecost.DayKey(rep.Period.To.AddDate(0, 0, -1)), rep.Mode)
	_, _ = fmt.Fprintf(out, "Total: %s (%s vs previous period)\n", formatMoney(rep.Totals.Total, rep.Currency), formatDeltaPct(rep.Totals.DeltaPct))
	if rep.IncludesUnallocated {
		_, _ = fmt.Fprintf(out, "Unallocated (idle capacity): %s\n", formatMoney(rep.Totals.Unallocated, rep.Currency))
	}
	if rep.Forecast != nil {
		_, _ = fmt.Fprintf(out, "Forecast month end: %s\n", formatMoney(*rep.Forecast, rep.Currency))
	}
	for _, n := range rep.UnpricedNodes {
		_, _ = fmt.Fprintf(out, "warning: node %s has no price, its apps are not costed\n", n)
	}
	groups := rep.Projects
	switch by {
	case "environment":
		groups = rep.Environments
	case "team":
		groups = rep.Teams
	case "resource":
		groups = rep.Resources
	case "tag":
		groups = rep.Tags
	default:
		by = "project"
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "\n%s\tCOST\tPREVIOUS\tCHANGE\n", strings.ToUpper(by))
	for _, g := range groups {
		_, _ = fmt.Fprintf(w, "%s\t%.2f\t%.2f\t%s\n", usageGroupLabel(rep.Labels, by, g), g.Total, g.Previous, formatDeltaPct(g.DeltaPct))
	}
	_ = w.Flush()
}

type nodeFlags []usagecost.NodePrice

func (n *nodeFlags) String() string { return fmt.Sprint(len(*n)) }

func (n *nodeFlags) Set(v string) error {
	id, rest, ok := strings.Cut(v, "=")
	parts := strings.Split(rest, ":")
	if !ok || id == "" || len(parts) < 1 || len(parts) > 3 {
		return fmt.Errorf("want NODE=MONTHLY[:VCPUS[:GIB]], got %q", v)
	}
	np := usagecost.NodePrice{NodeID: id}
	dst := []*float64{&np.MonthlyPrice, &np.VCPUs, &np.MemoryGiB}
	for i, p := range parts {
		f, err := strconv.ParseFloat(p, 64)
		if err != nil || f < 0 {
			return fmt.Errorf("%q is not a number of zero or more", p)
		}
		*dst[i] = f
	}
	*n = append(*n, np)
	return nil
}

func runUsageSettings(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	set := len(args) > 0 && args[0] == "set"
	if set {
		args = args[min(1, len(args)):]
	}
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "usage settings", "print the settings as JSON to stdout and nothing else", stderr)
	mode := fs.String("mode", "", "unit (price per core and GiB hour) or node (split each node's monthly bill)")
	currency := fs.String("currency", "", "currency label, such as USD")
	cpu := fs.Float64("cpu-core-hour", -1, "price per vCPU hour (unit mode)")
	mem := fs.Float64("mem-gib-hour", -1, "price per GiB of memory hour (unit mode)")
	storage := fs.Float64("storage-gib-month", -1, "price per GiB of volume storage per month")
	egress := fs.Float64("egress-gib", -1, "price per GiB sent out")
	spread := fs.Float64("idle-spread-pct", -1, "percent of idle capacity spread over apps by usage (0 to 100)")
	basis := fs.String("basis", "", "avg or p50")
	weight := fs.Float64("cpu-weight-pct", -1, "percent of a node bill attributed to CPU, the rest to memory")
	var nodes nodeFlags
	fs.Var(&nodes, "node", "node bill as NODE=MONTHLY[:VCPUS[:GIB]], repeatable; replaces all node bills")
	reprice := fs.Bool("reprice-month", false, "also reprice the month to date with the new prices")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	cur, err := client.GetUsageSettings(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get usage settings: %w", err))
	}
	if set {
		s := cur.Settings
		applyUsageFlags(&s, *mode, *currency, *basis, *cpu, *mem, *storage, *egress, *spread, *weight)
		upd := apiclient.UsageSettingsUpdate{Settings: &s, RepriceMonth: *reprice}
		if len(nodes) > 0 {
			n := []usagecost.NodePrice(nodes)
			upd.Nodes = &n
		}
		if cur, err = client.UpdateUsageSettings(ctx, upd); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("update usage settings: %w", err))
		}
	}
	return writeScheduledTaskResult(stdout, stderr, of, cur, func() { printUsageSettings(stdout, cur) })
}

func applyUsageFlags(s *usagecost.Settings, mode, currency, basis string, cpu, mem, storage, egress, spread, weight float64) {
	if mode != "" {
		s.Mode = mode
	}
	if currency != "" {
		s.Currency = currency
	}
	if basis != "" {
		s.Basis = basis
	}
	for _, p := range []struct {
		v   float64
		dst *float64
	}{{cpu, &s.CPUCoreHour}, {mem, &s.MemGiBHour}, {storage, &s.StorageGiBMonth}, {egress, &s.EgressGiB}, {spread, &s.IdleSpreadPct}, {weight, &s.CPUWeightPct}} {
		if p.v >= 0 {
			*p.dst = p.v
		}
	}
}

func printUsageSettings(out io.Writer, u apiclient.UsageSettings) {
	s := u.Settings
	_, _ = fmt.Fprintf(out, "mode: %s   currency: %s   basis: %s\n", s.Mode, s.Currency, s.Basis)
	_, _ = fmt.Fprintf(out, "cpu per core hour: %g   memory per GiB hour: %g   storage per GiB month: %g   egress per GiB: %g\n",
		s.CPUCoreHour, s.MemGiBHour, s.StorageGiBMonth, s.EgressGiB)
	_, _ = fmt.Fprintf(out, "idle spread: %g%%   cpu weight: %g%%\n", s.IdleSpreadPct, s.CPUWeightPct)
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "\nNODE\tMONTHLY\tVCPUS\tMEMORY GIB")
	for _, n := range u.Nodes {
		_, _ = fmt.Fprintf(w, "%s\t%.2f\t%g\t%.1f\n", n.NodeID, n.MonthlyPrice, n.VCPUs, n.MemoryGiB)
	}
	_ = w.Flush()
}

func runUsageBudgets(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	verb := ""
	if len(args) > 0 && (args[0] == "set" || args[0] == "delete" || args[0] == "list") {
		verb, args = args[0], args[1:]
	}
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "usage budgets", "print the result as JSON to stdout and nothing else", stderr)
	monthly := fs.Float64("monthly", 0, "monthly budget amount (set)")
	warn := fs.Float64("warn-pct", 0, "percent of the budget that raises the first alert, default 80 (set)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	rest := fs.Args()
	switch verb {
	case "set":
		if len(rest) != 2 || *monthly <= 0 {
			_, _ = fmt.Fprintf(stderr, "usage: %s usage budgets set project|team ID --monthly N [--warn-pct 80]\n", prog)
			return exitUsage
		}
		b, err := client.SetUsageBudget(ctx, rest[0], rest[1], *monthly, *warn)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set usage budget: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, b, func() {
			_, _ = fmt.Fprintf(stdout, "budget %s: %s %s, %.2f a month, warns at %.0f%%\n", b.ID, b.Scope, b.ScopeID, b.Monthly, b.WarnPct)
		})
	case "delete":
		if len(rest) != 1 {
			_, _ = fmt.Fprintf(stderr, "usage: %s usage budgets delete BUDGET-ID\n", prog)
			return exitUsage
		}
		if err := client.DeleteUsageBudget(ctx, rest[0]); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete usage budget: %w", err))
		}
		_, _ = fmt.Fprintf(stdout, "deleted budget %s\n", rest[0])
		return exitOK
	}
	res, err := client.ListUsageBudgets(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list usage budgets: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printUsageBudgets(stdout, res) })
}

func printUsageBudgets(out io.Writer, res apiclient.UsageBudgets) {
	if len(res.Budgets) == 0 {
		_, _ = fmt.Fprintln(out, "no budgets set")
		return
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tSCOPE\tNAME\tBUDGET\tSPENT\tFORECAST\tSTATE")
	for _, b := range res.Budgets {
		state := b.Level
		if state == "" {
			state = "ok"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%.2f\t%.2f (%.0f%%)\t%.2f\t%s\n", b.ID, b.Scope, b.Name, b.Monthly, b.Spent, b.SpentPct, b.Forecast, state)
	}
	_ = w.Flush()
}
