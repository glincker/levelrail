package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type pipelineRunsAllFlags struct {
	all     *bool
	status  *string
	app     *string
	trigger *string
}

func addPipelineRunsAllFlags(c *pipelineCmd) pipelineRunsAllFlags {
	return pipelineRunsAllFlags{
		all:     c.fs.Bool("all", false, "list runs across every app"),
		status:  c.fs.String("status", "", "with --all: running, failed, succeeded, cancelled, waiting_approval, or held"),
		app:     c.fs.String("app", "", "with --all: only this app"),
		trigger: c.fs.String("trigger", "", "with --all: push, pull_request, tag, manual, schedule, or api"),
	}
}

func (c *pipelineCmd) listAllRuns(client *Client, f pipelineRunsAllFlags, pipeline string, limit int, of outputFlags, jsonOut bool) int {
	page, err := client.ListAllPipelineRuns(context.Background(), apiclient.PipelineRunsQuery{
		Status: *f.status, App: *f.app, Pipeline: pipeline, Trigger: *f.trigger, Limit: limit,
	})
	if err != nil {
		return reportError(c.stdout, c.stderr, jsonOut, fmt.Errorf("list runs across apps: %w", err))
	}
	return c.write(of, page.Runs, func() { printPipelineRunRows(c.stdout, page.Runs) })
}

func printPipelineRunRows(out io.Writer, runs []apiclient.PipelineRunRow) {
	if len(runs) == 0 {
		_, _ = fmt.Fprintln(out, "no runs")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tAPP\tPIPELINE\t#\tSTATUS\tTRIGGER\tREF\tSHA\tSTARTED\tDURATION")
	for _, r := range runs {
		status := r.Status
		switch {
		case r.HoldPending:
			status += " (held)"
		case r.ApprovalPending:
			status += " (approval)"
		}
		dur := "-"
		if r.DurationSeconds != nil {
			dur = (time.Duration(*r.DurationSeconds) * time.Second).String()
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\n", r.ID, r.App, r.Pipeline, r.Number, status, r.Trigger, r.Ref, r.ShortSHA,
			r.CreatedAt.Local().Format(time.DateTime), dur)
	}
	_ = tw.Flush()
}
