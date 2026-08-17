package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runAppsScheduledTasks dispatches "apps scheduled-tasks <verb> [flags]"
// to one of create/list/update/delete/run/runs, the CLI counterpart of
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
	case "update":
		return runAppsScheduledTasksUpdate(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runAppsScheduledTasksDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "run":
		return runAppsScheduledTasksRun(prog, args[1:], stdout, stderr, lookupEnv)
	case "runs":
		return runAppsScheduledTasksRuns(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps scheduled-tasks subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsScheduledTasksUsage(prog))
		return exitUsage
	}
}

func appsScheduledTasksUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scheduled-tasks create <app> --name NAME --command CMD --schedule CRON [flags]
  %[1]s apps scheduled-tasks list <app> [flags]
  %[1]s apps scheduled-tasks update <app> <id> --name NAME --command CMD --schedule CRON [flags]
  %[1]s apps scheduled-tasks delete <app> <id> [flags]
  %[1]s apps scheduled-tasks run <app> <id> [flags]
  %[1]s apps scheduled-tasks runs <app> <id> [flags]

Manage an app's scheduled tasks: an arbitrary command run inside its
container on a cron schedule (e.g. "php artisan schedule:run"), the same
non-interactive exec primitive "%[1]s apps exec" uses. AbilityRoot-gated
server-side on every verb but list, the same tier "%[1]s apps exec" itself
requires.

