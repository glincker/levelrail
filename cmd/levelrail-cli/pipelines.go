package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/pipeline"
)

// kvFlag collects repeated key=value flags.
type kvFlag map[string]string

func (k kvFlag) String() string { return fmt.Sprint(map[string]string(k)) }

func (k kvFlag) Set(v string) error {
	key, val, ok := strings.Cut(v, "=")
	if !ok || key == "" {
		return fmt.Errorf("expected key=value, got %q", v)
	}
	k[key] = val
	return nil
}

func pipelinesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s pipelines list <app> [flags]                       list an app's pipelines
  %[1]s pipelines validate <file> [--json]                 validate a pipeline file locally (no API call)
  %[1]s pipelines save <app> <file-or-repo-dir> [--name N] [flags]   create or update pipelines from a file, or every file in a repo's pipeline directory
  %[1]s pipelines delete <app> <name> [flags]              delete a pipeline
  %[1]s pipelines run <app> <name> [--ref R] [--sha S] [--input k=v]... [--follow] [flags]
  %[1]s pipelines runs <app> [<run-id>] [--pipeline N] [--limit N] [flags]   list runs, or show one run's jobs and steps
  %[1]s pipelines runs --all [--status S] [--app A] [--trigger T] [--pipeline N] [--limit N] [flags]   list runs across every app
  %[1]s pipelines logs <app> <run-id> [--job KEY] [--follow] [flags]
  %[1]s pipelines cancel <app> <run-id> [flags]
  %[1]s pipelines approve <app> <run-id> [--reject] [--comment TEXT] [--approval ID] [flags]   decide approval gates, or release a run held for approval
  %[1]s pipelines sync <app> [--repo-truth=true|false] [flags]   sync pipeline files from the repository now, or set repository as source of truth
  %[1]s pipelines triggers <app> [flags]                   why recent git events did or did not start runs

