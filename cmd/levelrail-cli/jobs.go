package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const jobsUsage = `Usage:
  %[1]s jobs list <app>
  %[1]s jobs get <app> <job>
  %[1]s jobs create <app> <name> --schedule CRON [flags] -- <command> [args...]
  %[1]s jobs update <app> <job> [flags] [-- <command> [args...]]
  %[1]s jobs run <app> <job>
  %[1]s jobs run <app> --command "CMD" [--completions N] [--parallelism P] [--backoff-limit B] [--timeout D] [--wait]
  %[1]s jobs pause <app> <job>
  %[1]s jobs resume <app> <job>
  %[1]s jobs next --schedule CRON [--timezone ZONE] [--count N]
  %[1]s jobs history <app> [<job>] [--status STATUS] [--limit N]
  %[1]s jobs logs <app> <job> [--run RUN_ID]
  %[1]s jobs enable <app> <job>
  %[1]s jobs disable <app> <job>
  %[1]s jobs delete <app> <job>

A job runs a command, or in http mode calls a path on the app, on a cron
schedule (5 fields, or @hourly, @daily, @weekly, @monthly, @yearly). By default each run starts a fresh
container from the app's current image with the app's env, secrets, volumes
and network, and removes it afterwards (--mode exec runs it inside the
running container instead). <job> is the job's name or its ID. <command> is
a real argv, no shell involved; use "sh -c '...'" for a shell pipeline.

Flags for create and update:
  --schedule CRON        5-field cron expression
  --timezone ZONE        IANA zone the schedule is read in (default UTC)
  --mode MODE            fresh (default), exec or http
  --concurrency POLICY   allow, forbid (skip), replace or queue
  --http-path PATH       http mode: path on the app to call (wakes it if asleep)
  --http-method METHOD   http mode: GET, POST (default), PUT, PATCH or DELETE
  --http-body TEXT       http mode: request body
  --background           http mode: await the app under the longer
                         background timeout (APP_JOBS_BACKGROUND_TIMEOUT)
  --timeout DURATION     per-attempt timeout, e.g. 10m
  --retries N            retries after a failed attempt
  --retry-backoff DUR    first retry delay, doubled each retry
  --catch-up POLICY      skip (default) or run_once after control plane downtime
  --catch-up-lookback D  how far back run_once still applies
  --cpu CORES            CPU limit for a fresh container, e.g. 0.5
  --memory MB            memory limit in MiB for a fresh container
  --warn-after DURATION  alert when a run exceeds this
  --disabled             create the job disabled

Run "%[1]s apps alerts create <app> --kind scheduled_task_failure
--scheduled-task-id ID --restart-count-threshold 1" to be alerted on failure
(also kinds scheduled_task_missed and scheduled_task_running_long).
`

func runJobs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, jobsUsage, prog)
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprintf(stdout, jobsUsage, prog)
		return exitOK
	case "run":
		if wantsOneShot(args[1:]) {
			return runJobsOneShot(prog, args[1:], stdout, stderr, lookupEnv)
		}
		return runJobsVerb(prog, "run", args[1:], stdout, stderr, lookupEnv)
	case "list", "get", "create", "update", "history", "logs", "enable", "disable", "pause", "resume", "delete", "next":
		return runJobsVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown jobs subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprintf(stderr, jobsUsage, prog)
		return exitUsage
	}
}

type jobFlags struct {
	schedule, timezone, mode, concurrency, catchUp string
	timeout, retryBackoff, catchUpLookback, warn   time.Duration
	retries, memoryMB                              int
	cpu                                            float64
	disabled, background                           bool
	httpPath, httpMethod, httpBody                 string
	count                                          int
	status, runID                                  string
	limit                                          int
}