Run "%[1]s apps scheduled-tasks <subcommand> -h" for a subcommand's own
flags.
`, prog)
}

func scheduledTaskFlags(fs *flag.FlagSet, name, command, schedule *string, disabled *bool, timeout *int) {
	fs.StringVar(name, "name", "", "display name for the task (required)")
	fs.StringVar(command, "command", "", "command to run, executed as `sh -c COMMAND` inside the container (required)")
	fs.StringVar(schedule, "schedule", "", "standard 5-field cron expression, e.g. \"*/5 * * * *\" (required)")
	fs.BoolVar(disabled, "disabled", false, "create/save the task disabled (default: enabled)")
	fs.IntVar(timeout, "timeout", 0, "command timeout in seconds (0 means the server's own default)")
}

func runAppsScheduledTasksCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" apps scheduled-tasks create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag, name, command, schedule string
	var disabled bool
	var timeout int
	var jsonOut bool
	scheduledTaskFlags(fs, &name, &command, &schedule, &disabled, &timeout)
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print the created task as JSON to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScheduledTasksCreateUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps scheduled-tasks create requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	appName := rest[0]

	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	if command == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--command is required"))
	}
	if schedule == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--schedule is required"))
	}

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))
	created, err := client.CreateScheduledTask(context.Background(), appName, scheduledTaskRequest{
		Name: name, Command: command, Schedule: schedule, Enabled: !disabled, TimeoutSeconds: timeout,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create scheduled task for app %q: %w", appName, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, created); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "scheduled task %q (id %s) created for app %q\n", created.Name, created.ID, appName)
	return exitOK
}

func appsScheduledTasksCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scheduled-tasks create <app> --name NAME --command CMD --schedule CRON [flags]

Creates a new scheduled task for <app>.

Flags:
  --name string           display name for the task (required)
  --command string        command to run, executed as ` + "`sh -c COMMAND`" + ` (required)
  --schedule string       standard 5-field cron expression (required)
  --disabled                 create the task disabled (default: enabled)
  --timeout int            command timeout in seconds (0 means the server's own default)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --json                     print the created task as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsScheduledTasksList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" apps scheduled-tasks list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag string
	var jsonOut bool
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print tasks as a JSON array to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScheduledTasksListUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps scheduled-tasks list requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	appName := rest[0]

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))
	tasks, err := client.ListScheduledTasks(context.Background(), appName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list scheduled tasks for app %q: %w", appName, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, tasks); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printScheduledTasksTable(stdout, tasks)
	return exitOK
}

func printScheduledTasksTable(out io.Writer, tasks []scheduledTaskResource) {
	if len(tasks) == 0 {
		_, _ = fmt.Fprintln(out, "no scheduled tasks")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tCOMMAND\tSCHEDULE\tENABLED")
	for _, t := range tasks {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\n", t.ID, t.Name, t.Command, t.Schedule, t.Enabled)
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
  --json                    print tasks as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsScheduledTasksUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" apps scheduled-tasks update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag, name, command, schedule string
	var disabled bool
	var timeout int
	var jsonOut bool
	scheduledTaskFlags(fs, &name, &command, &schedule, &disabled, &timeout)
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print the updated task as JSON to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScheduledTasksUpdateUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps scheduled-tasks update requires an app name and a task id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	appName, id := rest[0], rest[1]

	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	if command == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--command is required"))
	}
	if schedule == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--schedule is required"))
	}

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))
	updated, err := client.UpdateScheduledTask(context.Background(), appName, id, scheduledTaskRequest{
		Name: name, Command: command, Schedule: schedule, Enabled: !disabled, TimeoutSeconds: timeout,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("update scheduled task %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, updated); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "scheduled task %q updated\n", updated.ID)
	return exitOK
}

func appsScheduledTasksUpdateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scheduled-tasks update <app> <id> --name NAME --command CMD --schedule CRON [flags]

Replaces every editable field of an existing scheduled task (a full
replace, not a partial patch): all three of --name/--command/--schedule
must be supplied on every call, the same as the value already saved.

Flags:
  --name string           display name for the task (required)
  --command string        command to run, executed as ` + "`sh -c COMMAND`" + ` (required)
  --schedule string       standard 5-field cron expression (required)
  --disabled                 save the task disabled (default: enabled)
  --timeout int            command timeout in seconds (0 means the server's own default)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --json                     print the updated task as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsScheduledTasksDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" apps scheduled-tasks delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag string
	var jsonOut bool
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print {} to stdout on success instead of a plain confirmation")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScheduledTasksDeleteUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps scheduled-tasks delete requires an app name and a task id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	appName, id := rest[0], rest[1]

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))
	if err := client.DeleteScheduledTask(context.Background(), appName, id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete scheduled task %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]any{}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "scheduled task %q deleted\n", id)
	return exitOK
}

func appsScheduledTasksDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scheduled-tasks delete <app> <id> [flags]

Deletes a scheduled task and its execution history.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print {} to stdout on success instead of a plain confirmation
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsScheduledTasksRun(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" apps scheduled-tasks run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag string
	var jsonOut bool
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print the started run as JSON to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScheduledTasksRunUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps scheduled-tasks run requires an app name and a task id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	appName, id := rest[0], rest[1]

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))
	started, err := client.RunScheduledTask(context.Background(), appName, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("run scheduled task %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, started); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "run %q started for scheduled task %q (use \"%s apps scheduled-tasks runs %s %s\" to see the result)\n", started.ID, id, prog, appName, id)
	return exitOK
}

func appsScheduledTasksRunUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scheduled-tasks run <app> <id> [flags]

Triggers an immediate run of a scheduled task, the same command the cron
schedule itself would run. Returns as soon as the attempt is recorded and
under way, not once the command finishes; "%[1]s apps scheduled-tasks
runs" is how you see the outcome.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print the started run as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsScheduledTasksRuns(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" apps scheduled-tasks runs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag string
	var jsonOut bool
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print run history as a JSON array to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScheduledTasksRunsUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps scheduled-tasks runs requires an app name and a task id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	appName, id := rest[0], rest[1]

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))
	runs, err := client.ListScheduledTaskRuns(context.Background(), appName, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list runs for scheduled task %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, runs); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printScheduledTaskRunsTable(stdout, runs)
	return exitOK
}

func printScheduledTaskRunsTable(out io.Writer, runs []scheduledTaskRunResource) {
	if len(runs) == 0 {
		_, _ = fmt.Fprintln(out, "no runs")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tSTATUS\tEXIT CODE\tSTARTED\tFINISHED")
	for _, r := range runs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", r.ID, r.Status, r.ExitCode, r.StartedAt, r.FinishedAt)
	}
	_ = tw.Flush()
}

func appsScheduledTasksRunsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scheduled-tasks runs <app> <id> [flags]

Lists a scheduled task's execution history (both cron-triggered and
manually-triggered runs share the same history).

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print run history as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