Pipelines live in the control plane per app, or in the repository under a
pipeline directory. Run "%[1]s pipelines <subcommand> -h" for its flags.
`, prog)
}

func runPipelines(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, pipelinesUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, pipelinesUsage(prog))
		return exitOK
	case "list":
		return runPipelinesList(prog, rest, stdout, stderr, lookupEnv)
	case "validate":
		return runPipelinesValidate(prog, rest, stdout, stderr)
	case "save":
		return runPipelinesSave(prog, rest, stdout, stderr, lookupEnv)
	case "delete":
		return runPipelinesDelete(prog, rest, stdout, stderr, lookupEnv)
	case "run":
		return runPipelinesRun(prog, rest, stdout, stderr, lookupEnv)
	case "runs":
		return runPipelinesRuns(prog, rest, stdout, stderr, lookupEnv)
	case "logs":
		return runPipelinesLogs(prog, rest, stdout, stderr, lookupEnv)
	case "cancel":
		return runPipelinesCancel(prog, rest, stdout, stderr, lookupEnv)
	case "approve":
		return runPipelinesApprove(prog, rest, stdout, stderr, lookupEnv)
	case "sync":
		return runPipelinesSync(prog, rest, stdout, stderr, lookupEnv)
	case "triggers":
		return runPipelinesTriggers(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown pipelines subcommand %q\n\n%s", prog, args[0], pipelinesUsage(prog))
	return exitUsage
}

type pipelineCmd struct {
	fs     *flag.FlagSet
	ptrs   apiFlagPtrs
	prog   string
	label  string
	stdout io.Writer
	stderr io.Writer
}

func newPipelineCmd(prog, label, jsonHelp string, stdout, stderr io.Writer) *pipelineCmd {
	fs, t, a, p, j, o, q := apiFlagSet(prog, label, jsonHelp, stderr)
	c := &pipelineCmd{fs: fs, ptrs: apiFlagPtrs{t, a, p, j, o, q}, prog: prog, label: label, stdout: stdout, stderr: stderr}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pipelinesUsage(prog)) }
	return c
}

// parse resolves flags and exactly n positional arguments (or between
// min and max when they differ), returning a ready API client.
func (c *pipelineCmd) parse(args []string, minArgs, maxArgs int, lookupEnv func(string) (string, bool)) (client *Client, pos []string, of outputFlags, jsonOut bool, code int, ok bool) {
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(c.fs, args, c.ptrs, c.prog, c.stderr)
	if !ok {
		return nil, nil, of, jsonOut, code, false
	}
	pos = c.fs.Args()
	if len(pos) < minArgs || len(pos) > maxArgs {
		_, _ = fmt.Fprintf(c.stderr, "%s: %s takes %d to %d arguments\n\n", c.prog, c.label, minArgs, maxArgs)
		c.fs.Usage()
		return nil, nil, of, jsonOut, exitUsage, false
	}
	return apiClientFromFlags(c.prog, apiURL, token, profile, lookupEnv), pos, of, jsonOut, exitOK, true
}

func (c *pipelineCmd) write(of outputFlags, value any, human func()) int {
	return writeScheduledTaskResult(c.stdout, c.stderr, of, value, human)
}

func runPipelinesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines list", "print pipelines as JSON", stdout, stderr)
	client, pos, of, jsonOut, code, ok := c.parse(args, 1, 1, lookupEnv)
	if !ok {
		return code
	}
	list, err := client.ListPipelines(context.Background(), pos[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list pipelines for app %q: %w", pos[0], err))
	}
	return c.write(of, list, func() {
		if len(list) == 0 {
			_, _ = fmt.Fprintln(stdout, "no pipelines")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "NAME\tENABLED\tTRIGGERS\tJOBS\tLAST RUN")
		for _, p := range list {
			last := "never"
			if p.LastRun != nil {
				last = fmt.Sprintf("#%d %s", p.LastRun.Number, p.LastRun.Status)
			}
			_, _ = fmt.Fprintf(tw, "%s\t%t\t%s\t%d\t%s\n", p.Name, p.Enabled, strings.Join(p.Triggers, ","), p.Jobs, last)
		}
		_ = tw.Flush()
	})
}

func runPipelinesValidate(prog string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(prog+" pipelines validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "print the validation result as JSON")
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: pipelines validate requires exactly one file\n", prog)
		return exitUsage
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return reportError(stdout, stderr, *jsonOut, fmt.Errorf("read %s: %w", fs.Arg(0), err))
	}
	_, issues := pipeline.Validate(data)
	if *jsonOut {
		if issues == nil {
			issues = []pipeline.Issue{}
		}
		_ = writeJSONValue(stdout, map[string]any{"valid": len(issues) == 0, "issues": issues})
	} else if len(issues) == 0 {
		_, _ = fmt.Fprintf(stdout, "%s is valid\n", fs.Arg(0))
	} else {
		for _, i := range issues {
			_, _ = fmt.Fprintf(stderr, "%s:%s\n", fs.Arg(0), i)
		}
	}
	if len(issues) > 0 {
		return exitValidation
	}
	return exitOK
}

func runPipelinesSave(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines save", "print the saved pipelines as JSON", stdout, stderr)
	name := c.fs.String("name", "", "pipeline name for a single file (default: the file's name field)")
	client, pos, of, jsonOut, code, ok := c.parse(args, 2, 2, lookupEnv)
	if !ok {
		return code
	}
	ctx := context.Background()
	files := []string{pos[1]}
	if info, err := os.Stat(pos[1]); err == nil && info.IsDir() { //nolint:gosec // operator-supplied local path on their own CLI
		brandName, _ := client.BrandShortName(ctx)
		found, derr := pipeline.Discover(pos[1], brandName)
		if derr != nil || len(found) == 0 {
			return reportError(stdout, stderr, jsonOut, newValidationError("no pipeline files found under %s", pos[1]))
		}
		files, *name = found, ""
	}
	var saved []apiclient.PipelineResource
	for _, f := range files {
		p, err := savePipelineFile(ctx, client, pos[0], f, *name, stderr)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		saved = append(saved, p)
	}
	return c.write(of, saved, func() {
		for _, p := range saved {
			_, _ = fmt.Fprintf(stdout, "pipeline %q saved for app %q\n", p.Name, pos[0])
		}
	})
}

func savePipelineFile(ctx context.Context, client *Client, app, path, nameOverride string, stderr io.Writer) (apiclient.PipelineResource, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied CLI argument
	if err != nil {
		return apiclient.PipelineResource{}, fmt.Errorf("read %s: %w", path, err)
	}
	def, issues := pipeline.Validate(data)
	if len(issues) > 0 {
		for _, i := range issues {
			_, _ = fmt.Fprintf(stderr, "%s:%s\n", path, i)
		}
		return apiclient.PipelineResource{}, newValidationError("%s is not a valid pipeline", path)
	}
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	pname := firstNonEmptyString(nameOverride, def.Name, stem)
	req := apiclient.PipelineSaveRequest{Name: pname, YAML: string(data)}
	saved, err := client.UpdatePipeline(ctx, app, pname, req)
	if err != nil && isNotFound(err) {
		saved, err = client.CreatePipeline(ctx, app, req)
	}
	if err != nil {
		return apiclient.PipelineResource{}, fmt.Errorf("save pipeline %q: %w", pname, err)
	}
	return saved, nil
}

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func isNotFound(err error) bool {
	var ae *apiclient.APIError
	return errors.As(err, &ae) && ae.StatusCode == 404
}

func runPipelinesDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines delete", "print the result as JSON", stdout, stderr)
	client, pos, of, jsonOut, code, ok := c.parse(args, 2, 2, lookupEnv)
	if !ok {
		return code
	}
	if err := client.DeletePipeline(context.Background(), pos[0], pos[1]); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete pipeline %q: %w", pos[1], err))
	}
	return c.write(of, map[string]string{"deleted": pos[1]}, func() { _, _ = fmt.Fprintf(stdout, "pipeline %q deleted\n", pos[1]) })
}

func runPipelinesRun(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines run", "print the started run as JSON", stdout, stderr)
	ref := c.fs.String("ref", "", "git ref the run reports, e.g. refs/heads/main")
	sha := c.fs.String("sha", "", "commit SHA to check out and tag builds with")
	follow := c.fs.Bool("follow", false, "stream the run's logs until it finishes")
	inputs := kvFlag{}
	c.fs.Var(inputs, "input", "manual input as key=value (repeatable)")
	client, pos, of, jsonOut, code, ok := c.parse(args, 2, 2, lookupEnv)
	if !ok {
		return code
	}
	run, err := client.StartPipelineRun(context.Background(), pos[0], pos[1], apiclient.PipelineStartRequest{Ref: *ref, SHA: *sha, Inputs: inputs})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("start pipeline %q: %w", pos[1], err))
	}
	if code := c.write(of, run, func() { _, _ = fmt.Fprintf(stdout, "run #%d started (%s)\n", run.Number, run.ID) }); code != exitOK || !*follow {
		return code
	}
	return followPipelineRun(client, pos[0], run.ID, "", stdout, stderr, jsonOut)
}

func followPipelineRun(client *Client, app, runID, job string, stdout, stderr io.Writer, jsonOut bool) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := client.StreamPipelineRunLogs(ctx, app, runID, job, func(l apiclient.PipelineLogLine) error {
		if jsonOut {
			return writeJSONLine(stdout, l)
		}
		_, werr := fmt.Fprintf(stdout, "[%s] %s\n", l.Job, l.Line)
		return werr
	})
	if err != nil && ctx.Err() == nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("stream logs: %w", err))
	}
	final, gerr := client.GetPipelineRun(context.Background(), app, runID)
	if gerr != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read final run state: %w", gerr))
	}
	if !jsonOut {
		_, _ = fmt.Fprintf(stdout, "run #%d %s: %s\n", final.Number, final.Status, final.Reason)
	}
	if final.Status == "failed" || final.Status == "cancelled" {
		return exitNetwork
	}
	return exitOK
}

func runPipelinesRuns(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines runs", "print runs as JSON", stdout, stderr)
	name := c.fs.String("pipeline", "", "only runs of this pipeline")
	limit := c.fs.Int("limit", 20, "maximum runs to list")
	allFlags := addPipelineRunsAllFlags(c)
	client, pos, of, jsonOut, code, ok := c.parse(args, 0, 2, lookupEnv)
	if !ok {
		return code
	}
	if *allFlags.all {
		if len(pos) > 0 {
			_, _ = fmt.Fprintf(stderr, "%s: pipelines runs --all takes no app argument, use --app to filter\n", prog)
			return exitUsage
		}
		return c.listAllRuns(client, allFlags, *name, *limit, of, jsonOut)
	}
	if len(pos) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: pipelines runs needs an app name, or --all for every app\n\n", prog)
		c.fs.Usage()
		return exitUsage
	}
	ctx := context.Background()
	if len(pos) == 2 {
		run, err := client.GetPipelineRun(ctx, pos[0], pos[1])
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get run %q: %w", pos[1], err))
		}
		return c.write(of, run, func() { printPipelineRun(stdout, run) })
	}
	runs, err := client.ListPipelineRuns(ctx, pos[0], *name, *limit)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list runs for app %q: %w", pos[0], err))
	}
	return c.write(of, runs, func() {
		if len(runs) == 0 {
			_, _ = fmt.Fprintln(stdout, "no runs")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tPIPELINE\t#\tSTATUS\tTRIGGER\tREF\tSTARTED\tREASON")
		for _, r := range runs {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n", r.ID, r.PipelineName, r.Number, r.Status, r.Trigger, r.Ref, r.CreatedAt.Local().Format(time.DateTime), r.Reason)
		}
		_ = tw.Flush()
	})
}

func printPipelineRun(out io.Writer, r apiclient.PipelineRunResource) {
	_, _ = fmt.Fprintf(out, "run #%d of %s: %s (%s)\n", r.Number, r.PipelineName, r.Status, r.Reason)
	for _, j := range r.Jobs {
		_, _ = fmt.Fprintf(out, "  job %s: %s %s\n", j.Key, j.Status, j.Reason)
		for _, s := range j.Steps {
			_, _ = fmt.Fprintf(out, "    %d. %s [%s]: %s %s\n", s.Index, s.Name, s.Kind, s.Status, s.Reason)
		}
	}
	for _, a := range r.Approvals {
		state := firstNonEmptyString(a.Decision, "pending")
		_, _ = fmt.Fprintf(out, "  approval %d (%s, needs %s): %s\n", a.ID, a.Job, a.RequiredAbility, state)
	}
}

func runPipelinesLogs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines logs", "print log lines as JSON", stdout, stderr)
	job := c.fs.String("job", "", "only this job key")
	follow := c.fs.Bool("follow", false, "keep streaming until the run finishes")
	client, pos, of, jsonOut, code, ok := c.parse(args, 2, 2, lookupEnv)
	if !ok {
		return code
	}
	if *follow {
		return followPipelineRun(client, pos[0], pos[1], *job, stdout, stderr, jsonOut)
	}
	lines, err := client.ListPipelineRunLogs(context.Background(), pos[0], pos[1], *job, 0, 5000)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get logs: %w", err))
	}
	return c.write(of, lines, func() {
		for _, l := range lines {
			_, _ = fmt.Fprintf(stdout, "[%s] %s\n", l.Job, l.Line)
		}
	})
}

func runPipelinesCancel(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines cancel", "print the result as JSON", stdout, stderr)
	client, pos, of, jsonOut, code, ok := c.parse(args, 2, 2, lookupEnv)
	if !ok {
		return code
	}
	if err := client.CancelPipelineRun(context.Background(), pos[0], pos[1]); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("cancel run %q: %w", pos[1], err))
	}
	return c.write(of, map[string]string{"status": "cancelling"}, func() { _, _ = fmt.Fprintf(stdout, "run %s is being cancelled\n", pos[1]) })
}

func runPipelinesApprove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines approve", "print the result as JSON", stdout, stderr)
	reject := c.fs.Bool("reject", false, "reject the gate instead of approving it")
	comment := c.fs.String("comment", "", "comment recorded with the decision")
	only := c.fs.Int64("approval", 0, "decide only this approval id (default: every pending gate of the run)")
	client, pos, of, jsonOut, code, ok := c.parse(args, 2, 2, lookupEnv)
	if !ok {
		return code
	}
	ctx := context.Background()
	run, err := client.GetPipelineRun(ctx, pos[0], pos[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get run %q: %w", pos[1], err))
	}
	decision := "approved"
	if *reject {
		decision = "rejected"
	}
	if run.Hold != nil && run.Hold.State == "pending" && *only == 0 {
		if err := client.DecidePipelineRunHold(ctx, pos[0], pos[1], decision); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("decide hold on run %q: %w", pos[1], err))
		}
		return c.write(of, map[string]string{"decision": decision, "hold": run.Hold.Reason}, func() {
			_, _ = fmt.Fprintf(stdout, "%s run %s held for approval (%s)\n", decision, pos[1], run.Hold.Reason)
		})
	}
	var decided []int64
	for _, a := range run.Approvals {
		if a.Decision != "" || (*only != 0 && a.ID != *only) {
			continue
		}
		if err := client.DecidePipelineApproval(ctx, pos[0], pos[1], a.ID, decision, *comment); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("decide approval %d: %w", a.ID, err))
		}
		decided = append(decided, a.ID)
	}
	if len(decided) == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("run %s has no pending approval gates", pos[1]))
	}
	return c.write(of, map[string]any{"decision": decision, "approvals": decided}, func() {
		_, _ = fmt.Fprintf(stdout, "%s %d approval gate(s) on run %s\n", decision, len(decided), pos[1])
	})
}