func (f jobFlags) apply(req *scheduledTaskRequest) {
	req.Schedule, req.Timezone, req.Mode, req.ConcurrencyPolicy, req.CatchUp = f.schedule, f.timezone, f.mode, f.concurrency, f.catchUp
	req.TimeoutSeconds, req.MaxRetries = int(f.timeout.Seconds()), f.retries
	req.RetryBackoffSeconds = int(f.retryBackoff.Seconds())
	req.CatchUpLookbackSeconds, req.WarnAfterSeconds = int(f.catchUpLookback.Seconds()), int(f.warn.Seconds())
	req.CPULimitMillis, req.MemoryLimitMB = int(f.cpu*1000), f.memoryMB
	if f.httpPath != "" || f.httpMethod != "" || f.httpBody != "" || f.background {
		req.HTTP = &apiclient.JobHTTP{Method: f.httpMethod, Path: f.httpPath, Body: f.httpBody, Background: f.background}
	}
}

func runJobsVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "jobs "+verb, "print the result as JSON to stdout and nothing else", stderr)
	var f jobFlags
	fs.StringVar(&f.schedule, "schedule", "", "5-field cron expression")
	fs.StringVar(&f.timezone, "timezone", "", "IANA time zone the schedule is read in")
	fs.StringVar(&f.mode, "mode", "", "fresh or exec")
	fs.StringVar(&f.concurrency, "concurrency", "", "allow, forbid or replace")
	fs.StringVar(&f.catchUp, "catch-up", "", "skip or run_once")
	fs.DurationVar(&f.timeout, "timeout", 0, "per-attempt timeout")
	fs.DurationVar(&f.retryBackoff, "retry-backoff", 0, "first retry delay")
	fs.DurationVar(&f.catchUpLookback, "catch-up-lookback", 0, "catch-up lookback window")
	fs.DurationVar(&f.warn, "warn-after", 0, "alert when a run exceeds this")
	fs.IntVar(&f.retries, "retries", 0, "retries after a failed attempt")
	fs.IntVar(&f.memoryMB, "memory", 0, "memory limit in MiB")
	fs.Float64Var(&f.cpu, "cpu", 0, "CPU limit in cores")
	fs.BoolVar(&f.disabled, "disabled", false, "create the job disabled")
	fs.StringVar(&f.httpPath, "http-path", "", "http mode: path to call on the app")
	fs.StringVar(&f.httpMethod, "http-method", "", "http mode: request method")
	fs.StringVar(&f.httpBody, "http-body", "", "http mode: request body")
	fs.BoolVar(&f.background, "background", false, "http mode: await under the background timeout")
	fs.IntVar(&f.count, "count", 5, "next: how many upcoming runs to print")
	fs.StringVar(&f.status, "status", "", "history: only runs with this status")
	fs.StringVar(&f.runID, "run", "", "logs: a specific run ID (default the latest)")
	fs.IntVar(&f.limit, "limit", 0, "history: at most this many runs")
	fs.Usage = func() { _, _ = fmt.Fprintf(stderr, jobsUsage, prog) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	pos := fs.Args()
	label := "jobs " + verb
	need := map[string]int{"list": 1, "get": 2, "create": 3, "update": 2, "run": 2, "history": 1, "logs": 2, "enable": 2, "disable": 2, "pause": 2, "resume": 2, "delete": 2, "next": 0}[verb]
	if verb == "create" && f.mode == "http" {
		need = 2
	}
	if len(pos) < need || (verb != "create" && verb != "update" && verb != "history" && len(pos) != need) || (verb == "history" && len(pos) > 2) {
		_, _ = fmt.Fprintf(stderr, "%s: %s: wrong number of arguments\n\n", prog, label)
		fs.Usage()
		return exitUsage
	}
	if verb == "next" {
		return jobsNext(f.schedule, f.timezone, f.count, time.Now(), stdout, stderr)
	}
	app := pos[0]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	fail := func(err error) int { return reportError(stdout, stderr, jsonOut, err) }

	switch verb {
	case "list":
		jobs, err := client.ListJobs(ctx, app)
		if err != nil {
			return fail(fmt.Errorf("list jobs for %q: %w", app, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, jobs, func() { printJobsTable(stdout, jobs) })
	case "get":
		job, err := client.GetJob(ctx, app, pos[1])
		if err != nil {
			return fail(fmt.Errorf("get job %q: %w", pos[1], err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, job, func() { printJob(stdout, job) })
	case "create":
		if f.schedule == "" {
			return fail(newValidationError("--schedule is required"))
		}
		if f.mode == "http" && f.httpPath == "" {
			return fail(newValidationError("--http-path is required with --mode http"))
		}
		if f.mode != "http" && len(pos) < 4 {
			return fail(newValidationError("a command is required after -- (jobs create <app> <name> --schedule CRON -- <command>)"))
		}
		req := scheduledTaskRequest{Name: pos[1], Command: pos[2:], Enabled: !f.disabled}
		f.apply(&req)
		job, err := client.CreateJob(ctx, app, req)
		if err != nil {
			return fail(fmt.Errorf("create job for %q: %w", app, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, job, func() {
			_, _ = fmt.Fprintf(stdout, "job %q created for app %q (id %s)\n", jobLabelOf(job), app, job.ID)
		})
	case "update":
		req := scheduledTaskRequest{}
		f.apply(&req)
		if len(pos) > 2 {
			req.Command = pos[2:]
		}
		current, err := client.GetJob(ctx, app, pos[1])
		if err != nil {
			return fail(fmt.Errorf("get job %q: %w", pos[1], err))
		}
		req.Enabled = current.Enabled
		if f.disabled {
			req.Enabled = false
		}
		if req.Command == nil {
			req.Command = current.Command
		}
		if req.Schedule == "" {
			req.Schedule = current.Schedule
		}
		job, err := client.UpdateJob(ctx, app, current.ID, req)
		if err != nil {
			return fail(fmt.Errorf("update job %q: %w", pos[1], err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, job, func() { _, _ = fmt.Fprintf(stdout, "job %q updated\n", jobLabelOf(job)) })
	case "run":
		job, err := client.RunJob(ctx, app, pos[1])
		if err != nil {
			return fail(fmt.Errorf("run job %q: %w", pos[1], err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, job, func() {
			_, _ = fmt.Fprintf(stdout, "job %q started (run %s); follow it with: %s jobs logs %s %s --run %s\n",
				jobLabelOf(job), job.RunID, prog, app, pos[1], job.RunID)
		})
	case "enable", "disable", "pause", "resume":
		on := verb == "enable" || verb == "resume"
		job, err := client.SetJobEnabled(ctx, app, pos[1], on)
		if err != nil {
			return fail(fmt.Errorf("%s job %q: %w", verb, pos[1], err))
		}
		done := map[string]string{"enable": "enabled", "disable": "disabled", "pause": "paused", "resume": "resumed"}[verb]
		return writeScheduledTaskResult(stdout, stderr, of, job, func() { _, _ = fmt.Fprintf(stdout, "job %q %s\n", jobLabelOf(job), done) })
	case "delete":
		if err := client.DeleteJob(ctx, app, pos[1]); err != nil {
			return fail(fmt.Errorf("delete job %q: %w", pos[1], err))
		}
		_, _ = fmt.Fprintf(stdout, "job %q deleted\n", pos[1])
		return exitOK
	case "history":
		job := ""
		if len(pos) == 2 {
			job = pos[1]
		}
		runs, err := client.ListJobRuns(ctx, app, job, f.status, f.limit)
		if err != nil {
			return fail(fmt.Errorf("job history for %q: %w", app, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, runs, func() { printRunsTable(stdout, runs) })
	default:
		return jobsLogs(ctx, client, app, pos[1], f.runID, of, stdout, stderr, jsonOut)
	}
}

func jobsLogs(ctx context.Context, client *Client, app, job, runID string, of outputFlags, stdout, stderr io.Writer, jsonOut bool) int {
	if runID == "" {
		runs, err := client.ListJobRuns(ctx, app, job, "", 1)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("find latest run of %q: %w", job, err))
		}
		if len(runs) == 0 {
			_, _ = fmt.Fprintf(stderr, "job %q has not run yet\n", job)
			return exitOK
		}
		runID = runs[0].ID
	}
	run, err := client.GetJobRun(ctx, app, job, runID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get run %q: %w", runID, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, run, func() {
		_, _ = fmt.Fprintf(stderr, "run %s: %s (trigger %s, %d attempt(s), %s)\n", run.ID, run.Status, run.Trigger, run.Attempts, time.Duration(run.DurationMS)*time.Millisecond)
		if run.Message != "" {
			_, _ = fmt.Fprintf(stderr, "%s\n", run.Message)
		}
		_, _ = fmt.Fprint(stdout, run.Output)
		if run.Output != "" && !strings.HasSuffix(run.Output, "\n") {
			_, _ = fmt.Fprintln(stdout)
		}
	})
}

func jobLabelOf(j scheduledTaskResource) string {
	if j.Name != "" {
		return j.Name
	}
	return j.ID
}

func printJobsTable(out io.Writer, jobs []scheduledTaskResource) {
	if len(jobs) == 0 {
		_, _ = fmt.Fprintln(out, "no jobs")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tID\tSCHEDULE\tMODE\tENABLED\tLAST RUN\tNEXT RUN\tCOMMAND")
	for _, j := range jobs {
		next := "-"
		if j.NextRunAt != nil {
			next = j.NextRunAt.Format(time.RFC3339)
		}
		tz := ""
		if j.Timezone != "" && j.Timezone != "UTC" {
			tz = " " + j.Timezone
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s%s\t%s\t%t\t%s\t%s\t%s\n", jobLabelOf(j), j.ID, j.Schedule, tz, j.Mode, j.Enabled, lastRunSummary(j), next, jobTarget(j))
	}
	_ = tw.Flush()
}

func printJob(out io.Writer, j scheduledTaskResource) {
	_, _ = fmt.Fprintf(out, "name:        %s\nid:          %s\napp:         %s\nmode:        %s\ncommand:     %s\nschedule:    %s (%s)\nenabled:     %t\nconcurrency: %s\n",
		j.Name, j.ID, j.ServiceName, j.Mode, jobTarget(j), j.Schedule, orStr(j.Timezone, "UTC"), j.Enabled, j.ConcurrencyPolicy)
	_, _ = fmt.Fprintf(out, "timeout:     %ds, retries %d (backoff %ds), catch-up %s\n", j.TimeoutSeconds, j.MaxRetries, j.RetryBackoffSeconds, orStr(j.CatchUp, "skip"))
	_, _ = fmt.Fprintf(out, "source:      %s\nlast run:    %s\nfailures:    %d in a row, %d missed\n", orStr(j.Source, "api"), lastRunSummary(j), j.ConsecutiveFailures, j.ConsecutiveMissed)
	if j.NextRunAt != nil {
		_, _ = fmt.Fprintf(out, "next run:    %s\n", j.NextRunAt.Format(time.RFC3339))
	}
}

func orStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func printRunsTable(out io.Writer, runs []jobRunResource) {
	if len(runs) == 0 {
		_, _ = fmt.Fprintln(out, "no runs")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "RUN\tJOB\tSTATUS\tTRIGGER\tSTARTED\tDURATION\tEXIT\tATTEMPTS\tNODE")
	for _, r := range runs {
		exit := "-"
		if r.ExitCode != nil {
			exit = fmt.Sprint(*r.ExitCode)
		}
		node := r.NodeID
		if node == "" {
			node = "local"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n", r.ID, orStr(r.JobName, r.TaskID), r.Status, r.Trigger,
			r.StartedAt.Format(time.RFC3339), (time.Duration(r.DurationMS) * time.Millisecond).Round(time.Millisecond), exit, r.Attempts, node)
	}
	_ = tw.Flush()
}

func jobTarget(j scheduledTaskResource) string {
	if j.Mode == "http" && j.HTTP != nil {
		bg := ""
		if j.HTTP.Background {
			bg = " (background)"
		}
		return orStr(j.HTTP.Method, "POST") + " " + j.HTTP.Path + bg
	}
	return strings.Join(j.Command, " ")
}
