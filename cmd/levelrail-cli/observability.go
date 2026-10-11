package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func metricsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s metrics query <app> --metric NAME [--since 1h | --from T --to T] [--step 5m] [--max-points N] [--compare]
  %[1]s metrics top [--by cpu|memory|network] [--limit N]
  %[1]s metrics investigate <app> [--at T] [--window 10m]

query    one metric series, downsampled server side (--max-points, default 600),
         with --compare adding the previous equal-length period.
top      apps ranked by their latest CPU, memory or network reading.
investigate  everything around a spike: request summary against the window
         before it, busiest routes, status codes and the merged deploy, restart
         and saturation timeline.

Metric names: cpu_percent, memory_usage_bytes, memory_limit_bytes,
network_rx_bytes, network_tx_bytes, disk_read_bytes, disk_write_bytes,
container_restart_count, build_duration_seconds.
%[2]s`, prog, commonFlagsHelp)
}

func runMetrics(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, metricsUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, metricsUsage(prog))
		return exitOK
	case "query":
		return runAPICmd(prog, metricsQueryCommand(), rest, stdout, stderr, lookupEnv)
	case "top":
		return runAPICmd(prog, metricsTopCommand(), rest, stdout, stderr, lookupEnv)
	case "investigate":
		return runAPICmd(prog, metricsInvestigateCommand(), rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown metrics subcommand %q\n\n%s", prog, args[0], metricsUsage(prog))
		return exitUsage
	}
}

func metricsQueryCommand() apiCmd {
	var tr timeRangeFlags
	var metric, step string
	var maxPoints int
	var compare bool
	return apiCmd{
		label: "metrics query",
		usage: metricsUsage,
		args:  1,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&metric, "metric", "", "metric name (required)")
			fs.StringVar(&tr.since, "since", "", "how far back, e.g. 1h or 7d-as-168h (default 1h)")
			fs.StringVar(&tr.from, "from", "", "RFC3339 window start")
			fs.StringVar(&tr.to, "to", "", "RFC3339 window end (default now)")
			fs.StringVar(&step, "step", "", "bucket size, e.g. 5m (default chosen from --max-points)")
			fs.IntVar(&maxPoints, "max-points", 600, "cap on returned points; the server widens the step to fit")
			fs.BoolVar(&compare, "compare", false, "also return the previous period, aligned to this window")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			if metric == "" {
				return nil, nil, newValidationError("--metric is required")
			}
			from, to, err := resolveTimeRange(tr, time.Now())
			if err != nil {
				return nil, nil, err
			}
			stepD, err := parseStepFlag(step)
			if err != nil {
				return nil, nil, err
			}
			res, err := c.QueryAppMetricsSeries(ctx, pos[0], metric, from, to, apiclient.MetricsQueryOptions{Step: stepD, MaxPoints: maxPoints, Compare: compare})
			return res, func(w io.Writer) { printMetricSeries(w, res) }, err
		},
	}
}

func printMetricSeries(w io.Writer, res apiclient.MetricsSeriesResource) {
	prev := make(map[int64]float64, len(res.PreviousPoints))
	for _, p := range res.PreviousPoints {
		prev[p.Timestamp.Unix()] = p.Value
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	header := "TIMESTAMP\tVALUE\tMAX"
	if len(res.PreviousPoints) > 0 {
		header += "\tPREVIOUS"
	}
	_, _ = fmt.Fprintln(tw, header)
	for _, p := range res.Points {
		row := fmt.Sprintf("%s\t%.2f\t%.2f", p.Timestamp.UTC().Format(time.RFC3339), p.Value, p.Max)
		if len(res.PreviousPoints) > 0 {
			if v, ok := prev[p.Timestamp.Unix()]; ok {
				row += fmt.Sprintf("\t%.2f", v)
			} else {
				row += "\t-"
			}
		}
		_, _ = fmt.Fprintln(tw, row)
	}
	_ = tw.Flush()
	note := fmt.Sprintf("# %s, %d points", res.Metric, len(res.Points))
	if res.StepSeconds > 0 {
		note += fmt.Sprintf(", step %s", time.Duration(res.StepSeconds*float64(time.Second)))
	}
	if res.Downsampled {
		note += " (downsampled)"
	}
	_, _ = fmt.Fprintln(w, note)
}

func metricsTopCommand() apiCmd {
	var by string
	var limit int
	return apiCmd{
		label: "metrics top",
		usage: metricsUsage,
		args:  0,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&by, "by", "cpu", "rank by cpu, memory or network")
			fs.IntVar(&limit, "limit", 10, "apps to show")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			if by != "cpu" && by != "memory" && by != "network" {
				return nil, nil, newValidationError("--by must be cpu, memory or network")
			}
			if limit < 1 {
				return nil, nil, newValidationError("--limit must be positive")
			}
			rows, err := c.ListAppResourceUsage(ctx)
			if err != nil {
				return nil, nil, fmt.Errorf("list app resource usage: %w", err)
			}
			ranked := rankUsage(rows, by, limit)
			return ranked, func(w io.Writer) { printUsage(w, ranked) }, nil
		},
	}
}

func usageValue(r apiclient.AppResourceUsageResource, by string) float64 {
	get := func(p *float64) float64 {
		if p == nil {
			return 0
		}
		return *p
	}
	switch by {
	case "memory":
		return get(r.MemoryUsageBytes)
	case "network":
		return get(r.NetworkRxBytes) + get(r.NetworkTxBytes)
	}
	return get(r.CPUPercent)
}

func rankUsage(rows []apiclient.AppResourceUsageResource, by string, limit int) []apiclient.AppResourceUsageResource {
	out := append([]apiclient.AppResourceUsageResource(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool {
		vi, vj := usageValue(out[i], by), usageValue(out[j], by)
		if vi != vj {
			return vi > vj
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func printUsage(w io.Writer, rows []apiclient.AppResourceUsageResource) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "APP\tCPU%\tMEMORY\tNET RX\tNET TX")
	f := func(p *float64, bytes bool) string {
		if p == nil {
			return "-"
		}
		if bytes {
			return humanBytes(int64(*p))
		}
		return fmt.Sprintf("%.1f", *p)
	}
	for _, r := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Name, f(r.CPUPercent, false), f(r.MemoryUsageBytes, true), f(r.NetworkRxBytes, true), f(r.NetworkTxBytes, true))
	}
	_ = tw.Flush()
}

func metricsInvestigateCommand() apiCmd {
	var at, window string
	var tr timeRangeFlags
	return apiCmd{
		label: "metrics investigate",
		usage: metricsUsage,
		args:  1,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&at, "at", "", "centre of the window: RFC3339 time or a duration ago (default now)")
			fs.StringVar(&window, "window", "10m", "window width when --at is used")
			fs.StringVar(&tr.from, "from", "", "RFC3339 window start (overrides --at)")
			fs.StringVar(&tr.to, "to", "", "RFC3339 window end (default now)")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			now := time.Now()
			var from, to time.Time
			if tr.from != "" {
				var err error
				if from, to, err = resolveTimeRange(tr, now); err != nil {
					return nil, nil, err
				}
			} else {
				width, err := time.ParseDuration(window)
				if err != nil || width <= 0 {
					return nil, nil, newValidationError("--window must be a positive duration like 10m")
				}
				centre := now
				if at != "" {
					if centre, err = apiclient.ParseLogTime(at, now); err != nil {
						return nil, nil, newValidationError("--at: %v", err)
					}
				}
				from, to = centre.Add(-width/2), centre.Add(width/2)
				if to.After(now) {
					to = now
				}
			}
			res, err := c.InvestigateApp(ctx, pos[0], from, to)
			return res, func(w io.Writer) { printInvestigation(w, res) }, err
		},
	}
}

func printInvestigation(w io.Writer, r apiclient.InvestigationResource) {
	_, _ = fmt.Fprintf(w, "app: %s\nwindow: %s to %s\n", r.App, r.From.UTC().Format(time.RFC3339), r.To.UTC().Format(time.RFC3339))
	row := func(label string, s apiclient.InvestigateSummary) {
		if !s.HasTraffic {
			_, _ = fmt.Fprintf(w, "%-9s no requests\n", label)
			return
		}
		_, _ = fmt.Fprintf(w, "%-9s %.0f req  %.2f/s  4xx %.2f%%  5xx %.2f%%  p50 %.0fms  p95 %.0fms  p99 %.0fms\n",
			label, s.Requests, s.RatePerSec, s.ErrorRate4xx*100, s.ErrorRate5xx*100, s.P50Ms, s.P95Ms, s.P99Ms)
	}
	row("window:", r.Summary)
	row("before:", r.Baseline)
	if len(r.TopRoutes) > 0 {
		_, _ = fmt.Fprintln(w, "\ntop routes")
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ROUTE\tREQUESTS\tSHARE\t5XX%\tAVG MS")
		for _, t := range r.TopRoutes {
			_, _ = fmt.Fprintf(tw, "%s\t%.0f\t%.0f%%\t%.1f\t%.0f\n", t.Route, t.Requests, t.Share*100, t.ErrorRate5xx*100, t.AvgMs)
		}
		_ = tw.Flush()
	} else if !r.RoutesAvailable {
		_, _ = fmt.Fprintln(w, "\nno route breakdown (APP_REQUEST_ROUTES is off or no traffic)")
	}
	if len(r.StatusCodes) > 0 {
		parts := make([]string, 0, len(r.StatusCodes))
		for _, s := range r.StatusCodes {
			parts = append(parts, fmt.Sprintf("%d x%.0f", s.Status, s.Count))
		}
		_, _ = fmt.Fprintf(w, "\nstatus codes: %s\n", strings.Join(parts, ", "))
	}
	_, _ = fmt.Fprintln(w, "\nwhat changed")
	if len(r.Timeline) == 0 {
		_, _ = fmt.Fprintln(w, "  nothing recorded in this window")
	}
	for _, e := range r.Timeline {
		mark := " "
		if e.LikelyCause {
			mark = "*"
		}
		_, _ = fmt.Fprintf(w, " %s %s  %-10s %s\n", mark, e.At.UTC().Format("15:04:05"), e.Kind, e.Title)
	}
}

func logsSearchCommand() apiCmd {
	var s apiclient.LogSearch
	var tr timeRangeFlags
	var fields stringList
	return apiCmd{
		label: "logs search",
		usage: logsUsage,
		args:  1,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&s.Text, "q", "", "full text phrase")
			fs.StringVar(&s.Level, "level", "", "minimum level: trace, debug, info, warn, error, fatal")
			fs.StringVar(&s.Container, "container", "", "container id prefix")
			fs.StringVar(&s.Stream, "stream", "", "stdout or stderr")
			fs.Var(&fields, "field", "structured field filter, repeatable: key=value, key!=value, key~text, key>n")
			fs.StringVar(&tr.since, "since", "", "how far back (default 1h)")
			fs.StringVar(&tr.from, "from", "", "RFC3339 window start")
			fs.StringVar(&tr.to, "to", "", "RFC3339 window end (default now)")
			fs.IntVar(&s.Limit, "limit", 200, "newest N matches")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			from, to, err := resolveTimeRange(tr, time.Now())
			if err != nil {
				return nil, nil, err
			}
			s.From, s.To, s.Fields = from, to, fields
			res, err := c.SearchAppLogs(ctx, pos[0], s)
			return res, func(w io.Writer) { printLogSearch(w, res) }, err
		},
	}
}

func printLogSearch(w io.Writer, r apiclient.LogSearchResource) {
	for _, e := range r.Entries {
		_, _ = fmt.Fprintf(w, "%s %s %s\n", e.Timestamp.UTC().Format(time.RFC3339), e.Stream, e.Message)
	}
	_, _ = fmt.Fprintf(w, "# showing %d of %d matching lines", len(r.Entries), r.Total)
	if len(r.Containers) > 1 {
		ids := make([]string, 0, len(r.Containers))
		for _, c := range r.Containers {
			id := c.ID
			if len(id) > 12 {
				id = id[:12]
			}
			ids = append(ids, fmt.Sprintf("%s=%d", id, c.Count))
		}
		_, _ = fmt.Fprintf(w, ", containers: %s", strings.Join(ids, " "))
	}
	_, _ = fmt.Fprintln(w)
}

func logsFailureCommand() apiCmd {
	return apiCmd{
		label: "logs failure",
		usage: logsUsage,
		args:  1,
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			res, err := c.GetFailureContext(ctx, pos[0])
			return res, func(w io.Writer) { printFailureContext(w, res) }, err
		},
	}
}

func printFailureContext(w io.Writer, r apiclient.FailureContextResource) {
	switch r.State {
	case "healthy":
		_, _ = fmt.Fprintln(w, "healthy: no crashloop and the newest deploy did not fail")
		return
	case "crashlooping":
		_, _ = fmt.Fprintf(w, "crashlooping: %d restarts in the last %s\n", r.Restarts, time.Duration(r.WindowSeconds*float64(time.Second)))
	default:
		_, _ = fmt.Fprintln(w, "newest deploy failed")
	}
	if r.Deploy != nil {
		_, _ = fmt.Fprintf(w, "deploy %s (%s) %s\n", r.Deploy.ID, r.Deploy.Image, r.Deploy.Error)
	}
	for _, l := range r.Lines {
		_, _ = fmt.Fprintf(w, "%s %s\n", l.Timestamp.UTC().Format(time.RFC3339), l.Message)
	}
	_, _ = fmt.Fprintf(w, "# last %d of %d lines (%s)\n", len(r.Lines), r.TotalLines, r.LinesSource)
}
