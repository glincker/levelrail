package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const jobsRunOneShotUsage = `Usage:
  %[1]s jobs run <app> --command "CMD ARGS" [flags]
  %[1]s jobs run <app> [flags] -- <command> [args...]
  %[1]s jobs run <app> <job>        run an existing scheduled job now

Run a command once, to completion, in a fresh container built from the app.

Flags:
  --command STR       command to run (quotes group words; no shell involved)
  --completions N     runs that must succeed (default 1)
  --parallelism P     most runs at the same time (default 1)
  --backoff-limit B   failed attempts tolerated before the job fails
  --timeout DURATION  per-attempt timeout, e.g. 10m
  --wait              block until the job settles; exit 5 when it failed
  --poll DURATION     status poll interval with --wait (default 2s)
`

// wantsOneShot reports whether a "jobs run" call asks for a one-shot job.
func wantsOneShot(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return true
		}
		if a == "--command" || strings.HasPrefix(a, "--command=") || a == "-command" {
			return true
		}
	}
	return false
}

// splitCommand splits s into argv, honoring single and double quotes.
func splitCommand(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	var quote rune
	started := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, started = r, true
		case r == ' ' || r == '\t':
			if started || cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, newValidationError("--command has an unterminated quote")
	}
	if started || cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out, nil
}

func runJobsOneShot(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "jobs run", "print the result as JSON to stdout and nothing else", stderr)
	var command string
	var completions, parallelism int
	backoffLimit := -1
	var timeout, poll time.Duration
	var wait bool
	fs.StringVar(&command, "command", "", "command to run")
	fs.IntVar(&completions, "completions", 1, "runs that must succeed")
	fs.IntVar(&parallelism, "parallelism", 1, "most runs at the same time")
	fs.IntVar(&backoffLimit, "backoff-limit", -1, "failed attempts tolerated before the job fails")
	fs.DurationVar(&timeout, "timeout", 0, "per-attempt timeout")
	fs.DurationVar(&poll, "poll", 2*time.Second, "status poll interval with --wait")
	fs.BoolVar(&wait, "wait", false, "block until the job settles")
	fs.Usage = func() { _, _ = fmt.Fprintf(stderr, jobsRunOneShotUsage, prog) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	pos := fs.Args()
	fail := func(err error) int { return reportError(stdout, stderr, jsonOut, err) }
	if len(pos) < 1 {
		_, _ = fmt.Fprintf(stderr, "%s: jobs run: an app is required\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	app := pos[0]
	if strings.ContainsAny(app, "/:") {
		return fail(newValidationError("%q looks like an image; one-shot jobs run against an app so they get its env, secrets and network", app))
	}
	argv := pos[1:]
	if command != "" {
		parsed, err := splitCommand(command)
		if err != nil {
			return fail(err)
		}
		argv = append(parsed, argv...)
	}
	if len(argv) == 0 {
		return fail(newValidationError("a command is required (--command \"...\" or after --)"))
	}

	req := apiclient.OneShotJobRequest{App: app, Command: argv, Completions: completions, Parallelism: parallelism, TimeoutSeconds: int(timeout.Seconds())}
	if backoffLimit >= 0 {
		req.BackoffLimit = &backoffLimit
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	job, err := client.RunOneShotJob(ctx, req)
	if err != nil {
		return fail(fmt.Errorf("run one-shot job on %q: %w", app, err))
	}
	if wait {
		for job.State == "Running" {
			time.Sleep(poll)
			if job, err = client.GetOneShotJob(ctx, job.ID); err != nil {
				return fail(fmt.Errorf("get one-shot job %q: %w", job.ID, err))
			}
		}
	}
	code := writeScheduledTaskResult(stdout, stderr, of, job, func() { printOneShot(stdout, job) })
	if code == exitOK && wait && job.State == "Failed" {
		return exitDeployFailed
	}
	return code
}

func printOneShot(out io.Writer, j apiclient.OneShotJob) {
	_, _ = fmt.Fprintf(out, "job %s on %q: %s, %d/%d completions succeeded, %d failed attempt(s), %d running\n",
		j.ID, j.App, j.State, j.Succeeded, j.Completions, j.FailedAttempts, j.Active)
	if j.Message != "" {
		_, _ = fmt.Fprintln(out, j.Message)
	}
	if j.State == "Running" {
		_, _ = fmt.Fprintf(out, "re-run with --wait to block until it settles, or poll GET /api/v1/jobs/runs/%s\n", j.ID)
	}
}
