package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"
)

func runPipelinesSync(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines sync", "print the result as JSON", stdout, stderr)
	truth := c.fs.String("repo-truth", "", "set whether the repository overwrites pipelines edited in the dashboard (true or false); no sync is run")
	client, pos, of, jsonOut, code, ok := c.parse(args, 1, 1, lookupEnv)
	if !ok {
		return code
	}
	ctx := context.Background()
	if *truth != "" {
		if *truth != "true" && *truth != "false" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--repo-truth must be true or false"))
		}
		st, err := client.SetPipelineSync(ctx, pos[0], *truth == "true")
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set pipeline sync for app %q: %w", pos[0], err))
		}
		return c.write(of, st, func() { _, _ = fmt.Fprintf(stdout, "repository is source of truth: %t\n", st.RepoIsTruth) })
	}
	res, err := client.RunPipelineSync(ctx, pos[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("sync pipelines for app %q: %w", pos[0], err))
	}
	return c.write(of, res, func() {
		_, _ = fmt.Fprintf(stdout, "synced from %s", res.SHA)
		if res.Dir != "" {
			_, _ = fmt.Fprintf(stdout, " (%s)", res.Dir)
		}
		_, _ = fmt.Fprintln(stdout)
		if len(res.Items) == 0 {
			_, _ = fmt.Fprintln(stdout, "no pipeline files found")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "FILE\tPIPELINE\tOUTCOME\tNOTE")
		for _, it := range res.Items {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", it.File, it.Name, it.Outcome, it.Message)
		}
		_ = tw.Flush()
	})
}

func runPipelinesTriggers(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines triggers", "print the decisions as JSON", stdout, stderr)
	client, pos, of, jsonOut, code, ok := c.parse(args, 1, 1, lookupEnv)
	if !ok {
		return code
	}
	list, err := client.ListPipelineTriggers(context.Background(), pos[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list pipeline triggers for app %q: %w", pos[0], err))
	}
	return c.write(of, list, func() {
		if len(list) == 0 {
			_, _ = fmt.Fprintln(stdout, "no trigger decisions recorded")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "TIME\tEVENT\tPIPELINE\tDECISION\tREASON")
		for _, e := range list {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", e.CreatedAt.Local().Format(time.DateTime), e.Event, e.Pipeline, e.Decision, e.Reason)
		}
		_ = tw.Flush()
	})
}
