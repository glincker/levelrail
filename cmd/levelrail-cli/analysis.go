package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAnalysis dispatches `analysis list <app>`, `analysis show <id>` (read only) and `analysis run <app>`.
func runAnalysis(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s analysis list <app> [--json]\n  %[1]s analysis show <id> [--json]\n  %[1]s analysis run <app> [--json]\n", prog)
		return 2
	}
	switch args[0] {
	case "list":
		return runAnalysisList(prog, args[1:], stdout, stderr, lookupEnv)
	case "show":
		return runAnalysisShow(prog, args[1:], stdout, stderr, lookupEnv)
	case "run":
		return runAnalysisRun(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "unknown analysis command %q\n", args[0])
	return 2
}

func runAnalysisList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "analysis list", "print the reports as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s analysis list <app> [flags]\n\nLists an app's incident and cost analysis reports, newest first.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, app, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "analysis list", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	rows, err := client.ListAnalysisReports(context.Background(), app)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list analysis reports for app %q: %w", app, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rows, func() {
		if len(rows) == 0 {
			_, _ = fmt.Fprintln(stdout, "No analysis reports.")
			return
		}
		for _, r := range rows {
			_, _ = fmt.Fprintf(stdout, "%s  %s  %-17s %-5s %s\n", r.ID, r.CreatedAt.Format("2006-01-02 15:04"), r.Status, r.TriggerKind, r.Summary)
		}
	})
}

func runAnalysisShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "analysis show", "print the report as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s analysis show <id> [flags]\n\nShows one analysis report: summary, timeline, ranked causes and recommendations.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, id, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "analysis show", "report id"}, lookupEnv)
	if !ok {
		return exitCode
	}
	rep, err := client.GetAnalysisReport(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get analysis report %q: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rep, func() { printAnalysisReport(stdout, rep) })
}

func runAnalysisRun(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "analysis run", "print the report as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s analysis run <app> [flags]\n\nGenerates an analysis report for an app now. Needs APP_ANALYSIS_ENABLED on the control plane.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, app, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "analysis run", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	rep, err := client.RunAnalysis(context.Background(), app)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("run analysis for app %q: %w", app, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rep, func() { printAnalysisReport(stdout, rep) })
}

func printAnalysisReport(w io.Writer, rep apiclient.AnalysisReport) {
	var body struct {
		Timeline []struct {
			At     string `json:"at"`
			Kind   string `json:"kind"`
			Detail string `json:"detail"`
		} `json:"timeline"`
		Causes []struct {
			Rank       int      `json:"rank"`
			Title      string   `json:"title"`
			Confidence string   `json:"confidence"`
			Evidence   []string `json:"evidence"`
		} `json:"causes"`
		Recommendations []struct {
			Text string `json:"text"`
		} `json:"recommendations"`
	}
	_ = json.Unmarshal(rep.Report, &body)
	_, _ = fmt.Fprintf(w, "%s  %s  %s\n%s\n", rep.ID, rep.App, rep.Status, rep.Summary)
	if len(body.Timeline) > 0 {
		_, _ = fmt.Fprintln(w, "\nTimeline")
		for _, e := range body.Timeline {
			_, _ = fmt.Fprintf(w, "  %s  %-8s %s\n", e.At, e.Kind, e.Detail)
		}
	}
	if len(body.Causes) > 0 {
		_, _ = fmt.Fprintln(w, "\nSuspected causes")
		for _, c := range body.Causes {
			_, _ = fmt.Fprintf(w, "  %d. %s (%s) evidence: %v\n", c.Rank, c.Title, c.Confidence, c.Evidence)
		}
	}
	if len(body.Recommendations) > 0 {
		_, _ = fmt.Fprintln(w, "\nRecommendations")
		for _, r := range body.Recommendations {
			_, _ = fmt.Fprintf(w, "  - %s\n", r.Text)
		}
	}
}
