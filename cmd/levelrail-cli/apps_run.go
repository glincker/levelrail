package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	envRunWaitGrace = "APP_RUN_WAIT_GRACE"
	// envRunPollRetries is how many failed polls in a row are tolerated before giving up on a job the server keeps running.
	envRunPollRetries = "APP_RUN_POLL_RETRIES"
	// exitRunTimeout follows the coreutils timeout convention so it never reads as a command's own code.
	exitRunTimeout = 124
	// exitInterrupted is the shell convention for a run stopped by Ctrl-C.
	exitInterrupted = 130
)

// serviceAppName maps a service key of a multi-service app to the app name the API knows it by.
func serviceAppName(app, service string) string {
	if service == "" {
		return app
	}
	return app + "-" + service
}

// runAppsRun implements "apps run <app> [--service S] -- <cmd...>": a one-shot job that waits,
// prints the command's output and exits with the command's own exit code.
func runAppsRun(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps run", "print the finished job as JSON to stdout and nothing else", stderr)
	var service string
	var timeout, poll time.Duration
	var retries int
	fs.StringVar(&service, "service", "", "service key of a multi-service app (runs against <app>-<service>)")
	fs.DurationVar(&timeout, "timeout", 0, "per-attempt timeout, e.g. 10m (default: the server's one-shot limit)")
	fs.IntVar(&retries, "retries", 0, "extra attempts after a failed one (default 0: the command runs once); the server enforces its own maximum")
	fs.DurationVar(&poll, "poll", 2*time.Second, "status poll interval while waiting")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsRunUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	fail := func(err error) int { return reportError(stdout, stderr, jsonOut, err) }

	pos := fs.Args()
	if len(pos) < 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps run requires an app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	if len(pos) > 1 && !slices.Contains(args, "--") {
		return fail(newValidationError("put the command after --, e.g. %s apps run %s -- %s", prog, pos[0], strings.Join(pos[1:], " ")))
	}
	if len(pos) < 2 {
		return fail(newValidationError("a command is required after --"))
	}
	if poll <= 0 || timeout < 0 || retries < 0 {
		return fail(newValidationError("--poll must be positive, --timeout and --retries must not be negative"))
	}
	target := serviceAppName(pos[0], service)
	if strings.ContainsAny(target, "/:") {
		return fail(newValidationError("%q looks like an image; apps run needs an app so the command gets its env, secrets and network", target))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	// An explicit 0 stops the server default (which retries) from re-running a failed command.
	req := apiclient.OneShotJobRequest{App: target, Command: pos[1:], TimeoutSeconds: int(timeout.Seconds()), BackoffLimit: &retries}
	job, err := client.RunOneShotJob(ctx, req)
	if err != nil {
		return fail(fmt.Errorf("run command on %q: %w", target, err))
	}

	jobID := job.ID
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout*time.Duration(retries+1) + envDurationOr(lookupEnv, envRunWaitGrace, 30*time.Second))
	}
	maxPollFails := 3
	if v, ok := lookupEnv(envRunPollRetries); ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			maxPollFails = n
		}
	}
	textOut := of.Format != outputJSON && of.Query == ""
	printed := ""
	// Ctrl-C cancels the run on the server so its container is killed and removed, not left behind.
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	pollFails := 0
	for job.State == "Running" || job.State == "" {
		if !deadline.IsZero() && time.Now().After(deadline) {
			_, _ = fmt.Fprintf(stderr, "%s: job %s did not finish within %s; it may still be running, check GET /api/v1/jobs/runs/%s\n", prog, jobID, timeout, jobID)
			return exitRunTimeout
		}
		select {
		case <-sigCtx.Done():
			cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			_, cerr := client.CancelOneShotJob(cctx, jobID)
			cancel()
			if cerr != nil {
				_, _ = fmt.Fprintf(stderr, "%s: cancel job %s: %v\n", prog, jobID, cerr)
			} else {
				_, _ = fmt.Fprintf(stderr, "%s: cancelled job %s; its container is removed\n", prog, jobID)
			}
			return exitInterrupted
		case <-time.After(poll):
		}
		next, err := client.GetOneShotJob(sigCtx, jobID)
		if err != nil {
			if sigCtx.Err() != nil {
				continue
			}
			if pollFails++; pollFails > maxPollFails {
				return fail(fmt.Errorf("get job %q (it may still be running on the server): %w", jobID, err))
			}
			continue
		}
		pollFails, job = 0, next
		if textOut {
			printed = writeOutputDelta(stdout, printed, job.Output)
		}
	}

	code := appsRunExitCode(job)
	if of.Format == outputJSON || of.Query != "" {
		if rc := writeScheduledTaskResult(stdout, stderr, of, job, func() {}); rc != exitOK {
			return rc
		}
		return code
	}
	writeOutputDelta(stdout, printed, job.Output)
	if job.Output != "" && !strings.HasSuffix(job.Output, "\n") {
		_, _ = fmt.Fprintln(stdout)
	}
	if job.OutputTruncated {
		_, _ = fmt.Fprintln(stderr, "output was longer than the server keeps; only the last part is shown")
	}
	if job.OutputWithheld {
		_, _ = fmt.Fprintln(stderr, "output withheld: this token lacks the read_sensitive ability, so only the status and exit code are shown")
	}
	if code != exitOK {
		reason := job.Message
		if reason == "" {
			reason = job.RunStatus
		}
		_, _ = fmt.Fprintf(stderr, "%s: command on %q failed (%s), exit %d\n", prog, target, reason, code)
	}
	return code
}

// writeOutputDelta prints the part of output not yet shown. A tail that no
// longer extends what was printed (the server cut the head) is not reprinted.
func writeOutputDelta(w io.Writer, printed, output string) string {
	if strings.HasPrefix(output, printed) {
		_, _ = io.WriteString(w, output[len(printed):])
		return output
	}
	return printed
}

// appsRunExitCode returns the command's own exit code when the platform has one, so scripts can branch on it.
func appsRunExitCode(j apiclient.OneShotJob) int {
	if j.State == "Succeeded" {
		return exitOK
	}
	if j.RunStatus == "timeout" {
		return exitRunTimeout
	}
	if j.ExitCode != nil && *j.ExitCode > 0 && *j.ExitCode < 256 {
		return *j.ExitCode
	}
	return exitDeployFailed
}

func appsRunUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps run <app> [--service S] [flags] -- <command> [args...]

Runs a command once (no retries unless --retries is set) in a fresh container built from the app, with its env,
secrets and network, waits for it, prints its output and exits with the
command's own exit code. No terminal is needed. Output is shown when the
command finishes.
Ctrl-C cancels the run: the container is killed and removed. Secret values are redacted from the output. Use
"apps exec" to run inside the live container instead.

Access: one-off runs and scheduled jobs follow console exec access rules,
so this needs the root ability on the app and exec enabled for it.

Exit codes: the command's code when it failed, %[5]d on timeout, 5 when the
job failed without a command exit code (for example the container could not start).

Flags:
  --service string       service key of a multi-service app (runs against <app>-<service>)
  --timeout duration     per-attempt timeout, e.g. 10m (default: the server's one-shot limit)
  --retries int          extra attempts after a failed one (default 0, so the command runs once)
  --poll duration        status poll interval while waiting (default 2s)
  --token string         API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                 print the finished job as JSON to stdout, nothing else
  --output string        output format: json, table, or text (--json is shorthand for --output json)
  --query string         JMESPath expression to filter the result before printing
  -h, --help             show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL, exitRunTimeout)
}
