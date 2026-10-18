package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/pipeline"
)

func runPipelinesTemplates(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines templates", "print the templates as JSON", stdout, stderr)
	out := c.fs.String("out", "", "write the template's pipeline file to this path")
	create := c.fs.String("create", "", "save the template as a pipeline on this app")
	pname := c.fs.String("name", "", "pipeline name when using --create (default: the template's own)")
	client, pos, of, jsonOut, code, ok := c.parse(args, 0, 1, lookupEnv)
	if !ok {
		return code
	}
	if len(pos) == 0 {
		list := pipeline.Catalog()
		return c.write(of, list, func() {
			tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ID\tLANGUAGE\tNAME\tDESCRIPTION")
			for _, t := range list {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", t.ID, t.Language, t.Name, t.Description)
			}
			_ = tw.Flush()
		})
	}
	tpl, err := pipeline.CatalogByID(pos[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s", err.Error()))
	}
	if *out != "" {
		if err := os.WriteFile(*out, []byte(tpl.YAML), 0o600); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("write %s: %w", *out, err))
		}
	}
	if *create != "" {
		name := firstNonEmptyString(*pname, mustDefName(tpl.YAML), tpl.ID)
		saved, err := client.CreatePipeline(context.Background(), *create, apiclient.PipelineSaveRequest{Name: name, YAML: tpl.YAML})
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("create pipeline %q on app %q: %w", name, *create, err))
		}
		return c.write(of, saved, func() { _, _ = fmt.Fprintf(stdout, "created pipeline %s on %s\n", saved.Name, *create) })
	}
	return c.write(of, tpl, func() {
		if *out != "" {
			_, _ = fmt.Fprintf(stdout, "wrote %s\n", *out)
			return
		}
		_, _ = fmt.Fprint(stdout, tpl.YAML)
	})
}

func mustDefName(y string) string {
	def, err := pipeline.Parse([]byte(y))
	if err != nil {
		return ""
	}
	return def.Name
}

func runPipelinesFixes(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines fixes", "print the fixes as JSON", stdout, stderr)
	apply := c.fs.String("apply", "", "apply an automatic fix kind (add_retries, raise_timeout, raise_memory) to --job")
	job := c.fs.String("job", "", "job to apply the fix to")
	rerun := c.fs.Bool("rerun", false, "start a new run after applying the fix")
	client, pos, of, jsonOut, code, ok := c.parse(args, 2, 2, lookupEnv)
	if !ok {
		return code
	}
	ctx := context.Background()
	if *apply != "" {
		if *job == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--job is required with --apply"))
		}
		res, err := client.ApplyPipelineRunFix(ctx, pos[0], pos[1], *apply, *job, *rerun)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("apply fix to run %q: %w", pos[1], err))
		}
		return c.write(of, res, func() {
			_, _ = fmt.Fprintf(stdout, "updated pipeline %s\n", res.Pipeline)
			if res.Run != "" {
				_, _ = fmt.Fprintf(stdout, "started run %s\n", res.Run)
			}
		})
	}
	fixes, err := client.ListPipelineRunFixes(ctx, pos[0], pos[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list fixes for run %q: %w", pos[1], err))
	}
	return c.write(of, fixes, func() {
		if len(fixes) == 0 {
			_, _ = fmt.Fprintln(stdout, "no known fix matches this run")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "JOB\tFIX\tKIND\tHINT")
		for _, f := range fixes {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", f.Job, f.Code, f.Kind, f.Hint)
		}
		_ = tw.Flush()
	})
}

func runPipelinesInsights(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines insights", "print the insights as JSON", stdout, stderr)
	client, pos, of, jsonOut, code, ok := c.parse(args, 2, 2, lookupEnv)
	if !ok {
		return code
	}
	in, err := client.GetPipelineInsights(context.Background(), pos[0], pos[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("pipeline insights for %q: %w", pos[1], err))
	}
	return c.write(of, in, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "JOB\tRUNS\tFAIL%\tP50\tLAST\tFLAKY")
		for _, j := range in.Jobs {
			flaky := ""
			if j.Flaky {
				flaky = j.FlakyReason
			}
			_, _ = fmt.Fprintf(tw, "%s\t%d\t%.0f\t%ds\t%ds\t%s\n", j.Job, j.Runs, j.FailureRate*100, j.P50Seconds, j.LastSeconds, flaky)
		}
		_ = tw.Flush()
		if len(in.Diff) == 0 {
			return
		}
		_, _ = fmt.Fprintf(stdout, "\nlatest run vs run %s\n", in.DiffBaseID)
		tw = tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "JOB\tBEFORE\tNOW\tCHANGE")
		for _, d := range in.Diff {
			_, _ = fmt.Fprintf(tw, "%s\t%ds\t%ds\t%+ds (%+.0f%%)\n", d.Job, d.BaselineSeconds, d.CurrentSeconds, d.DeltaSeconds, d.DeltaPercent)
		}
		_ = tw.Flush()
	})
}
