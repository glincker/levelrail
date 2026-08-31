package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runAppsAlerts dispatches "apps alerts <verb> [flags]" to one of
// list/create/delete, the CLI counterpart of internal/api/alerts.go's
// own /api/v1/apps/{name}/alerts routes.
func runAppsAlerts(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsAlertsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsAlertsUsage(prog))
		return exitOK
	case "list":
		return runAppsAlertsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runAppsAlertsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runAppsAlertsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps alerts subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsAlertsUsage(prog))
		return exitUsage
	}
}

func appsAlertsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps alerts list <app> [flags]
  %[1]s apps alerts create <app> --kind threshold --metric METRIC --comparator OP --threshold N [flags]
  %[1]s apps alerts create <app> --kind crashloop --restart-count-threshold N --restart-window DURATION [flags]
  %[1]s apps alerts create <app> --kind cert_expiry [flags]
  %[1]s apps alerts create <app> --kind patch_status [flags]
  %[1]s apps alerts create <app> --kind scheduled_task_failure --scheduled-task-id ID --restart-count-threshold N [flags]
  %[1]s apps alerts create <app> --kind node_disk_space [flags]
  %[1]s apps alerts delete <app> <id> [flags]

Manages an app's alert rules. A threshold rule watches a metric; a
crashloop rule watches container restarts; a cert_expiry rule watches
every certificate on the whole control plane (not just this app's own
domains), a patch_status rule watches every node's pending OS security
patches the same way, and a node_disk_space rule watches every node's
disk usage percentage the same way too, none of the three needing any
metric/threshold/restart flags; a scheduled_task_failure rule watches
one of this app's scheduled tasks' consecutive-failure count (see "apps
scheduled-tasks list"), reusing --restart-count-threshold as that
count's threshold. All six notify the same way once they fire.

Run "%[1]s apps alerts <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsAlertsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps alerts list", "print rules as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsAlertsListUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	appName, ok := requireOneArg(fs, stderr, prog, "apps alerts list", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)
	rules, err := client.ListAlertRules(context.Background(), appName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list alert rules for app %q: %w", appName, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, rules); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printAlertRulesTable(stdout, rules)
	return exitOK
}

func printAlertRulesTable(out io.Writer, rules []alertRuleResource) {
	if len(rules) == 0 {
		_, _ = fmt.Fprintln(out, "no alert rules")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tKIND\tCONDITION\tENABLED\tFIRING")
	for _, r := range rules {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%t\n", r.ID, r.Name, r.Kind, alertRuleCondition(r), r.Enabled, r.Firing)
	}
	_ = tw.Flush()
}

// alertRuleCondition summarizes what r actually watches, one line per
// kind, matching the web dashboard's own AlertRulesPanel.tsx
// formatCondition.
func alertRuleCondition(r alertRuleResource) string {
	switch r.Kind {
	case "threshold":
		cond := fmt.Sprintf("%s %s %g", r.Metric, r.Comparator, r.Threshold)
		if r.ForDuration != "" {
			cond += " for " + r.ForDuration
		}
		return cond
	case "crashloop":
		return fmt.Sprintf("%d restarts in %s", r.RestartCountThreshold, r.RestartWindow)
	case "cert_expiry":
		return "any certificate expiring soon or expired (platform-wide)"
	case "patch_status":
		return "any node over its pending security-patch threshold (platform-wide)"
	case "node_disk_space":
		return "any node over its disk-usage percentage threshold (platform-wide)"
	case "scheduled_task_failure":
		return fmt.Sprintf("task %s fails %d runs in a row", r.ScheduledTaskID, r.RestartCountThreshold)
	default:
		return "-"
	}
}

func appsAlertsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps alerts list <app> [flags]

Lists an app's alert rules, including disabled ones.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print rules as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsAlertsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps alerts create", "print the created rule as JSON to stdout and nothing else", stderr)
	var (
		name, kind, metric, comparator, forDuration string
		threshold                                   float64
		restartCountThreshold                       int
		restartWindow                               string
		scheduledTaskID                             string
		channelID, notifyURL, notifyKind            string
		disabled                                    bool
	)
	fs.StringVar(&name, "name", "", "display name for the rule (required)")
	fs.StringVar(&kind, "kind", "", "rule kind: threshold, crashloop, cert_expiry, patch_status, scheduled_task_failure, node_disk_space (required)")
	fs.StringVar(&metric, "metric", "", "metric name (--kind threshold only, required for that kind)")
	fs.StringVar(&comparator, "comparator", "", "one of >, <, >=, <= (--kind threshold only, required for that kind)")
	fs.Float64Var(&threshold, "threshold", 0, "threshold value (--kind threshold only)")
	fs.StringVar(&forDuration, "for-duration", "", "how long the condition must hold before firing, e.g. \"2m\" (--kind threshold only, optional)")
	fs.IntVar(&restartCountThreshold, "restart-count-threshold", 0, "restart count (--kind crashloop) or consecutive-failure count (--kind scheduled_task_failure) that triggers firing, required for both kinds")
	fs.StringVar(&restartWindow, "restart-window", "", "time window restarts are counted in, e.g. \"5m\" (--kind crashloop only, required for that kind)")
	fs.StringVar(&scheduledTaskID, "scheduled-task-id", "", "which of this app's scheduled tasks to watch (--kind scheduled_task_failure only, required for that kind; see \"apps scheduled-tasks list\")")
	fs.StringVar(&channelID, "channel-id", "", "attach an already-connected notification channel (see \"channels list\")")
	fs.StringVar(&notifyURL, "notify-url", "", "legacy alternative to --channel-id: a raw webhook URL/destination")
	fs.StringVar(&notifyKind, "notify-kind", "", "legacy alternative to --channel-id: generic, slack, discord, telegram, email, pushover, pagerduty, teams")
	fs.BoolVar(&disabled, "disabled", false, "create the rule disabled (default: enabled)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsAlertsCreateUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	appName, ok := requireOneArg(fs, stderr, prog, "apps alerts create", "app name")
	if !ok {
		return exitUsage
	}

	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	switch kind {
	case "threshold":
		if metric == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--metric is required for --kind threshold"))
		}
		if comparator == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--comparator is required for --kind threshold"))
		}
	case "crashloop":
		if restartCountThreshold <= 0 {
			return reportError(stdout, stderr, jsonOut, newValidationError("--restart-count-threshold must be a positive integer for --kind crashloop"))
		}
		if restartWindow == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--restart-window is required for --kind crashloop"))
		}
	case "cert_expiry", "patch_status", "node_disk_space":
		// No kind-specific flags: a cert_expiry rule watches every
		// certificate on the control plane, a patch_status rule watches
		// every node's pending OS security patches, and a node_disk_space
		// rule watches every node's disk usage, none tied to this app's
		// own metrics.
	case "scheduled_task_failure":
		if scheduledTaskID == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--scheduled-task-id is required for --kind scheduled_task_failure"))
		}
		if restartCountThreshold <= 0 {
			return reportError(stdout, stderr, jsonOut, newValidationError("--restart-count-threshold must be a positive integer for --kind scheduled_task_failure"))
		}
	case "":
		return reportError(stdout, stderr, jsonOut, newValidationError("--kind is required (threshold, crashloop, cert_expiry, patch_status, scheduled_task_failure, or node_disk_space)"))
	default:
		return reportError(stdout, stderr, jsonOut, newValidationError("--kind %q is not valid: must be threshold, crashloop, cert_expiry, patch_status, scheduled_task_failure, or node_disk_space", kind))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)
	created, err := client.CreateAlertRule(context.Background(), appName, createAlertRuleRequest{
		Name: name, Kind: kind, Metric: metric, Comparator: comparator, Threshold: threshold, ForDuration: forDuration,
		RestartCountThreshold: restartCountThreshold, RestartWindow: restartWindow, ScheduledTaskID: scheduledTaskID,
		ChannelID: channelID, NotifyURL: notifyURL, NotifyKind: notifyKind,
		Enabled: !disabled,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create alert rule for app %q: %w", appName, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, created); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "alert rule %q (id %s, kind %s) created for app %q\n", created.Name, created.ID, created.Kind, appName)
	return exitOK
}

func appsAlertsCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps alerts create <app> --name NAME --kind threshold --metric METRIC --comparator OP --threshold N [flags]
  %[1]s apps alerts create <app> --name NAME --kind crashloop --restart-count-threshold N --restart-window DURATION [flags]
  %[1]s apps alerts create <app> --name NAME --kind cert_expiry [flags]
  %[1]s apps alerts create <app> --name NAME --kind patch_status [flags]
  %[1]s apps alerts create <app> --name NAME --kind scheduled_task_failure --scheduled-task-id ID --restart-count-threshold N [flags]
  %[1]s apps alerts create <app> --name NAME --kind node_disk_space [flags]

Creates a new alert rule for <app>. cert_expiry, patch_status, and
node_disk_space rules are platform-wide (cert_expiry watches every
certificate on the control plane, patch_status watches every node's
pending OS security patches, node_disk_space watches every node's disk
usage percentage, none limited to <app>'s own domains or workloads);
<app> only decides where the rule shows up in "apps alerts list", not
what it evaluates. A scheduled_task_failure rule watches one of <app>'s
own scheduled tasks (--scheduled-task-id must belong to <app>; see "apps
scheduled-tasks list") and fires once it has failed
--restart-count-threshold runs in a row.

Flags:
  --name string                        display name for the rule (required)
  --kind string                        threshold, crashloop, cert_expiry, patch_status, scheduled_task_failure, or node_disk_space (required)
  --metric string                      metric name (--kind threshold only)
  --comparator string                  >, <, >=, or <= (--kind threshold only)
  --threshold float                    threshold value (--kind threshold only)
  --for-duration string                how long the condition must hold before firing, e.g. "2m" (--kind threshold only)
  --restart-count-threshold int        restart count (--kind crashloop) or consecutive-failure count (--kind scheduled_task_failure) that triggers firing
  --restart-window string              time window restarts are counted in, e.g. "5m" (--kind crashloop only)
  --scheduled-task-id string           which of <app>'s scheduled tasks to watch (--kind scheduled_task_failure only)
  --channel-id string                  attach an already-connected notification channel
  --notify-url string                  legacy alternative to --channel-id
  --notify-kind string                 legacy alternative to --channel-id
  --disabled                           create the rule disabled (default: enabled)
  --token string                       API token (default: %[2]s env var, then the credentials file)
  --api-url string                    control plane base URL (default: %[3]s env var, then %[4]s)
  --json                                 print the created rule as JSON to stdout, nothing else
  -h, --help                           show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsAlertsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps alerts delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsAlertsDeleteUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	rest, argsOK := requireArgs(fs, stderr, prog, "apps alerts delete", "an app name and a rule id", 2)
	if !argsOK {
		return exitUsage
	}
	appName, id := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)
	if err := client.DeleteAlertRule(context.Background(), appName, id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete alert rule %q for app %q: %w", id, appName, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]bool{"deleted": true}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "alert rule %q deleted\n", id)
	return exitOK
}

func appsAlertsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps alerts delete <app> <id> [flags]

Deletes one of <app>'s alert rules.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print {"deleted": true} as JSON to stdout on success, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
