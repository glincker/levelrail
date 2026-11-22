package main

import (
	"context"
	"fmt"
	"io"
)

func alertsDigestUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s alerts digest preview             print the weekly summary as it would be mailed
  %[1]s alerts digest schedule <off|day@hour>   for example mon@08 (UTC); off by default

The digest is mailed to the admin only when email is configured.
`, prog)
}

func runAlertsDigest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 {
		switch args[0] {
		case "preview":
			return runAPICmd(prog, digestPreviewCommand(), args[1:], stdout, stderr, lookupEnv)
		case "schedule":
			return runAPICmd(prog, digestScheduleCommand(), args[1:], stdout, stderr, lookupEnv)
		}
	}
	return dispatchSub(prog, "alerts digest", alertsDigestUsage, map[string]apiCmd{}, args, stdout, stderr, lookupEnv)
}

func digestPreviewCommand() apiCmd {
	return apiCmd{
		label: "alerts digest preview",
		usage: alertsDigestUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			p, err := c.AlertDigestPreview(ctx)
			return p, func(w io.Writer) { _, _ = fmt.Fprintf(w, "Subject: %s\n\n%s", p.Subject, p.Text) }, err
		},
	}
}

func digestScheduleCommand() apiCmd {
	return apiCmd{
		label: "alerts digest schedule",
		usage: alertsDigestUsage,
		run: func(ctx context.Context, c *Client, args []string) (any, func(io.Writer), error) {
			if len(args) != 1 {
				return nil, nil, newValidationError("give off or <day>@<hour>, for example mon@08")
			}
			st, err := c.SetDefaultAlertSettings(ctx, map[string]any{"digest_schedule": args[0]})
			return st, func(w io.Writer) { _, _ = fmt.Fprintf(w, "weekly digest: %s\n", st.DigestSchedule) }, err
		},
	}
}
