package mcptools

import (
	"context"
	"fmt"
	"sort"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/benchreport"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type resilienceInput struct {
	RunID     string `json:"run_id,omitempty"`
	CompareTo string `json:"compare_to,omitempty"`
}

type resilienceOutput struct {
	Results []string `json:"results" jsonschema:"one line per scenario"`
}

const maxResilienceDeltaLines = 60

func registerResilienceTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_resilience_report",
		Description: "Chaos results and benchmark runs; run_id shows one, compare_to diffs two.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in resilienceInput) (*mcp.CallToolResult, resilienceOutput, error) {
		if in.RunID != "" {
			return benchmarkLines(ctx, client, in)
		}
		rep, err := client.GetResilienceReport(ctx)
		if err != nil {
			return nil, resilienceOutput{}, fmt.Errorf("get resilience report: %w", err)
		}
		out := resilienceOutput{Results: []string{}}
		for _, r := range rep.Latest {
			out.Results = append(out.Results, fmt.Sprintf("%s %s: recovery %.0f ms, %d of %d requests failed, %s", r.Scenario, r.Status, r.RecoveryMS, r.RequestErrors, r.Requests, r.RanAt.Format("2006-01-02")))
		}
		for _, n := range rep.Unmeasured {
			out.Results = append(out.Results, n+" not yet measured")
		}
		if runs, err := client.ListBenchmarks(ctx, 10); err == nil {
			for _, r := range runs.Runs {
				out.Results = append(out.Results, fmt.Sprintf("benchmark %s %s commit %.10s on %s: %d passed, %d failed", r.ID, r.GeneratedAt.Format("2006-01-02"), r.Commit, r.HostSummary, r.Passed, r.Failed))
			}
		}
		return nil, out, nil
	})
}

func benchmarkLines(ctx context.Context, client *apiclient.Client, in resilienceInput) (*mcp.CallToolResult, resilienceOutput, error) {
	if in.CompareTo != "" {
		c, err := client.CompareBenchmarks(ctx, in.CompareTo, in.RunID)
		if err != nil {
			return nil, resilienceOutput{}, fmt.Errorf("compare benchmark runs: %w", err)
		}
		out := resilienceOutput{Results: []string{fmt.Sprintf("compare base %s (%s) to head %s (%s), %s", c.Base.ID, c.Base.Commit, c.Head.ID, c.Head.Commit, c.Statistic)}}
		for _, w := range c.Warnings {
			out.Results = append(out.Results, "warning: "+w)
		}
		for _, s := range c.Status {
			out.Results = append(out.Results, fmt.Sprintf("%s status %s to %s", s.Scenario, s.Base, s.Head))
		}
		deltas := make([]benchreport.Delta, 0, len(c.Deltas))
		for _, d := range c.Deltas {
			if d.Direction == benchreport.DirectionBetter || d.Direction == benchreport.DirectionWorse {
				deltas = append(deltas, d)
			}
		}
		sort.SliceStable(deltas, func(i, j int) bool {
			return deltas[i].Direction == benchreport.DirectionWorse && deltas[j].Direction != benchreport.DirectionWorse
		})
		for i, d := range deltas {
			if i == maxResilienceDeltaLines {
				out.Results = append(out.Results, fmt.Sprintf("%d more changed metrics not shown", len(deltas)-i))
				break
			}
			pct := "n/a"
			if d.DeltaPct != nil {
				pct = fmt.Sprintf("%+.1f%%", *d.DeltaPct)
			}
			out.Results = append(out.Results, fmt.Sprintf("%s %s %.2f to %.2f (%s) %s", d.Scenario, d.Metric, *d.Base, *d.Head, pct, d.Direction))
		}
		return nil, out, nil
	}
	run, err := client.GetBenchmark(ctx, in.RunID)
	if err != nil {
		return nil, resilienceOutput{}, fmt.Errorf("get benchmark run: %w", err)
	}
	rep := run.Report
	out := resilienceOutput{Results: []string{fmt.Sprintf("%s version %s commit %s on %s, %d run(s) per scenario", run.Run.ID, rep.Build.Version, rep.Build.Commit, rep.Host.Summary(), rep.Repeat)}}
	for _, c := range rep.Caveats {
		out.Results = append(out.Results, "caveat: "+c)
	}
	for _, r := range rep.Results {
		out.Results = append(out.Results, fmt.Sprintf("%s %s: recovery %.0f ms, %d of %d requests failed, %d metrics", r.Scenario, r.Status, r.RecoveryMS, r.RequestErrors, r.Requests, len(r.Metrics)))
	}
	for _, l := range rep.Limits {
		out.Results = append(out.Results, "limit: "+l.Question+": "+l.Status)
	}
	return nil, out, nil
}
