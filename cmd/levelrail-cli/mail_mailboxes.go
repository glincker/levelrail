package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const mailMailboxUsageLines = `  %[1]s mail domains list                          list the mail domains your teams own
  %[1]s mail domains claim <domain> --team ID [--app name]   give a served domain to one team (asks your password)
  %[1]s mail mailboxes list [--domain D]           list mailboxes
  %[1]s mail mailboxes add <address> [--linked-user ID]    create a mailbox; prints its password once
  %[1]s mail mailboxes suspend|resume <address>    block or allow logins; mail keeps arriving while suspended
  %[1]s mail mailboxes passwd <address>            set a new generated password, shown once (asks your password)
  %[1]s mail mailboxes delete <address> --confirm <address> [--delete-mail]   delete for good (asks your password)
  %[1]s mail aliases list --domain D               list a domain's aliases
  %[1]s mail aliases add <address> --to a@d,b@d    forward an address to mailboxes your team owns
  %[1]s mail aliases remove <address>              remove an alias
  %[1]s mail queue list [--app name] [--max N]     list queued mail (sender, recipient domain, attempts, last error)
  %[1]s mail queue hold|release [--app name] [--ids 1,2]   pause or resume delivery; release asks your password
`

const mailShownOnceLine = "Shown once: Levelrail does not store this password. Copy it now."

