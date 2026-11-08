package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const mailBroadcastUsageLines = `  %[1]s mail broadcasts list                       list broadcasts with sent, failed and suppressed counts
  %[1]s mail broadcasts send --subject S (--body T | --body-file F) (--audience ID | --to a@d,b@d) [--schedule RFC3339] [--dry-run]
                                                   queue an announcement; --dry-run only counts who would get it
  %[1]s mail broadcasts cancel <id>                stop a scheduled or running broadcast
  %[1]s mail audiences list                        list saved recipient groups
  %[1]s mail suppressions list|add|remove [address] [--reason R]   manage addresses that never get announcements
`

// runMailBroadcasts handles the "mail broadcasts", "mail audiences" and "mail suppressions" groups.
func runMailBroadcasts(prog, group string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: mail %s needs a subcommand\n\n%s", prog, group, mailUsage(prog))
		return 2
	}
	sub, rest := args[0], args[1:]
	label := "mail " + group + " " + sub
	ctx := context.Background()
	var subject, body, bodyFile, audience, to, schedule, reason string
	var dryRun bool
	parse := func(want int, argLabel, usage string, setup func(fs *flag.FlagSet)) (dbAccessCall, int, bool) {
		return parseDBAccess(prog, label, label+usage, rest, want, argLabel, stdout, stderr, lookupEnv, setup)
	}
	switch group + " " + sub {
	case "broadcasts list":
		c, code, ok := parse(0, "", "", nil)
		if !ok {
			return code
		}
		res, err := c.client.MailBroadcasts(ctx)
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() {
			tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ID\tSTATUS\tSENT\tFAILED\tSUPPRESSED\tTOTAL\tSUBJECT")
			for _, b := range res {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%d\t%s\n", b.ID, b.Status, b.Sent, b.Failed, b.Suppressed, b.Total, b.Subject)
			}
			_ = tw.Flush()
		})
	case "broadcasts send":
		c, code, ok := parse(0, "", " --subject S (--body T | --body-file F) (--audience ID | --to a@d,b@d)", func(fs *flag.FlagSet) {
			fs.StringVar(&subject, "subject", "", "message subject")
			fs.StringVar(&body, "body", "", "message body")
			fs.StringVar(&bodyFile, "body-file", "", "read the message body from a file")
			fs.StringVar(&audience, "audience", "", "saved audience group id")
			fs.StringVar(&to, "to", "", "comma separated extra recipient addresses")
			fs.StringVar(&schedule, "schedule", "", "send later, an RFC 3339 time within 90 days")
			fs.BoolVar(&dryRun, "dry-run", false, "count who would get the message without sending")
		})
		if !ok {
			return code
		}
		if bodyFile != "" {
			raw, err := os.ReadFile(bodyFile) //nolint:gosec // operator-chosen file for the message body
			if err != nil {
				return c.fail(fmt.Errorf("%s: --body-file: %w", label, err))
			}
			body = string(raw)
		}
		if subject == "" || body == "" {
			return c.fail(fmt.Errorf("%s: --subject and a body (--body or --body-file) are required", label))
		}
		req := apiclient.MailBroadcastRequest{Subject: subject, Body: body, Audience: audience, Recipients: splitList(to), ScheduleAt: schedule}
		if dryRun {
			plan, err := c.client.PlanMailBroadcast(ctx, req)
			if err != nil {
				return c.fail(fmt.Errorf("%s: %w", label, err))
			}
			return c.result(plan, func() {
				_, _ = fmt.Fprintf(stdout, "dry run: %d recipients, %d deliverable, %d suppressed, %d left of today's cap\n", plan.Total, plan.Deliverable, plan.Suppressed, plan.CapRemaining)
				if !plan.ProviderConfigured {
					_, _ = fmt.Fprintln(stdout, "no email provider is configured; a real send will be refused")
				}
			})
		}
		res, err := c.client.CreateMailBroadcast(ctx, req)
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() {
			_, _ = fmt.Fprintf(stdout, "broadcast %s: %s (%d recipients)\n", res.ID, res.Status, res.Total)
		})
	case "broadcasts cancel":
		c, code, ok := parse(1, "<id>", " <id>", nil)
		if !ok {
			return code
		}
		res, err := c.client.MailBroadcastCancel(ctx, c.rest[0])
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() { _, _ = fmt.Fprintf(stdout, "broadcast %s: %s\n", res.ID, res.Status) })
	case "audiences list":
		c, code, ok := parse(0, "", "", nil)
		if !ok {
			return code
		}
		res, err := c.client.ListMailGroups(ctx)
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() {
			tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ID\tNAME\tDESCRIPTION")
			for _, g := range res {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", g.ID, g.Name, g.Description)
			}
			_ = tw.Flush()
		})
	case "suppressions list":
		c, code, ok := parse(0, "", "", nil)
		if !ok {
			return code
		}
		res, err := c.client.MailSuppressions(ctx)
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() {
			tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ADDRESS\tREASON\tSINCE")
			for _, s := range res {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Email, s.Reason, s.CreatedAt)
			}
			_ = tw.Flush()
		})
	case "suppressions add":
		c, code, ok := parse(1, "<address>", " <address> [--reason R]", func(fs *flag.FlagSet) {
			fs.StringVar(&reason, "reason", "", "why the address is suppressed")
		})
		if !ok {
			return code
		}
		res, err := c.client.MailSuppressionAdd(ctx, c.rest[0], reason)
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() { _, _ = fmt.Fprintf(stdout, "suppressed %s\n", res.Email) })
	case "suppressions remove":
		c, code, ok := parse(1, "<address>", " <address>", nil)
		if !ok {
			return code
		}
		if err := c.client.MailSuppressionRemove(ctx, c.rest[0]); err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(map[string]string{"removed": c.rest[0]}, func() { _, _ = fmt.Fprintf(stdout, "removed %s\n", c.rest[0]) })
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown subcommand %q\n\n%s", prog, label, mailUsage(prog))
	return 2
}
