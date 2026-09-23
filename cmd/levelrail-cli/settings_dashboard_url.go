package main

import (
	"context"
	"fmt"
	"io"
)

func runSettingsDashboardURL(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsDashboardURLUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsDashboardURLUsage(prog))
		return exitOK
	case "get":
		return runListCommand(prog, args[1:], stdout, stderr, lookupEnv, listCommandParams[dashboardURLResource]{
			cmdLabel:  "settings dashboard-url get",
			jsonUsage: "print the dashboard URL setting as JSON to stdout and nothing else",
			usageText: fmt.Sprintf("Usage:\n  %s settings dashboard-url get [flags]\n\nShows the public dashboard URL.\n\nFlags:\n", prog),
			fetch: func(c *Client, ctx context.Context) (dashboardURLResource, error) {
				return c.GetDashboardURL(ctx)
			},
			errVerb: "get dashboard url",
			print:   printDashboardURLHuman,
		})
	case "set":
		return runSettingsDashboardURLSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings dashboard-url subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsDashboardURLUsage(prog))
		return exitUsage
	}
}

func settingsDashboardURLUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings dashboard-url get [flags]
  %[1]s settings dashboard-url set --url URL [flags]

The public URL operators use to reach the dashboard. Once it is https,
sign-in over plain HTTP is refused (set APP_ALLOW_INSECURE_LOGIN=true on
the control plane to recover). Pass --url "" to clear it.
`, prog)
}

func runSettingsDashboardURLSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings dashboard-url set", "print the updated setting as JSON to stdout and nothing else", stderr)
	var dashboardURL string
	fs.StringVar(&dashboardURL, "url", "", "public dashboard URL, e.g. https://dash.example.com (empty clears it)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings dashboard-url set --url URL [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.UpdateDashboardURL(context.Background(), dashboardURLResource{DashboardURL: dashboardURL})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set dashboard url: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printDashboardURLHuman(stdout, res) })
}

func printDashboardURLHuman(out io.Writer, r dashboardURLResource) {
	u := r.DashboardURL
	if u == "" {
		u = "(not set)"
	}
	_, _ = fmt.Fprintf(out, "dashboard_url: %s\n", u)
}
