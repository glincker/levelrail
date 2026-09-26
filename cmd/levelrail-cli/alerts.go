package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAlerts dispatches the top-level "alerts" tree: silences,
// maintenance windows and history. Per-app rules stay under "apps alerts".
func runAlerts(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 {
		switch args[0] {
		case "silences":
			return dispatchSub(prog, "alerts silences", alertsSilencesUsage, silenceCommands(), args[1:], stdout, stderr, lookupEnv)
		case "silence":
			return runAPICmd(prog, quickSilenceCommand(), args[1:], stdout, stderr, lookupEnv)
		case "maintenance":
			return dispatchSub(prog, "alerts maintenance", alertsMaintenanceUsage, maintenanceCommands(), args[1:], stdout, stderr, lookupEnv)
		case "history":
			return runAPICmd(prog, historyCommand(), args[1:], stdout, stderr, lookupEnv)
		}
	}
	return dispatchSub(prog, "alerts", alertsUsage, map[string]apiCmd{}, args, stdout, stderr, lookupEnv)
}

func alertsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s alerts silences list|create|delete [flags]     mute matching alerts for a while
  %[1]s alerts silence <app> <rule-id> [--for 1h]      silence one rule (quick action)
  %[1]s alerts maintenance list|create|update|delete   recurring silences (cron + duration + timezone)
  %[1]s alerts history [flags]                         firings and what happened to each notification

Alert rules themselves live under "%[1]s apps alerts".
Run "%[1]s alerts <subcommand> -h" for its flags.
`, prog)
}

func alertsSilencesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s alerts silences list [--all]
  %[1]s alerts silences create --for DURATION [--rule ID] [--app NAME] [--node NAME] [--kind KIND] [--severity S] [--label k=v] [--reason TEXT]
  %[1]s alerts silences delete <id>

A silence keeps matching alerts evaluating and recorded in history but
stops them notifying. Every filter given must match (AND); repeat a
flag to accept any of several values. "delete" ends a silence now and
keeps it in history ("list --all").
`, prog)
}

func alertsMaintenanceUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s alerts maintenance list
  %[1]s alerts maintenance create --name NAME --cron "0 3 * * 0" --duration 2h [--tz Europe/Berlin] [--scope all|app|node] [--target NAME]...
  %[1]s alerts maintenance update <id> (same flags as create, plus --disabled)
  %[1]s alerts maintenance delete <id>
`, prog)
}

func silenceCommands() map[string]apiCmd {
	return map[string]apiCmd{"list": silenceListCommand(), "create": silenceCreateCommand(), "delete": silenceDeleteCommand()}
}

func silenceListCommand() apiCmd {
	var all bool
	return apiCmd{
		label: "alerts silences list",
		usage: alertsSilencesUsage,
		setup: func(fs *flag.FlagSet) { fs.BoolVar(&all, "all", false, "include expired silences") },
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListAlertSilences(ctx, all)
			return list, func(w io.Writer) { printSilences(w, list) }, err
		},
	}
}

func silenceCreateCommand() apiCmd {
	var (
		rules, apps, nodes, kinds, severities, labels stringList
		dur, reason                                   string
	)
	return apiCmd{
		label: "alerts silences create",
		usage: alertsSilencesUsage,
		setup: func(fs *flag.FlagSet) {
			fs.Var(&rules, "rule", "match this rule ID (repeatable)")
			fs.Var(&apps, "app", "match alerts of this app (repeatable)")
			fs.Var(&nodes, "node", "match alerts of apps on this node, by name or ID (repeatable)")
			fs.Var(&kinds, "kind", "match this rule kind (repeatable)")
			fs.Var(&severities, "severity", "match info, warning or critical (repeatable)")
			fs.Var(&labels, "label", "match a rule label, key=value (repeatable)")
			fs.StringVar(&dur, "for", "", "how long the silence lasts, e.g. 1h, 4h, 24h (required)")
			fs.StringVar(&reason, "reason", "", "why (shown in the dashboard)")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			if dur == "" {
				return nil, nil, newValidationError("--for is required, e.g. --for 1h")
			}
			lm, err := labelMap(labels)
			if err != nil {
				return nil, nil, err
			}
			m := apiclient.SilenceMatchers{RuleIDs: rules, Apps: apps, Nodes: nodes, Kinds: kinds, Severities: severities, Labels: lm}
			s, err := c.CreateAlertSilence(ctx, apiclient.CreateSilenceRequest{Matchers: m, Duration: dur, Reason: reason})
			return s, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "silence %s active until %s\n", s.ID, s.EndsAt.Format(time.RFC3339))
			}, err
		},
	}
}

func silenceDeleteCommand() apiCmd {
	return apiCmd{
		label: "alerts silences delete",
		usage: alertsSilencesUsage,
		args:  1,
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			s, err := c.ExpireAlertSilence(ctx, pos[0])
			return s, func(w io.Writer) { _, _ = fmt.Fprintf(w, "silence %s expired\n", s.ID) }, err
		},
	}
}

func quickSilenceCommand() apiCmd {
	var dur, reason string
	return apiCmd{
		label: "alerts silence",
		usage: alertsUsage,
		args:  2,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&dur, "for", "1h", "how long to silence the rule, e.g. 1h, 4h, 24h")
			fs.StringVar(&reason, "reason", "", "why")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			s, err := c.SilenceAlertRule(ctx, pos[0], pos[1], dur, reason)
			return s, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "rule %s silenced until %s (silence %s)\n", pos[1], s.EndsAt.Format(time.RFC3339), s.ID)
			}, err
		},
	}
}

func describeMatchers(m apiclient.SilenceMatchers) string {
	var parts []string
	add := func(name string, vals []string) {
		if len(vals) > 0 {
			parts = append(parts, name+"="+strings.Join(vals, "|"))
		}
	}
	add("rule", m.RuleIDs)
	add("app", m.Apps)
	add("node", m.Nodes)
	add("kind", m.Kinds)
	add("severity", m.Severities)
	for k, v := range m.Labels {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, " ")
}

func printSilences(out io.Writer, list []apiclient.SilenceResource) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no silences")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tSTATUS\tMATCHES\tUNTIL\tBY\tREASON")
	for _, s := range list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", s.ID, s.Status, describeMatchers(s.Matchers), s.EndsAt.Format(time.RFC3339), orDash(s.CreatedBy), orDash(s.Reason))
	}
	_ = tw.Flush()
}
