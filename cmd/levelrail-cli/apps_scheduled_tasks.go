package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// runAppsScheduledTasks dispatches "apps scheduled-tasks <verb> [flags]"
// to one of create/list/get/update/delete/run, the CLI counterpart of
// internal/api/scheduled_tasks.go's own routes.
func runAppsScheduledTasks(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsScheduledTasksUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsScheduledTasksUsage(prog))
		return exitOK
	case "create":
		return runAppsScheduledTasksCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runAppsScheduledTasksList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runAppsScheduledTasksGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "update":
		return runAppsScheduledTasksUpdate(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runAppsScheduledTasksDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "run":
		return runAppsScheduledTasksRun(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps scheduled-tasks subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsScheduledTasksUsage(prog))
		return exitUsage
	}
}

func appsScheduledTasksUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scheduled-tasks create <app> --schedule CRON [--disabled] -- <command> [args...]
  %[1]s apps scheduled-tasks list <app> [flags]
  %[1]s apps scheduled-tasks get <app> <id> [flags]
  %[1]s apps scheduled-tasks update <app> <id> --schedule CRON [--disabled] -- <command> [args...]
  %[1]s apps scheduled-tasks delete <app> <id> [flags]
  %[1]s apps scheduled-tasks run <app> <id> [flags]

Manage an app's scheduled tasks: an arbitrary command run inside its
currently running container on a cron schedule. <command> is a real argv
run directly, no shell involved, the same contract "%[1]s apps exec"
documents; the "--" before it is required whenever the command itself
takes flags. AbilityWrite-gated server-side on every verb but list/get
(AbilityRead), AbilityDeploy on run.

