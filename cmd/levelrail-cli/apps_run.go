package main

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	envRunWaitGrace = "APP_RUN_WAIT_GRACE"
	// exitRunTimeout follows the coreutils timeout convention so it never reads as a command's own code.
	exitRunTimeout = 124
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
	fs.StringVar(&service, "service", "", "service key of a multi-service app (runs against <app>-<service>)")
	fs.DurationVar(&timeout, "timeout", 0, "per-attempt timeout, e.g. 10m (default: the server's one-shot limit)")
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
	if poll <= 0 || timeout < 0 {
		return fail(newValidationError("--poll must be positive and --timeout must not be negative"))
	}
	target := serviceAppName(pos[0], service)
	if strings.ContainsAny(target, "/:") {
		return fail(newValidationError("%q looks like an image; apps run needs an app so the command gets its env, secrets and network", target))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	req := apiclient.OneShotJobRequest{App: target, Command: pos[1:], TimeoutSeconds: int(timeout.Seconds())}
	job, err := client.RunOneShotJob(ctx, req)
	if err != nil {
		return fail(fmt.Errorf("run command on %q: %w", target, err))
	}

	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout + envDurationOr(lookupEnv, envRunWaitGrace, 30*time.Second))
	}
	for job.State == "Running" || job.State == "" {
		if !deadline.IsZero() && time.Now().After(deadline) {
			_, _ = fmt.Fprintf(stderr, "%s: job %s did not finish within %s; it may still be running, check GET /api/v1/jobs/runs/%s\n", prog, job.ID, timeout, job.ID)
			return exitRunTimeout
		}
		time.Sleep(poll)
		if job, err = client.GetOneShotJob(ctx, job.ID); err != nil {
			return fail(fmt.Errorf("get job %q: %w", job.ID, err))
		}
	}

	code := appsRunExitCode(job)
	if of.Format == outputJSON || of.Query != "" {
		if rc := writeScheduledTaskResult(stdout, stderr, of, job, func() {}); rc != exitOK {
			return rc
		}
		return code
	}
	_, _ = io.WriteString(stdout, job.Output)
	if job.Output != "" && !strings.HasSuffix(job.Output, "\n") {
		_, _ = fmt.Fprintln(stdout)
	}
	if job.OutputTruncated {
		_, _ = fmt.Fprintln(stderr, "output was longer than the server keeps; only the last part is shown")
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

Runs a command once in a fresh container built from the app, with its env,
secrets and network, waits for it, prints its output and exits with the
command's own exit code. No terminal is needed. Output is shown when the
command finishes. Secret values are redacted from the output. Use
"apps exec" to run inside the live container instead.

Exit codes: the command's code when it failed, %[5]d on timeout, 5 when the
job failed without a command exit code (for example the container could not start).

Flags:
  --service string       service key of a multi-service app (runs against <app>-<service>)
  --timeout duration     per-attempt timeout, e.g. 10m (default: the server's one-shot limit)
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
