package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func alertsDefaultsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s alerts defaults show
  %[1]s alerts defaults enable|disable            master switch for every default alert
  %[1]s alerts defaults list
  %[1]s alerts defaults enable|disable <id>       one default alert
  %[1]s alerts defaults reset

Default alerts are seeded on first boot: certificate expiry at 14, 7, 3 and
1 days, restore drill failures, node offline and more. Disabling one keeps
it disabled; it is never re-created or overwritten on restart.
`, prog)
}

func runAlertsDefaults(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 {
		switch args[0] {
		case "show":
			return runAPICmd(prog, defaultsShowCommand(), args[1:], stdout, stderr, lookupEnv)
		case "list":
			return runAPICmd(prog, defaultsListCommand(), args[1:], stdout, stderr, lookupEnv)
		case "reset":
			return runAPICmd(prog, defaultsResetCommand(), args[1:], stdout, stderr, lookupEnv)
		case "enable":
			return runAPICmd(prog, defaultsToggleCommand(true), args[1:], stdout, stderr, lookupEnv)
		case "disable":
			return runAPICmd(prog, defaultsToggleCommand(false), args[1:], stdout, stderr, lookupEnv)
		}
	}
	return dispatchSub(prog, "alerts defaults", alertsDefaultsUsage, map[string]apiCmd{}, args, stdout, stderr, lookupEnv)
}

func defaultsListCommand() apiCmd {
	return apiCmd{
		label: "alerts defaults list",
		usage: alertsDefaultsUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListDefaultAlerts(ctx)
			return list, func(w io.Writer) { printDefaultAlerts(w, list) }, err
		},
	}
}

func printDefaultAlerts(out io.Writer, list []apiclient.DefaultAlert) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no default alerts")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tSEVERITY\tENABLED\tFIRING")
	for _, a := range list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%t\n", a.ID, a.Name, a.Severity, a.Enabled, a.Firing)
	}
	_ = tw.Flush()
}

func defaultsToggleCommand(enabled bool) apiCmd {
	verb := "disable"
	if enabled {
		verb = "enable"
	}
	return apiCmd{
		label: "alerts defaults " + verb,
		usage: alertsDefaultsUsage,
		run: func(ctx context.Context, c *Client, args []string) (any, func(io.Writer), error) {
			if len(args) == 0 {
				st, err := c.SetDefaultAlertSettings(ctx, map[string]any{"enabled": enabled})
				return st, func(w io.Writer) { printDefaultAlertSettings(w, st) }, err
			}
			if len(args) != 1 {
				return nil, nil, newValidationError("give one default alert ID (see \"alerts defaults list\") or none for the master switch")
			}
			a, err := c.SetDefaultAlertEnabled(ctx, args[0], enabled)
			return a, func(w io.Writer) { _, _ = fmt.Fprintf(w, "%s: enabled=%t\n", a.ID, a.Enabled) }, err
		},
	}
}

func defaultsResetCommand() apiCmd {
	return apiCmd{
		label: "alerts defaults reset",
		usage: alertsDefaultsUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ResetDefaultAlerts(ctx)
			return list, func(w io.Writer) { printDefaultAlerts(w, list) }, err
		},
	}
}

func defaultsShowCommand() apiCmd {
	return apiCmd{
		label: "alerts defaults show",
		usage: alertsDefaultsUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			st, err := c.DefaultAlertSettings(ctx)
			return st, func(w io.Writer) { printDefaultAlertSettings(w, st) }, err
		},
	}
}

func printDefaultAlertSettings(w io.Writer, st apiclient.DefaultAlertSettings) {
	_, _ = fmt.Fprintf(w, "default alerts: %t\ncrash loop restarts: %d\n5xx rate percent: %d\nweekly digest: %s\n",
		st.Enabled, st.CrashloopRestarts, st.ErrorRatePercent, st.DigestSchedule)
}