Run "%[1]s apps scheduled-tasks <subcommand> -h" for a subcommand's own
flags.
`, prog)
}

func scheduledTaskFlags(fs *flag.FlagSet, schedule *string, disabled *bool) {
	fs.StringVar(schedule, "schedule", "", "standard 5-field cron expression, e.g. \"*/5 * * * *\" (required)")
	fs.BoolVar(disabled, "disabled", false, "create/save the task disabled (default: enabled)")
}

// parseScheduledTaskArgs splits fs's positional tokens into nArgs
// leading identifiers (app name, plus a task id where needed) and the
// trailing command argv. ok is false once help, a parse error, or a
// missing command has been reported.
func parseScheduledTaskArgs(fs *flag.FlagSet, args []string, stderr io.Writer, prog, cmdLabel string, nArgs int) (leading, command []string, exitCode int, ok bool) {
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return nil, nil, exitOK, false
		}
		return nil, nil, exitUsage, false
	}

	rest := fs.Args()
	if len(rest) < nArgs+1 {
		label := "an app name, a cron schedule, and a command"
		if nArgs == 2 {
			label = "an app name, a task id, a cron schedule, and a command"
		}
		_, _ = fmt.Fprintf(stderr, "%s: %s requires %s (use -- before the command)\n\n", prog, cmdLabel, label)
		fs.Usage()
		return nil, nil, exitUsage, false
	}
	return rest[:nArgs], rest[nArgs:], exitOK, true
}

func runAppsScheduledTasksCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps scheduled-tasks create", "print the created task as JSON to stdout and nothing else", stderr)
	var schedule string
	var disabled bool
	scheduledTaskFlags(fs, &schedule, &disabled)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScheduledTasksCreateUsage(prog)) }

	leading, command, code, ok := parseScheduledTaskArgs(fs, args, stderr, prog, "apps scheduled-tasks create", 1)
	if !ok {
		return code
	}
	appName := leading[0]
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	if schedule == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--schedule is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreateScheduledTask(context.Background(), appName, scheduledTaskRequest{
		Command: command, Schedule: schedule, Enabled: !disabled,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create scheduled task for app %q: %w", appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, created, func() {
		_, _ = fmt.Fprintf(stdout, "scheduled task %q created for app %q\n", created.ID, appName)
	})
}

func appsScheduledTasksCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scheduled-tasks create <app> --schedule CRON [--disabled] -- <command> [args...]

Creates a new scheduled task for <app>. <command> is executed directly
as argv inside the app's container, no shell involved.

Flags:
  --schedule string       standard 5-field cron expression (required)
  --disabled                 create the task disabled (default: enabled)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the created task as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsScheduledTasksList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps scheduled-tasks list", "print tasks as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScheduledTasksListUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	appName, ok := requireOneArg(fs, stderr, prog, "apps scheduled-tasks list", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	tasks, err := client.ListScheduledTasks(context.Background(), appName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list scheduled tasks for app %q: %w", appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, tasks, func() { printScheduledTasksTable(stdout, tasks) })
}

func printScheduledTasksTable(out io.Writer, tasks []scheduledTaskResource) {
	if len(tasks) == 0 {
		_, _ = fmt.Fprintln(out, "no scheduled tasks")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tCOMMAND\tSCHEDULE\tENABLED\tLAST RUN\tCONSECUTIVE FAILURES")
	for _, t := range tasks {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%s\t%d\n", t.ID, strings.Join(t.Command, " "), t.Schedule, t.Enabled, lastRunSummary(t), t.ConsecutiveFailures)
	}
	_ = tw.Flush()
}

func appsScheduledTasksListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scheduled-tasks list <app> [flags]

Lists an app's scheduled tasks.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print tasks as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// appAndTaskIDLabel is requireArgs' argsLabel for every "apps
// scheduled-tasks <verb> <app> <id>" subcommand (get/delete/run).
const appAndTaskIDLabel = "an app name and a task id"

// parseAppAndTaskID runs fs.Parse then extracts the <app> <id> pair
// get/delete/run all take with no other positional arguments, the
// common shape those three subcommands share.
func parseAppAndTaskID(fs *flag.FlagSet, args []string, stderr io.Writer, prog, cmdLabel string) (appName, id string, exitCode int, ok bool) {
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return "", "", exitOK, false
		}
		return "", "", exitUsage, false
	}
	rest, argsOK := requireArgs(fs, stderr, prog, cmdLabel, appAndTaskIDLabel, 2)
	if !argsOK {
		return "", "", exitUsage, false
	}
	return rest[0], rest[1], exitOK, true
}

func runAppsScheduledTasksGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps scheduled-tasks get", "print the task as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScheduledTasksGetUsage(prog)) }

	appName, id, code, ok := parseAppAndTaskID(fs, args, stderr, prog, "apps scheduled-tasks get")
	if !ok {
		return code
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	task, err := client.GetScheduledTask(context.Background(), appName, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get scheduled task %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, task, func() { printScheduledTaskHuman(stdout, task) })
}

func printScheduledTaskHuman(out io.Writer, t scheduledTaskResource) {
	_, _ = fmt.Fprintf(out, "id:        %s\n", t.ID)
	_, _ = fmt.Fprintf(out, "app:       %s\n", t.ServiceName)
	_, _ = fmt.Fprintf(out, "command:   %s\n", strings.Join(t.Command, " "))
	_, _ = fmt.Fprintf(out, "schedule:  %s\n", t.Schedule)
	_, _ = fmt.Fprintf(out, "enabled:   %t\n", t.Enabled)
	_, _ = fmt.Fprintf(out, "last run:  %s\n", lastRunSummary(t))
	if t.ConsecutiveFailures > 0 {
		_, _ = fmt.Fprintf(out, "consecutive failures: %d\n", t.ConsecutiveFailures)
	}
	if t.LastRunOutput != "" {
		_, _ = fmt.Fprintf(out, "output:\n%s\n", t.LastRunOutput)
	}
}

func lastRunSummary(t scheduledTaskResource) string {
	if t.LastRunAt == nil {
		return "never"
	}
	return fmt.Sprintf("%s (%s)", t.LastRunAt.Format("2006-01-02T15:04:05Z07:00"), t.LastRunStatus)
}

func appsScheduledTasksGetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scheduled-tasks get <app> <id> [flags]

Shows one scheduled task, including its most recent run outcome and
output.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the task as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsScheduledTasksUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps scheduled-tasks update", "print the updated task as JSON to stdout and nothing else", stderr)
	var schedule string
	var disabled bool
	scheduledTaskFlags(fs, &schedule, &disabled)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScheduledTasksUpdateUsage(prog)) }

	leading, command, code, ok := parseScheduledTaskArgs(fs, args, stderr, prog, "apps scheduled-tasks update", 2)
	if !ok {
		return code
	}
	appName, id := leading[0], leading[1]
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	if schedule == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--schedule is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.UpdateScheduledTask(context.Background(), appName, id, scheduledTaskRequest{
		Command: command, Schedule: schedule, Enabled: !disabled,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("update scheduled task %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, updated, func() {
		_, _ = fmt.Fprintf(stdout, "scheduled task %q updated\n", updated.ID)
	})
}

func appsScheduledTasksUpdateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scheduled-tasks update <app> <id> --schedule CRON [--disabled] -- <command> [args...]

Replaces every editable field of an existing scheduled task (a full
replace, not a partial patch): --schedule and the command must be
supplied on every call, the same as the value already saved.

Flags:
  --schedule string       standard 5-field cron expression (required)
  --disabled                 save the task disabled (default: enabled)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the updated task as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsScheduledTasksDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps scheduled-tasks delete", "print {} to stdout on success instead of a plain confirmation", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScheduledTasksDeleteUsage(prog)) }

	appName, id, code, ok := parseAppAndTaskID(fs, args, stderr, prog, "apps scheduled-tasks delete")
	if !ok {
		return code
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteScheduledTask(context.Background(), appName, id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete scheduled task %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, map[string]any{}, func() {
		_, _ = fmt.Fprintf(stdout, "scheduled task %q deleted\n", id)
	})
}

func appsScheduledTasksDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scheduled-tasks delete <app> <id> [flags]

Deletes a scheduled task.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {} to stdout on success instead of a plain confirmation
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsScheduledTasksRun(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps scheduled-tasks run", "print the task as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScheduledTasksRunUsage(prog)) }

	appName, id, code, ok := parseAppAndTaskID(fs, args, stderr, prog, "apps scheduled-tasks run")
	if !ok {
		return code
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	started, err := client.RunScheduledTask(context.Background(), appName, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("run scheduled task %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, started, func() {
		_, _ = fmt.Fprintf(stdout, "run started for scheduled task %q (use \"%s apps scheduled-tasks get %s %s\" to see the result)\n", id, prog, appName, id)
	})
}

func appsScheduledTasksRunUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scheduled-tasks run <app> <id> [flags]

Triggers an immediate run of a scheduled task, the same command the cron
schedule itself would run. Returns as soon as the attempt is recorded and
under way, not once the command finishes; "%[1]s apps scheduled-tasks
get" is how you see the outcome.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the task as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// writeScheduledTaskResult prints value as JSON when jsonOut is set,
// otherwise runs plainOut, the success-output shape every apps
// scheduled-tasks subcommand shares.
func writeScheduledTaskResult(stdout, stderr io.Writer, jsonOut bool, value any, plainOut func()) int {
	if jsonOut {
		if err := writeJSONValue(stdout, value); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	plainOut()
	return exitOK
}