// runMailMailboxes handles the "mail domains", "mail mailboxes", "mail aliases" and "mail queue" groups.
func runMailMailboxes(prog, group string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: mail %s needs a subcommand\n\n%s", prog, group, mailUsage(prog))
		return 2
	}
	sub, rest := args[0], args[1:]
	label := "mail " + group + " " + sub
	ctx := context.Background()
	var app, domain, team, linked, confirm, to, ids string
	var deleteMail bool
	var limit int
	parse := func(want int, argLabel, usage string, setup func(fs *flag.FlagSet)) (dbAccessCall, int, bool) {
		return parseDBAccess(prog, label, label+usage, rest, want, argLabel, stdout, stderr, lookupEnv, setup)
	}
	switch group + " " + sub {
	case "domains list":
		c, code, ok := parse(0, "", "", nil)
		if !ok {
			return code
		}
		res, err := c.client.MailDomains(ctx)
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() {
			tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "DOMAIN\tTEAM\tAPP")
			for _, d := range res {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", d.Domain, d.TeamID, d.Service)
			}
			_ = tw.Flush()
		})
	case "domains claim":
		c, code, ok := parse(1, "a domain", " <domain> --team ID [--app name]", func(fs *flag.FlagSet) {
			fs.StringVar(&team, "team", "", "team that owns the domain and its mailboxes")
			fs.StringVar(&app, "app", "", "mail app when several are installed")
		})
		if !ok {
			return code
		}
		if team == "" {
			return c.fail(fmt.Errorf("%s: --team is required", label))
		}
		res, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.MailDomain, error) {
			return c.client.ClaimMailDomain(ctx, c.rest[0], team, app, p)
		})
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() { _, _ = fmt.Fprintf(stdout, "%s now belongs to team %s\n", res.Domain, res.TeamID) })
	case "mailboxes list":
		c, code, ok := parse(0, "", " [--domain D]", func(fs *flag.FlagSet) {
			fs.StringVar(&domain, "domain", "", "only this domain")
		})
		if !ok {
			return code
		}
		res, err := c.client.Mailboxes(ctx, domain)
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() {
			tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ADDRESS\tTEAM\tSTATE\tLINKED USER")
			for _, m := range res {
				state := "active"
				switch {
				case m.Suspended:
					state = "suspended"
				case m.SuspendPending:
					state = "suspend pending (mail server unreachable, retried on the next mail call)"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", m.Address, m.TeamID, state, m.LinkedUserID)
			}
			_ = tw.Flush()
		})
	case "mailboxes add":
		c, code, ok := parse(1, "an address", " <address> [--linked-user ID]", func(fs *flag.FlagSet) {
			fs.StringVar(&linked, "linked-user", "", "Levelrail user this mailbox belongs to; it is suspended when they leave the team")
		})
		if !ok {
			return code
		}
		res, err := c.client.CreateMailbox(ctx, c.rest[0], linked)
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() { printMailboxSecret(stdout, res) })
	case "mailboxes suspend", "mailboxes resume":
		c, code, ok := parse(1, "an address", " <address>", nil)
		if !ok {
			return code
		}
		res, err := c.client.SetMailboxSuspended(ctx, c.rest[0], sub == "suspend")
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() { _, _ = fmt.Fprintf(stdout, "%s: suspended=%t\n", res.Address, res.Suspended) })
	case "mailboxes passwd":
		c, code, ok := parse(1, "an address", " <address>", nil)
		if !ok {
			return code
		}
		res, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.MailboxSecret, error) {
			return c.client.ResetMailboxPassword(ctx, c.rest[0], p)
		})
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() { printMailboxSecret(stdout, res) })
	case "mailboxes delete":
		c, code, ok := parse(1, "an address", " <address> --confirm <address> [--delete-mail]", func(fs *flag.FlagSet) {
			fs.StringVar(&confirm, "confirm", "", "type the mailbox address again to confirm")
			fs.BoolVar(&deleteMail, "delete-mail", false, "also delete the mail it holds; without it a mailbox with mail is refused")
		})
		if !ok {
			return code
		}
		if !strings.EqualFold(confirm, c.rest[0]) {
			return c.fail(fmt.Errorf("%s: --confirm must repeat the address", label))
		}
		res, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (struct{}, error) {
			return c.client.DeleteMailbox(ctx, c.rest[0], confirm, deleteMail, p)
		})
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() { _, _ = fmt.Fprintf(stdout, "deleted %s\n", c.rest[0]) })
	case "aliases list":
		c, code, ok := parse(0, "", " --domain D", func(fs *flag.FlagSet) {
			fs.StringVar(&domain, "domain", "", "domain whose aliases to list")
		})
		if !ok {
			return code
		}
		res, err := c.client.MailAliases(ctx, domain)
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() {
			for _, a := range res {
				_, _ = fmt.Fprintf(stdout, "%s -> %s\n", a.Address, strings.Join(a.Targets, ", "))
			}
		})
	case "aliases add":
		c, code, ok := parse(1, "an address", " <address> --to a@d,b@d", func(fs *flag.FlagSet) {
			fs.StringVar(&to, "to", "", "comma separated mailboxes that receive the alias mail")
		})
		if !ok {
			return code
		}
		res, err := c.client.AddMailAlias(ctx, c.rest[0], splitList(to))
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() { _, _ = fmt.Fprintf(stdout, "%s -> %s\n", res.Address, strings.Join(res.Targets, ", ")) })
	case "aliases remove":
		c, code, ok := parse(1, "an address", " <address>", nil)
		if !ok {
			return code
		}
		if err := c.client.RemoveMailAlias(ctx, c.rest[0]); err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(map[string]string{"removed": c.rest[0]}, func() { _, _ = fmt.Fprintf(stdout, "removed %s\n", c.rest[0]) })
	case "queue list":
		c, code, ok := parse(0, "", " [--app name] [--max N]", func(fs *flag.FlagSet) {
			fs.StringVar(&app, "app", "", "mail app when several are installed")
			fs.IntVar(&limit, "max", 0, "most entries to show (default 100, at most 500)")
		})
		if !ok {
			return code
		}
		res, err := c.client.MailQueueList(ctx, app, limit)
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() {
			tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ID\tSENDER\tTO DOMAIN\tATTEMPTS\tHOLD\tLAST ERROR")
			for _, e := range res.Entries {
				_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%t\t%s\n", e.ID, e.SenderAccount, e.RecipientDomain, e.Attempts, e.Hold, e.LastError)
			}
			_ = tw.Flush()
		})
	case "queue hold", "queue release":
		c, code, ok := parse(0, "", " [--app name] [--ids 1,2]", func(fs *flag.FlagSet) {
			fs.StringVar(&app, "app", "", "mail app when several are installed")
			fs.StringVar(&ids, "ids", "", "comma separated queue ids; without it the whole queue")
		})
		if !ok {
			return code
		}
		var list []int64
		for _, s := range splitList(ids) {
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return c.fail(fmt.Errorf("%s: --ids: %q is not a queue id", label, s))
			}
			list = append(list, n)
		}
		hold := sub == "hold"
		call := func(p *apiclient.StepUpProof) (apiclient.MailQueueHoldResult, error) {
			return c.client.MailQueueHold(ctx, app, list, hold, p)
		}
		var res apiclient.MailQueueHoldResult
		var err error
		if hold {
			res, err = call(nil)
		} else {
			res, err = withStepUp(stderr, call)
		}
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() {
			_, _ = fmt.Fprintf(stdout, "%s: %s on %s (changed %d)\n", res.App, sub, res.Scope, res.Changed)
		})
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown subcommand %q\n\n%s", prog, label, mailUsage(prog))
	return 2
}

func printMailboxSecret(out io.Writer, s apiclient.MailboxSecret) {
	_, _ = fmt.Fprintf(out, "mailbox: %s (team %s)\npassword: %s\n%s\nsend cap: %d per hour (advisory)\n", s.Mailbox.Address, s.Mailbox.TeamID, s.Password, mailShownOnceLine, s.SendCapPerHour)
}
